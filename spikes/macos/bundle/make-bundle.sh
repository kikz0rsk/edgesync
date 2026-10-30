#!/usr/bin/env bash
# Builds spikes/out/EdgeSyncSpike.app (or the directory given as $1) on macOS: arm64 and amd64 binaries joined with
# lipo, a minimal Contents/MacOS + Contents/Info.plist layout, and an ad-hoc signature (D-106).
set -euo pipefail

bundle_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
spikes_dir="$(cd "${bundle_dir}/../.." && pwd)"
out_dir="${1:-${spikes_dir}/out}"
app="${out_dir}/EdgeSyncSpike.app"
executable="trayoverlay"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

for arch in arm64 amd64; do
	CGO_ENABLED=0 GOOS=darwin GOARCH="${arch}" go -C "${spikes_dir}" build -trimpath \
		-o "${work}/${executable}-${arch}" ./macos/cmd/trayoverlay
done

rm -rf "${app}"
mkdir -p "${app}/Contents/MacOS"
lipo -create -output "${app}/Contents/MacOS/${executable}" "${work}/${executable}-arm64" "${work}/${executable}-amd64"
cp "${bundle_dir}/Info.plist" "${app}/Contents/Info.plist"

plutil -lint "${app}/Contents/Info.plist"
codesign -s - --force "${app}"
codesign --verify --verbose=2 "${app}"
lipo -archs "${app}/Contents/MacOS/${executable}"
echo "built ${app}"
