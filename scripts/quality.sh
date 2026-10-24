#!/usr/bin/env bash
# Empty arrays also work with the Bash 3 shipped on macOS (nounset does not).
set -eo pipefail

cd "$(git rev-parse --show-toplevel)"

files=()
while IFS= read -r -d '' file; do
  [[ -f "$file" ]] && files+=("./$file")
done < <(git ls-files --cached --others --exclude-standard -z -- '*.go')

staged=()
while IFS= read -r -d '' file; do
  staged+=("$file")
done < <(git diff --cached --name-only --diff-filter=ACMRD -z)

# Keep these sections in sync with .golangci.yml. Use the standalone mise pins:
# golangci-lint embeds its own (potentially older) formatter dependencies.
gci_sections=(-s standard -s default -s 'prefix(github.com/TomaszJanusz/kitbag/)')

# gci also formats stdin whenever it is not a terminal (a pipe on CI runners),
# failing on the empty input. /dev/null is a character device, so gci skips it.
gci() {
  command gci "$@" </dev/null
}

format_go() {
  gofumpt -w "$@"
  gci write "${gci_sections[@]}" "$@"
}

case "${1:-}" in
format | format-check)
  if ((${#files[@]})); then
    if [[ "$1" == format ]]; then
      format_go "${files[@]}"
    else
      # Diff tools may exit zero with output; require empty diffs and success.
      status=0
      gofumpt_diff=$(gofumpt -d "${files[@]}") || status=$?
      gci_diff=$(gci diff "${gci_sections[@]}" "${files[@]}") || status=$?
      if [[ -n "$gofumpt_diff$gci_diff" || "$status" != 0 ]]; then
        printf '%s\n%s\n' "$gofumpt_diff" "$gci_diff"
        exit 1
      fi
    fi
  fi
  ;;
staged-format)
  files=()
  for file in "${staged[@]}"; do
    [[ "$file" == *.go && -f "$file" ]] && files+=("./$file")
  done
  ((${#files[@]} == 0)) || format_go "${files[@]}"
  ;;
staged-lint)
  packages=()
  for file in "${staged[@]}"; do
    case "$file" in
    go.mod | go.sum | .golangci.yml | mise.toml | scripts/quality.sh)
      golangci-lint config verify
      exec golangci-lint run ./...
      ;;
    *.go)
      dir=$(dirname -- "$file")
      # Deleted packages have nothing to lint; surviving files still matter.
      if compgen -G "$dir/*.go" >/dev/null; then
        found=false
        for package in "${packages[@]}"; do
          [[ "$package" == "./$dir" ]] && found=true
        done
        $found || packages+=("./$dir")
      fi
      ;;
    esac
  done
  if ((${#packages[@]})); then
    golangci-lint config verify
    golangci-lint run "${packages[@]}"
  fi
  ;;
staged-typos)
  files=()
  for file in "${staged[@]}"; do
    [[ -f "$file" ]] && files+=("./$file")
  done
  ((${#files[@]} == 0)) || typos -- "${files[@]}"
  ;;
*)
  printf 'usage: %s {format|format-check|staged-format|staged-lint|staged-typos}\n' "$0" >&2
  exit 2
  ;;
esac
