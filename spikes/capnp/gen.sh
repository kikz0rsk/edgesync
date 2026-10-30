#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
capnp="${CAPNP:-capnp}"

cd "$repo"
exe="$(go env GOEXE)"
go build -modfile=tools/tools.mod -o "bin/capnpc-go$exe" capnproto.org/go/capnp/v3/capnpc-go
std_dir="$(go list -modfile=tools/tools.mod -m -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std"
export PATH="$repo/bin:$PATH"

cd "$here"
rm -rf gen
mkdir gen
"$capnp" compile --no-standard-import -I"$std_dir" -ogo:gen --src-prefix=schema schema/*.capnp
