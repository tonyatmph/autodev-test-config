#!/usr/bin/env bash
set -euo pipefail

root="${AUTODEV_E2E_REPO_ROOT:-/tmp/autodev-e2e-repos}"
mkdir -p "$root"

app_repo="$root/autodev-e2e-app"
gitops_repo="$root/autodev-e2e-gitops"
journal_repo="$root/autodev-e2e-work-orders"

init_git_repo() {
  local repo_dir="$1"
  local remote_url="${2:-}"
  mkdir -p "$repo_dir"
  rm -f "$repo_dir/.git/index.lock"
  if [ ! -d "$repo_dir/.git" ]; then
    git -C "$repo_dir" init --initial-branch=main >/dev/null
  fi
  git -C "$repo_dir" config user.name "autodev-stage1" >/dev/null
  git -C "$repo_dir" config user.email "autodev.stage1@example.com" >/dev/null
  if [ -n "$remote_url" ]; then
    if git -C "$repo_dir" remote get-url origin >/dev/null 2>&1; then
      git -C "$repo_dir" remote set-url origin "$remote_url" >/dev/null
    else
      git -C "$repo_dir" remote add origin "$remote_url" >/dev/null
    fi
  fi
}

reset_remote_backed_repo() {
  local repo_dir="$1"
  rm -f "$repo_dir/.git/index.lock"
  if git -C "$repo_dir" remote get-url origin >/dev/null 2>&1; then
    git -C "$repo_dir" fetch origin main >/dev/null 2>&1 || true
    git -C "$repo_dir" checkout main >/dev/null 2>&1 || git -C "$repo_dir" checkout -b main >/dev/null 2>&1
    if git -C "$repo_dir" rev-parse --verify origin/main >/dev/null 2>&1; then
      git -C "$repo_dir" reset --hard origin/main >/dev/null 2>&1
    fi
  else
    git -C "$repo_dir" checkout main >/dev/null 2>&1 || git -C "$repo_dir" checkout -b main >/dev/null 2>&1
  fi
  git -C "$repo_dir" clean -fd >/dev/null 2>&1 || true
  while IFS= read -r branch; do
    if [ -n "$branch" ]; then
      git -C "$repo_dir" branch -D "$branch" >/dev/null 2>&1 || true
    fi
  done < <(git -C "$repo_dir" for-each-ref --format='%(refname:short)' refs/heads/autodev)
}

init_git_repo "$app_repo" "git@10.142.0.2:mph-tech/autodev-e2e-app.git"
reset_remote_backed_repo "$app_repo"
mkdir -p "$app_repo/cmd/server" "$app_repo/internal/version" "$app_repo/docs" "$app_repo/config"
cat <<'EOF' > "$app_repo/README.md"
# Autodev E2E App

Minimal baseline application used for Autodev e2e validation.
EOF
cat <<'EOF' > "$app_repo/go.mod"
module g7.mph.tech/mph-tech/autodev-e2e-app

go 1.24
EOF
cat <<'EOF' > "$app_repo/cmd/server/main.go"
package main

import (
	"fmt"

	"g7.mph.tech/mph-tech/autodev-e2e-app/internal/version"
)

func main() {
	fmt.Println(version.String())
}
EOF
cat <<'EOF' > "$app_repo/internal/version/version.go"
package version

const Value = "stage1-baseline"

func String() string {
	return Value
}
EOF
cat <<'EOF' > "$app_repo/internal/version/version_test.go"
package version

import "testing"

func TestString(t *testing.T) {
	if String() == "" {
		t.Fatal("version string must not be empty")
	}
}
EOF
cat <<'EOF' > "$app_repo/config/default.yaml"
app:
  name: autodev-e2e-app
  version: stage1-baseline
EOF
cat <<'EOF' > "$app_repo/docs/README.md"
# E2E Fixture Docs

This repository is the known-good e2e fixture baseline for Autodev.
EOF
cat <<'EOF' > "$app_repo/docs/security.md"
# Security Notes

The e2e fixture baseline uses Semgrep's default auto configuration and should stay
free of obvious insecure patterns.
EOF
cat <<'EOF' > "$app_repo/Makefile"
test:
	go test ./...
EOF
git -C "$app_repo" add . >/dev/null
if ! git -C "$app_repo" diff --cached --quiet; then
  git -C "$app_repo" commit -m "Initialize e2e fixture app" >/dev/null
fi

init_git_repo "$gitops_repo" "git@10.142.0.2:mph-tech/autodev-e2e-gitops.git"
reset_remote_backed_repo "$gitops_repo"
for env in local dev; do
  env_dir="$gitops_repo/clusters/$env/autodev-e2e-app"
  mkdir -p "$env_dir"
  cat <<'EOF' > "$env_dir/app.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: autodev-e2e-app
spec:
  template:
    spec:
      containers:
        - name: autodev-e2e-app
          image: registry.local/autodev-e2e-app:latest
EOF
  cat <<'EOF' > "$env_dir/infrastructure.yaml"
apiVersion: infra.example/v1
kind: Infrastructure
metadata:
  name: autodev-e2e-app
EOF
  cat <<'EOF' > "$env_dir/database.yaml"
apiVersion: db.example/v1
kind: Migration
metadata:
  name: autodev-e2e-app
EOF
done
rm -rf "$gitops_repo/clusters/prod"
cat <<'EOF' > "$gitops_repo/README.md"
# Autodev E2E GitOps

Local and dev desired-state manifests for the Autodev e2e fixture app.
EOF
git -C "$gitops_repo" add . >/dev/null
if ! git -C "$gitops_repo" diff --cached --quiet; then
  git -C "$gitops_repo" commit -m "Initialize e2e fixture GitOps" >/dev/null
fi

init_git_repo "$journal_repo" "git@10.142.0.2:mph-tech/autodev-e2e-work-orders.git"
reset_remote_backed_repo "$journal_repo"
mkdir -p "$journal_repo/runs"
cat <<'EOF' > "$journal_repo/README.md"
# E2E Work Order Journal

Git-backed durable journal for e2e fixture runs.
EOF
git -C "$journal_repo" add . >/dev/null
if ! git -C "$journal_repo" diff --cached --quiet; then
  git -C "$journal_repo" commit -m "Initialize e2e work-order journal" >/dev/null
fi

printf 'Initialized e2e fixture repos under %s\n' "$root"
