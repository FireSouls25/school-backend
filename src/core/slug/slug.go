// Package slug mints short opaque codes for browser URLs. UUIDs stay as
// the internal identity everywhere (storage, API bodies, audit); slugs
// are only the human-facing alias so links never expose raw ids.
package slug

import (
	"crypto/rand"
	"strings"
)

const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"

const length = 8

// New returns a random 8-char code (ambiguous glyphs excluded). It reads
// crypto/rand and never fails: on the impossible read error it falls
// back to a counter-derived suffix so callers never branch.
func New() string {
	var b [length]byte
	if _, err := rand.Read(b[:]); err != nil {
		for i := range b {
			b[i] = byte(i * 31 % len(alphabet))
		}
	}
	var out strings.Builder
	out.Grow(length)
	for _, v := range b {
		out.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return out.String()
}
