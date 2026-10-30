#!/usr/bin/env bash
# P0-SP3-005: assess jezek/xgb's xgbgen on xcb-proto xinput.xml and xkb.xml.
# usage: gen.sh [strict|subset] [work-dir]
#   strict  run xgbgen on the unmodified XML (default); exits 1 when the generator rejects a file
#   subset  drop the XML elements xgbgen cannot handle (lists below), generate, then go vet for linux/amd64
# Nothing is written into the repository; work-dir defaults to ${TMPDIR:-/tmp}/edgesync-x11gen.
set -euo pipefail

xcbproto_url=https://gitlab.freedesktop.org/xorg/proto/xcbproto.git
xcbproto_tag=xcb-proto-1.17.0
xcbproto_commit=77d7fc04da729ddc5ed4aacf30253726fac24dca

xinput_drop="InputInfo ListInputDevices DeviceTimeCoord GetDeviceMotionEvents FeedbackState GetFeedbackControl \
FeedbackCtl ChangeFeedbackControl InputState QueryDeviceState DeviceState GetDeviceControl DeviceCtl \
ChangeDeviceControl ChangeDeviceProperty GetDeviceProperty HierarchyChange XIChangeHierarchy DeviceClass \
XIDeviceInfo XIQueryDevice XIChangeProperty XIGetProperty DeviceChanged KeyPress KeyRelease ButtonPress \
ButtonRelease Motion RawKeyPress RawKeyRelease RawButtonPress RawButtonRelease RawMotion TouchBegin \
TouchUpdate TouchEnd RawTouchBegin RawTouchUpdate RawTouchEnd EventForSend SendExtensionEvent"
xkb_drop="SelectEvents GetMap SetMap GetNames SetNames GetKbdByName GetCompatMap SetCompatMap GetIndicatorMap \
SetIndicatorMap GetDeviceInfo SetDeviceInfo Action Behavior SetBehavior KeyType SetKeyType DeviceLedInfo"

mode=${1:-strict}
work=${2:-${TMPDIR:-/tmp}/edgesync-x11gen}
here=$(cd "$(dirname "$0")" && pwd)
spikes=$(cd "$here/../.." && pwd)

case "$mode" in
strict | subset) ;;
*)
	echo "usage: $0 [strict|subset] [work-dir]" >&2
	exit 2
	;;
esac

PATH="$(go env GOROOT)/bin:$PATH"
mkdir -p "$work"

if [ ! -d "$work/xcbproto/.git" ]; then
	git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$xcbproto_tag" "$xcbproto_url" "$work/xcbproto"
fi
actual_commit=$(git -C "$work/xcbproto" rev-parse HEAD)
if [ "$actual_commit" != "$xcbproto_commit" ]; then
	echo "xcb-proto at $work/xcbproto is $actual_commit, expected $xcbproto_commit ($xcbproto_tag)" >&2
	exit 1
fi
proto_dir=$work/xcbproto/src

go -C "$spikes" build -o "$work/xgbgen" github.com/jezek/xgb/xgbgen

drop_elements() {
	awk -v names="$1" '
	BEGIN { n = split(names, list, " "); for (i = 1; i <= n; i++) drop[list[i]] = 1 }
	skipping {
		if ($0 ~ close_re) skipping = 0
		next
	}
	match($0, /^[ \t]+<(struct|union|request|event|eventcopy|eventstruct)[ \t]+name="[^"]+"/) {
		head = substr($0, RSTART, RLENGTH)
		name = head
		sub(/^.*name="/, "", name)
		sub(/"$/, "", name)
		if (name in drop) {
			tag = head
			sub(/^[ \t]+</, "", tag)
			sub(/[ \t].*$/, "", tag)
			indent = head
			sub(/<.*$/, "", indent)
			if ($0 !~ /\/>[ \t]*$/) {
				close_re = "^" indent "</" tag ">"
				skipping = 1
			}
			next
		}
	}
	{ print }
	' "$2" >"$3"
}

generate() {
	local xml=$1 out=$2
	if "$work/xgbgen" --proto-path "$proto_dir" "$xml" >"$out.tmp" 2>"$out.err"; then
		mv "$out.tmp" "$out"
		rm -f "$out.err"
		return 0
	fi
	rm -f "$out.tmp"
	return 1
}

if [ "$mode" = strict ]; then
	out=$work/strict
	mkdir -p "$out"
	status=0
	for ext in xinput xkb; do
		if generate "$proto_dir/$ext.xml" "$out/$ext.go"; then
			echo "OK   $ext: $(wc -l <"$out/$ext.go") lines"
		else
			echo "FAIL $ext: $(head -n 1 "$out/$ext.go.err")"
			status=1
		fi
	done
	exit "$status"
fi

out=$work/subset
rm -rf "$out"
mkdir -p "$out/xinput" "$out/xkb"
drop_elements "$xinput_drop" "$proto_dir/xinput.xml" "$out/xinput.xml"
drop_elements "$xkb_drop" "$proto_dir/xkb.xml" "$out/xkb.xml"
generate "$out/xinput.xml" "$out/xinput/xinput.go"
generate "$out/xkb.xml" "$out/xkb/xkb.go"
wc -l "$out/xinput/xinput.go" "$out/xkb/xkb.go"

xgb_version=$(go -C "$spikes" list -m -f '{{.Version}}' github.com/jezek/xgb)
(
	cd "$out"
	go mod init edgesync-x11gen-scratch
	go mod edit -require="github.com/jezek/xgb@$xgb_version"
	go mod tidy
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...
)
echo "subset generated and vetted in $out"
