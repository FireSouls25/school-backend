# Build: compile a static server binary, then ship it in a minimal image.
# Two stages keep the toolchain (and its CVEs) out of the runtime image.

# --- build -----------------------------------------------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY src ./src

# CGO off produces a static binary that runs on a scratch-like base.
# -trimpath and the symbol table removal keep the image small.
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/server ./src/cmd/server

# --- runtime ---------------------------------------------------------------
FROM alpine:3.20

# ca-certificates: outbound TLS (Postgres, WhatsApp API later).
# tzdata: Colombia dates (America/Bogota) are printed by the domain.
# wget: busybox, used by the compose healthcheck.
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /app grado

WORKDIR /app
COPY --from=build /out/server /app/server

USER grado
EXPOSE 8080

ENV APP_ENV=production \
    PORT=8080

# The compose healthcheck polls this; keep it cheap and dependency-free.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/server"]