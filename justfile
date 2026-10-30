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
# Pass os and arch to cross-compile, for example `just build-prod linux arm64`.
build-prod os="" arch="": types
    pnpm --dir web build
    pnpm --dir docs build
    CGO_ENABLED=0 GOOS={{os}} GOARCH={{arch}} go build -tags web -trimpath -ldflags='-s -w' -o tstamps-server ./cmd/tstamps-server

# Build locally and deploy with Ansible: just deploy time.example.com [user] [arch]
deploy host user="root" arch="amd64":
    just build-prod linux {{arch}}
    ansible-playbook -i "{{host}}," -u {{user}} deploy/ansible/deploy.yml

# Provision a fresh Debian/Ubuntu VM as root with Ansible: just provision example.com [domain] [admin-email]. The domain must already resolve to the host.
provision host domain="" email="" arch="amd64":
    just build-prod linux {{arch}}
    ansible-playbook -i "{{host}}," -u root deploy/ansible/provision.yml -e tstamps_domain={{ if domain == "" { host } else { domain } }} -e tstamps_admin_email={{email}}

# Provision the public demo on a VM: just provision-demo demo.example.com [domain]. Same playbook, demo site only; run it on a production VM to add the demo beside it.
provision-demo host domain="" arch="amd64":
    just build-prod linux {{arch}}
    ansible-playbook -i "{{host}}," -u root deploy/ansible/provision.yml -e tstamps_prod=false -e tstamps_demo=true -e tstamps_domain={{host}} -e tstamps_demo_domain={{ if domain == "" { host } else { domain } }}

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

# Run the server binary with the UI and the manual embedded.
# Port: PORT=8081 just serve (default 8080).
serve *args:
    @if [ ! -f ./tstamps-server ]; then just build; fi
    ./tstamps-server serve --addr :${PORT:-8080} {{args}}

# Create a workspace (slug, admin email, admin name) and print its initial password.
workspace-create slug email name:
    go run ./cmd/tstamps-server workspace create {{slug}} --db ./tstamps.db --email {{email}} --name "{{name}}"

# Run the browser smoke test against a freshly built server.
e2e: build
    pnpm --dir web e2e
