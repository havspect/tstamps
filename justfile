# List the recipes.
default:
    @just --list

# Regenerate the API types from the Go OpenAPI document.
types:
    pnpm --dir web gen

# Run every check the build gate runs.
check:
    go vet ./...
    go test ./...
    pnpm --dir web check
    pnpm --dir web test
    pnpm --dir docs build

# Build the user CLI: no UI, no database, no build tags.
build-cli:
    go build -o tstamps ./cmd/tstamps

# Build the server binary with the UI and the manual embedded.
build: types
    pnpm --dir web build
    pnpm --dir docs build
    go build -tags web -o tstamps-server ./cmd/tstamps-server

# Build a static, stripped server binary for deployment (the SQLite driver is pure Go, so no CGO).
build-prod: types
    pnpm --dir web build
    pnpm --dir docs build
    CGO_ENABLED=0 go build -tags web -trimpath -ldflags='-s -w' -o tstamps-server ./cmd/tstamps-server

# Build the manual into web/dist/docs; a dead internal link fails the build.
docs:
    pnpm --dir docs build

# Preview the manual at http://localhost:5174/docs/ with hot reload.
docs-dev:
    pnpm --dir docs dev

# Run the API and the Vite dev server together, both reloading (needs hivemind and air).
dev: types
    hivemind Procfile.dev

# Run the API with hot reload; the UI is served by `pnpm --dir web dev`.
serve-watch:
    air

# Run the API only; the UI is embedded in the binary.
serve:
    go run ./cmd/tstamps-server serve

# Run the browser smoke test against a freshly built server.
e2e: build
    pnpm --dir web e2e
