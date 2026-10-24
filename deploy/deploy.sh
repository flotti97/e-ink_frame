#!/bin/sh
# Builds the Frame for the Pi Zero W and installs or updates it as a
# systemd service.
#
#	deploy/deploy.sh [pi@<pi-address>]
#
# Keeps the Pi's frame.json and Image library; a first install gets a
# frame.json to fill in with the panel's VCOM.
set -eu
host=${1:-pi@raspberrypi.local}
cd "$(dirname "$0")/.."

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 go build -o "$out/frame" ./cmd/frame

ssh "$host" mkdir -p frame
# The running binary can't be overwritten in place, so it is swapped in.
scp "$out/frame" "$host:frame/frame.new"
scp deploy/frame.service "$host:frame/"
ssh "$host" sh -s <<'REMOTE'
set -eu
cd frame
mv frame.new frame
mv frame.service /tmp/frame.service
sudo install -m 644 /tmp/frame.service /etc/systemd/system/frame.service
rm /tmp/frame.service
sudo systemctl daemon-reload
if [ ! -f frame.json ]; then
	echo '{"vcom": 0}' > frame.json
	echo "Set \"vcom\" in ~/frame/frame.json to the value on the panel's flex cable, then: sudo systemctl enable --now frame"
	exit 0
fi
sudo systemctl enable frame
sudo systemctl restart frame
systemctl --no-pager status frame | head -5
REMOTE
