package httpapi

// User administration: listing accounts, soft-deleting with retention,
// restoring inside the window, and revoking privileged roles.
//
// Owners (manage-owners) run deletions, restorations and anything that
// touches an admin or an owner account. Admins (manage-users,
// manage-roles) list accounts, create them (including admins) and
// deactivate non-admins. Secretaries never reach these routes: they lack
// both guards, exactly like the role matrix intends.

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"grade/src/core/audit"
	"grade/src/core/roles"
	"grade/src/core/users"
)

// userRow is the admin console's view of one login account.
type userRow struct {
	ID        string       `json:"ID"`
	Username  string       `json:"Username"`
	Email     string       `json:"Email"`
	Active    bool         `json:"Active"`
	DeletedAt *string      `json:"DeletedAt,omitempty"`
	Roles     []roles.Role `json:"Roles"`
}

// toUserRow renders a user with its roles for listing and detail.
func toUserRow(u users.User, rs []roles.Role) userRow {
	if rs == nil {
		rs = []roles.Role{}
	}
	row := userRow{
		ID: u.ID, Username: u.Username, Email: u.Email, Active: u.Active, Roles: rs,
	}
	if u.DeletedAt != nil {
		s := u.DeletedAt.UTC().Format("2006-01-02T15:04:05Z")
		row.DeletedAt = &s
	}
	return row
}

// callerIsOwner reports whether the caller holds the exclusive owner
// capability. Authorizer failures fail closed.
func callerIsOwner(deps Dependencies, r *http.Request) bool {
	subject, ok := SubjectFrom(r.Context())
	if !ok {
		return false
	}
	allowed, err := deps.Auth.Can(r.Context(), subject, roles.PermissionManageOwners)
	if err != nil {
		return false
	}
	return allowed
}

// hasPrivilegedRole reports whether id holds the admin or the owner
// role. Unknown subjects answer false without failing.
func hasPrivilegedRole(deps Dependencies, r *http.Request, id string) bool {
	if deps.RolesSvc == nil {
		return false
	}
	rs, err := deps.RolesSvc.Roles(r.Context(), id)
	if err != nil {
		return false
	}
	for _, role := range rs {
		if role == roles.RoleAdmin || role == roles.RoleOwner {
			return true
		}
	}
	return false
}

// activeOwners returns the active, non-deleted owners except excludeID.
// It backs the last-owner guard: any destructive owner action must leave
// at least one entry behind.
func activeOwners(deps Dependencies, r *http.Request, excludeID string) []users.User {
	if deps.RolesSvc == nil || deps.Users == nil {
		return nil
	}
	ids, err := deps.RolesSvc.SubjectsWithRole(r.Context(), roles.RoleOwner)
	if err != nil {
		return nil
	}
	var out []users.User
	for _, id := range ids {
		if id == excludeID {
			continue
		}
		u, err := deps.Users.ByID(r.Context(), id)
		if err != nil {
			continue
		}
		if u.Active && u.DeletedAt == nil {
			out = append(out, u)
		}
	}
	return out
}

// guardLastOwner refuses to remove owner standing from the last active
// owner, whether by delete, deactivation or role revocation. Without it
// the system could lock every administrator out with no one left able
// to restore access.
func guardLastOwner(deps Dependencies, r *http.Request, targetID string) error {
	targetRoles, err := deps.RolesSvc.Roles(r.Context(), targetID)
	if err != nil {
		return err
	}
	isOwner := false
	for _, role := range targetRoles {
		if role == roles.RoleOwner {
			isOwner = true
			break
		}
	}
	if !isOwner {
		return nil
	}
	if len(activeOwners(deps, r, targetID)) == 0 {
		return users.ErrLastOwner
	}
	return nil
}

// handleListUsers returns every login account with its roles. Deleted
// accounts only appear with includeDeleted, which is owner-only, so a
// routine admin listing never leaks the retention bin.
func handleListUsers(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var roleFilter *roles.Role
		if raw := strings.TrimSpace(q.Get("role")); raw != "" {
			role, err := roles.Parse(raw)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			roleFilter = &role
		}
		includeDeleted := q.Get("includeDeleted") == "true" || q.Get("includeDeleted") == "1"
		if includeDeleted && !callerIsOwner(deps, r) {
			WriteError(w, r, errForbidden)
			return
		}
		all, err := deps.Users.List(r.Context(), includeDeleted && callerIsOwner(deps, r))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]userRow, 0, len(all))
		for _, u := range all {
			rs, err := deps.Roles.Roles(r.Context(), u.ID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if roleFilter != nil {
				kept := false
				for _, held := range rs {
					if held == *roleFilter {
						kept = true
						break
					}
				}
				if !kept {
					continue
				}
			}
			out = append(out, toUserRow(u, rs))
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Username != out[j].Username {
				return out[i].Username < out[j].Username
			}
			return out[i].ID < out[j].ID
		})
		writeJSON(w, http.StatusOK, out)
	}
}

// handleGetUser returns one login account with its roles. Soft-deleted
// accounts answer 404 unless the caller is an owner.
func handleGetUser(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		u, err := deps.Users.ByID(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if u.DeletedAt != nil && !callerIsOwner(deps, r) {
			WriteError(w, r, users.ErrNotFound)
			return
		}
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserRow(u, rs))
	}
}

// handleDeleteUser soft-deletes a login account (owner-only): the login
// dies immediately while the row survives the retention window for a
// possible restore. Deleting your own account is refused, and the last
// active owner cannot be removed.
func handleDeleteUser(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		subject, _ := SubjectFrom(r.Context())
		if subject != "" && subject == id {
			recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, id, "", http.StatusForbidden)
			WriteError(w, r, users.ErrSelfDelete)
			return
		}
		if _, err := deps.Users.ByID(r.Context(), id); err != nil {
			WriteError(w, r, err)
			return
		}
		if err := guardLastOwner(deps, r, id); err != nil {
			recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, id, "", http.StatusConflict)
			WriteError(w, r, err)
			return
		}
		if err := deps.Users.Delete(r.Context(), id); err != nil {
			recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, id, "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		if deps.Tokens != nil {
			if err := deps.Tokens.RevokeAllForUser(r.Context(), id); err != nil {
				// Best effort: bearers already die on the version bump.
				recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, id, "", http.StatusOK)
			}
		}
		u, err := deps.Users.ByID(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, id, "", http.StatusOK)
		writeJSON(w, http.StatusOK, toUserRow(u, rs))
	}
}

// handleRestoreUser brings a soft-deleted account back inside its
// retention window (owner-only). Past the window only a fresh account
// can replace it.
func handleRestoreUser(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		u, err := deps.Users.Restore(r.Context(), id)
		if err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceUser, id, "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceUser, id, "", http.StatusOK)
		writeJSON(w, http.StatusOK, toUserRow(u, rs))
	}
}

// handleRemoveRole revokes one role from a subject. Removing the admin
// or the owner role is owner-only, and the last active owner keeps its
// standing. Anything else stays under manage-roles.
func handleRemoveRole(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body roleAssignment
		if !decodeJSON(w, r, &body) {
			return
		}
		role, err := roles.Parse(body.Role)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		subjectID := strings.TrimSpace(body.SubjectID)
		if subjectID == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		if role == roles.RoleAdmin || role == roles.RoleOwner {
			if !callerIsOwner(deps, r) {
				recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, subjectID, "", http.StatusForbidden)
				WriteError(w, r, errForbidden)
				return
			}
			if role == roles.RoleOwner {
				if err := guardLastOwner(deps, r, subjectID); err != nil {
					recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, subjectID, "", http.StatusConflict)
					WriteError(w, r, err)
					return
				}
			}
		}
		if err := deps.RolesSvc.Remove(r.Context(), subjectID, role); err != nil {
			WriteError(w, r, err)
			return
		}
		rs, err := deps.RolesSvc.Roles(r.Context(), subjectID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionDelete, audit.ResourceUser, subjectID, "", http.StatusOK)
		writeJSON(w, http.StatusOK, roleGrant{SubjectID: subjectID, Roles: rs})
	}
}
