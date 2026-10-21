#!/bin/bash
# Local signed build: Node downloads use HTTP_PROXY; codesign uses direct network.
set -euo pipefail
cd "$(dirname "$0")/../.."
[[ "$(uname -s)" == Darwin ]] || { echo 'This build requires macOS.' >&2; exit 1; }
: "${CSC_NAME:?Set CSC_NAME}"
: "${AWAKEN_CODESIGN_IDENTITY:?Set AWAKEN_CODESIGN_IDENTITY}"
: "${APPLE_API_KEY:?Set APPLE_API_KEY}"
: "${APPLE_API_KEY_ID:?Set APPLE_API_KEY_ID}"
: "${APPLE_API_ISSUER:?Set APPLE_API_ISSUER}"
test -f "$APPLE_API_KEY"

# Only toggle enabled manual proxies; preserve server, port and credentials.
services=()
kinds=()
restore_proxies() {
  local i failed=0
  for ((i=0; i<${#services[@]}; i++)); do
    networksetup "-set${kinds[$i]}state" "${services[$i]}" on || failed=1
  done
  if [[ "$failed" != 0 ]]; then
    echo 'Proxy restoration failed; restore the original settings manually.' >&2
    return 1
  fi
}
trap restore_proxies EXIT
while IFS= read -r service; do
  [[ "$service" == \** ]] && continue
  for kind in webproxy securewebproxy socksfirewallproxy; do
    if networksetup "-get$kind" "$service" | grep -q '^Enabled: Yes'; then
      services+=("$service")
      kinds+=("$kind")
      networksetup "-set${kind}state" "$service" off
    fi
  done
done < <(networksetup -listallnetworkservices | tail -n +2)

npm run build:ui
npm --workspace @awaken/desktop run dist:release

version=$(node -p "require('./apps/desktop/package.json').version")
dist="$PWD/apps/desktop/dist-electron"
node scripts/release/verify-update-artifacts.mjs --version "$version" --dir "$dist" --receipt "$dist/update-artifacts-receipt.json"
for arch in arm64 x64; do
  dmg="$dist/Awaken-$version-$arch.dmg"
  codesign --verify --strict "$dmg"
  xcrun stapler validate "$dmg"
  spctl -a -vv -t open --context context:primary-signature "$dmg"
  hdiutil verify "$dmg"
  shasum -a 256 "$dmg" "$dist/Awaken-$version-$arch.zip"
done
echo "Signed artifacts: $dist"
echo 'Run package-content audit and installation smoke before public distribution.'
