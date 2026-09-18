# https://developers.home-assistant.io/docs/apps/configuration#app-dockerfile
ARG BUILD_FROM=ghcr.io/home-assistant/base:3.23-2026.04.0@sha256:5ea71eca9735949b080a05d271988c9663af3bd1050297abedefa222c61e6807

FROM golang:1.27-alpine@sha256:4cb7ac979db5fcc41cae44b2227ba5ab8a51e8807f40d9ba4dee20a0ad960b5b AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o /usr/local/bin/unifi-port-profile-switcher .

FROM ${BUILD_FROM}

COPY --from=builder /usr/local/bin/unifi-port-profile-switcher /usr/local/bin/unifi-port-profile-switcher
# Copy root filesystem
COPY rootfs /

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://localhost:8099/healthz || exit 1

# OCI and io.hass.* labels are injected by .github/workflows/build-app.yaml so
# they carry workflow-context provenance (revision, version, created). Do not
# hand-write them here.
