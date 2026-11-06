#!/bin/sh
set -eu

# REEF_SOURCE=github switches back to GitHub releases. A private GitLab project needs GITLAB_TOKEN.
case "${REEF_SOURCE:-gitlab}" in
  gitlab) URL=https://gitlab.com/bhanuprakashsaip/reef/-/releases/permalink/latest/downloads/Reef-mac-arm64.zip ;;
  github) URL=https://github.com/bhanuprakaash/reef/releases/latest/download/Reef-mac-arm64.zip ;;
  *) echo "REEF_SOURCE must be gitlab or github." >&2; exit 1 ;;
esac
APP=/Applications/Reef.app
NEW=/Applications/.Reef.app.new
UPDATE="$HOME/Library/Application Support/reef-desktop/update"

main() {
  if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
    echo "Reef runs on Apple Silicon Macs only." >&2
    exit 1
  fi

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "Downloading Reef..."
  if [ -n "${GITLAB_TOKEN:-}" ]; then
    curl -fsSL -H "PRIVATE-TOKEN: $GITLAB_TOKEN" "$URL" -o "$tmp/Reef.zip"
  else
    curl -fsSL "$URL" -o "$tmp/Reef.zip"
  fi
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
