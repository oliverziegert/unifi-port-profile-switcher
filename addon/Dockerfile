# https://developers.home-assistant.io/docs/apps/configuration#app-dockerfile
ARG BUILD_FROM=ghcr.io/home-assistant/base:3.23-2026.04.0@sha256:5ea71eca9735949b080a05d271988c9663af3bd1050297abedefa222c61e6807

FROM golang:1.26-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder
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

LABEL \
    org.opencontainers.image.title="UniFi Port Profile Switcher" \
    org.opencontainers.image.description="Flip UniFi switch port profiles via a local HTTP API — trigger from HA dashboards, automations, and scripts." \
    org.opencontainers.image.source="https://github.com/oliverziegert/unifi-port-profile-switcher" \
    org.opencontainers.image.licenses="Apache License 2.0"
