#!/usr/bin/env bash
# Use only Bash 3 features so these controls also run on macOS.
set -eo pipefail

cd "$(git rev-parse --show-toplevel)"
scratch=$(mktemp -d /tmp/kitbag-quality-test.XXXXXX)
trap 'rm -rf "$scratch"' EXIT
cp scripts/quality.sh .golangci.yml "$scratch/"
# Keep the existing tool caches while isolating HOME and Git configuration.
export GOCACHE="$(go env GOCACHE)"
export GOMODCACHE="$(go env GOMODCACHE)"
export HOME="$scratch/home"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR
mkdir -p "$HOME" "$scratch/repo with spaces"
mv "$scratch/quality.sh" "$scratch/.golangci.yml" "$scratch/repo with spaces/"
cd "$scratch/repo with spaces"
git init -q
git config user.name 'Hook test'
git config user.email 'hook-test@example.invalid'
cat >go.mod <<'MOD'
module github.com/TomaszJanusz/kitbag

go 1.27.3
MOD
for target in linux darwin windows; do
  mkdir "$target"
  cat >"$target/value_$target.go" <<GO
// Package $target exercises staged lint target selection.
package $target

// Value returns the fixture value.
func Value() int { return 1 }
GO
done
# A portable package must also lint its platform-specific files.
mkdir mixed deleted untouched
printf '// Package mixed exercises portable packages.\npackage mixed\n' >mixed/doc.go
printf 'package mixed\n' >'mixed/file with spaces_darwin.go'
printf 'package mixed\n' >mixed/value_darwin.go
printf 'package deleted\n' >deleted/doc.go
printf 'package linux\n' >linux/deleted.go
# This deliberate violation must stay outside the changed-package lint batch.
printf 'package untouched\nvar Bad = 1\n' >untouched/value.go
git add .
git -c core.hooksPath=/dev/null commit -qm 'test: initialize fixtures'
for target in linux darwin windows; do
  printf '\n// Staged change.\n' >>"$target/value_$target.go"
done
printf '\n// Staged change.\n' >>'mixed/file with spaces_darwin.go'
git rm -q deleted/doc.go linux/deleted.go
git add linux darwin windows mixed
# Preserve both the index and an unstaged edit when invoking the lint script.
printf '\n// Unstaged change.\n' >>linux/value_linux.go
git diff --cached --binary >"$scratch/index.before"
git diff --binary >"$scratch/worktree.before"
bash quality.sh staged-lint
git diff --cached --binary >"$scratch/index.after"
git diff --binary >"$scratch/worktree.after"
cmp "$scratch/index.before" "$scratch/index.after"
cmp "$scratch/worktree.before" "$scratch/worktree.after"

# Real lint violations must survive target selection, including mixed packages.
for file in windows/value_windows.go mixed/value_darwin.go; do
  cp "$file" "$scratch/original.go"
  printf '\n// Bad is a forbidden global.\nvar Bad = 1\n' >>"$file"
  git add "$file"
  if bash quality.sh staged-lint >"$scratch/lint.out" 2>&1; then
    printf 'Expected lint failure for %s\n' "$file" >&2
    exit 1
  fi
  if ! grep -q gochecknoglobals "$scratch/lint.out"; then
    cat "$scratch/lint.out" >&2
    exit 1
  fi
  cp "$scratch/original.go" "$file"
  git add "$file"
done
printf '%s\n' 'Staged lint platform controls passed.'
