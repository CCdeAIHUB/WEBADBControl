# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS web-build
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM rust:1.97-bookworm AS core-build
WORKDIR /src/core
COPY core/Cargo.toml core/Cargo.lock ./
COPY core/crates ./crates
COPY core/assets ./assets
RUN cargo build --locked --release -p adbcontrol-core

FROM golang:1.23-bookworm AS server-build
WORKDIR /src/server
COPY server/go.mod server/go.sum* ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/webadbcontrol ./cmd/webadbcontrol

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends adb ca-certificates usbutils && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/webadbcontrol
COPY --from=server-build /out/webadbcontrol ./webadbcontrol
COPY --from=core-build /src/core/target/release/adbcontrol-core ./adbcontrol-core
COPY --from=web-build /src/web/dist ./web
COPY core/assets ./assets
COPY core/android/companion-app/app/src/main ./companion-source
RUN mkdir -p data assets/adb/linux-x64 assets/adb/linux-arm64 && \
    ln -sf /usr/bin/adb assets/adb/linux-x64/adb && \
    ln -sf /usr/bin/adb assets/adb/linux-arm64/adb && \
    useradd --system --home /opt/webadbcontrol --shell /usr/sbin/nologin webadb && \
    chown -R webadb:webadb /opt/webadbcontrol/data
ENV WEBADB_ADDRESS=0.0.0.0:8080 \
    WEBADB_CORE_BINARY=/opt/webadbcontrol/adbcontrol-core \
    WEBADB_DATA_DIR=/opt/webadbcontrol/data \
    WEBADB_WEB_DIR=/opt/webadbcontrol/web
EXPOSE 8080
USER webadb
ENTRYPOINT ["/opt/webadbcontrol/webadbcontrol"]
