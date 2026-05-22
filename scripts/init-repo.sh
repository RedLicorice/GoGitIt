#!/usr/bin/env bash
# Push the scaffold to your GoGitIt repository.
# Usage:
#   1. Unzip gogitit-scaffold.zip
#   2. cd gogitit
#   3. bash scripts/init-repo.sh
#
# This script:
#   - initializes git (if needed)
#   - sets the remote to your GitHub repo
#   - creates an initial commit
#   - pushes to main
set -euo pipefail

REMOTE_URL="${REMOTE_URL:-https://github.com/RedLicorice/GoGitIt.git}"
BRANCH="${BRANCH:-main}"

if [ ! -d .git ]; then
  echo "→ git init"
  git init -b "$BRANCH"
fi

if ! git remote get-url origin >/dev/null 2>&1; then
  echo "→ git remote add origin $REMOTE_URL"
  git remote add origin "$REMOTE_URL"
else
  echo "→ origin already set: $(git remote get-url origin)"
fi

echo "→ git add ."
git add .

if git diff --cached --quiet; then
  echo "Nothing to commit."
else
  echo "→ initial commit"
  git commit -m "chore: initial scaffold

- Go backend (chi + go-git + viper)
- OIDC auth (Keycloak), togglable
- Svelte + Vite + Tailwind frontend
- Docker + Traefik deployment for gogitit.example.com"
fi

echo "→ git push origin $BRANCH"
git push -u origin "$BRANCH"

echo "✓ Done."
