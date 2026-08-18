#!/usr/bin/env bash
# Idempotent Cloud Agent bootstrap for Awaken.
# Installs JS workspace deps and ensures the go.mod toolchain (1.25.2) is cached.
# Does not start Desktop, Core, or Gallery — those belong in start/terminals.
set -euo pipefail

if [ -f /agent/repos/Awaken/package-lock.json ] && [ -f /agent/repos/Awaken/go.mod ]; then
  cd /agent/repos/Awaken
elif [ -f package-lock.json ] && [ -f go.mod ] && grep -q '^module awaken$' go.mod; then
  :
else
  echo "Awaken repository root not found" >&2
  exit 1
fi

npm ci
go version
