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
    # Batch only changed packages for each supported OS. Checking every target
    # also covers OS-specific files in packages that build on the host OS.
    # The linter's package cache can reuse findings across GOOS values. Keep
    # target caches separate so one target cannot hide another target's issues.
    lint_cache=${GOLANGCI_LINT_CACHE:-$(go env GOCACHE)/kitbag-golangci-lint}
    for target in linux darwin windows; do
      target_packages=()
      for package in "${packages[@]}"; do
        # -e exposes only this package's loading error; dependency and compiler
        # errors must still reach the linter rather than being silently skipped.
        result=$(GOOS="$target" go list -e -f '{{if .Error}}{{.Error.Err}}{{else}}ok{{end}}' "$package")
        case "$result" in
        'build constraints exclude all Go files in '* | 'no Go files in '*) continue ;;
        ok) target_packages+=("$package") ;;
        *)
          printf '%s: %s: %s\n' "$target" "$package" "$result" >&2
          exit 1
          ;;
        esac
      done
      if ((${#target_packages[@]})); then
        GOOS="$target" GOLANGCI_LINT_CACHE="$lint_cache/staged-$target" \
          golangci-lint run "${target_packages[@]}"
      fi
    done
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
