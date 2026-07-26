#!/bin/bash
# Builds appie-cli and appie-mcp and puts them on your PATH.
set -euo pipefail

cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
    echo "Go is not installed. Get it from https://go.dev/dl/" >&2
    exit 1
fi

target="$HOME/go/bin"
if [ ! -d "$target" ] && [ -d "$HOME/.local/bin" ]; then
    target="$HOME/.local/bin"
fi
mkdir -p "$target"

echo "Building appie-cli and appie-mcp..."
go build -o "$target/" ./cmd/...

echo "Installed to $target"
case ":$PATH:" in
    *":$target:"*) ;;
    *) echo "Add it to your PATH: export PATH=\"$target:\$PATH\"" ;;
esac

cat <<EOF

Next steps:
  appie-cli login                    log in to Albert Heijn in your browser
  appie-cli status                   check that it worked
  claude mcp add ah -- $target/appie-mcp
EOF
