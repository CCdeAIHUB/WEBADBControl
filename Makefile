.PHONY: dev test build build-web build-server build-core

dev:
	cd web && pnpm dev

test:
	cd server && go test ./...
	cd web && pnpm test
	cd core && cargo test --workspace

build: build-web build-core build-server

build-web:
	cd web && pnpm build

build-core:
	cd core && cargo build --release -p adbcontrol-core

build-server:
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o webadbcontrol ./cmd/webadbcontrol
