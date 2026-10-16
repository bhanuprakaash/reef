#!/bin/sh
set -eu

URL=https://github.com/bhanuprakaash/reef/releases/latest/download/Reef-mac-arm64.zip
APP=/Applications/Reef.app
NEW=/Applications/.Reef.app.new
UPDATE="$HOME/Library/Application Support/Reef/update"

main() {
  if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
    echo "Reef runs on Apple Silicon Macs only." >&2
    exit 1
  fi

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "Downloading Reef..."
  curl -fsSL "$URL" -o "$tmp/Reef.zip"
  ditto -x -k "$tmp/Reef.zip" "$tmp"

  rm -rf "$UPDATE/staged" "$UPDATE/incoming" "$UPDATE/pending"

  if pgrep -x Reef >/dev/null; then
    echo "Quitting Reef..."
    osascript -e 'quit app "Reef"'
    waited=0
    while pgrep -x Reef >/dev/null; do
      waited=$((waited + 1))
      if [ "$waited" -gt 20 ]; then
        echo "Reef did not quit. Quit it and run this again." >&2
        exit 1
      fi
      sleep 1
    done
  fi

  rm -rf "$NEW"
  ditto "$tmp/Reef.app" "$NEW"
  xattr -cr "$NEW"
  rm -rf "$APP"
  mv "$NEW" "$APP"
  open "$APP"

  echo "Installed Reef $(defaults read "$APP/Contents/Info" CFBundleShortVersionString)"
}

main "$@"
