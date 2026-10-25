#!/usr/bin/env bash
# Invoked only by the trusted release workflow, after SLSA generation succeeds.
set -euo pipefail

bash scripts/release-verify.sh dist
cd dist
identity="https://github.com/${GITHUB_REPOSITORY}/.github/workflows/release.yml@${GITHUB_WORKFLOW_REF##*@}"
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity "$identity" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt

source=(--source-uri "github.com/$GITHUB_REPOSITORY")
if [[ "$GITHUB_REF" == refs/tags/* ]]; then
  source+=(--source-tag "$TAG")
else
  test "$GITHUB_REF" = refs/heads/main
  source+=(--source-branch main)
fi
for artifact in ./*.tar.gz ./*.deb ./*.rpm ./*.sbom.json install.sh kitbag-update.json checksums.txt checksums.txt.sigstore.json; do
  slsa-verifier verify-artifact "$artifact" --provenance-path kitbag.intoto.jsonl "${source[@]}" --print-provenance |
    jq -e --arg sha "$SOURCE_SHA" '.predicate.invocation.configSource.digest.sha1 == $sha' >/dev/null
done

if [[ "$NIGHTLY" == true ]]; then
  test "$TAG" = nightly
  jq -e '.channel == "nightly" and (.version | test("^[0-9]+\\.[0-9]+\\.[0-9]+-nightly\\."))' kitbag-update.json >/dev/null
  # List first so an API/network error cannot be mistaken for a missing release.
  existing=$(gh api --paginate "repos/$GH_REPO/releases" --jq '.[] | select(.tag_name == "nightly") | .id')
  if [[ -n "$existing" ]]; then
    gh release view nightly --json isPrerelease --jq .isPrerelease | grep -Fx true
    gh release edit nightly --prerelease --latest=false --title "Nightly $(jq -r .version kitbag-update.json)"
  else
    gh release create nightly --target "$SOURCE_SHA" --prerelease --latest=false \
      --title "Nightly $(jq -r .version kitbag-update.json)" \
      --notes 'Manual nightly channel. Verify checksums, keyless identity and SLSA provenance before installation. Self-update keyless support is pending; see docs/release.md.'
  fi
  # Only the mutable nightly alias moves. Stable version tags are never edited.
  gh api --method PATCH "repos/$GH_REPO/git/refs/tags/nightly" -f sha="$SOURCE_SHA" -F force=true >/dev/null
  gh release upload nightly ./*.tar.gz ./*.deb ./*.rpm ./*.sbom.json install.sh kitbag-update.json checksums.txt checksums.txt.sigstore.json kitbag.intoto.jsonl --clobber
else
  [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
  jq -e --arg version "${TAG#v}" '.channel == "stable" and .version == $version' kitbag-update.json >/dev/null
  existing=$(gh api --paginate "repos/$GH_REPO/releases" --jq ".[] | select(.tag_name == \"$TAG\") | .id")
  if [[ -z "$existing" ]]; then
    gh release create "$TAG" --verify-tag --latest --generate-notes
  fi
  gh release upload "$TAG" ./*.tar.gz ./*.deb ./*.rpm ./*.sbom.json install.sh kitbag-update.json checksums.txt checksums.txt.sigstore.json kitbag.intoto.jsonl
fi
