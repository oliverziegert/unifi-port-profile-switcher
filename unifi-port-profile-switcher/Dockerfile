# https://developers.home-assistant.io/docs/apps/configuration#app-dockerfile
ARG BUILD_FROM=ghcr.io/home-assistant/base:3.23-2026.04.0@sha256:5ea71eca9735949b080a05d271988c9663af3bd1050297abedefa222c61e6807

FROM golang:1.26-alpine@sha256:30b1e546a9c7fa3b747c2bdba3a0c7bdf3a02ebb4c5b8405d2f948d7e8aac4ec AS builder
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
