#!/usr/bin/env bash
# Builds appie-cli and appie-mcp, registers the MCP server with Claude Code
# and installs the skill. macOS / Linux.
#
#   ./install.sh
#
# Flags:
#   --name <name>   MCP server name in Claude Code (default: ah)
#   --read-only     register the server without write access
#   --skip-skill    do not copy the skill into ~/.claude/skills
set -euo pipefail

name="ah"
readonly_mode=0
skip_skill=0

while [ $# -gt 0 ]; do
    case "$1" in
        --name) name="$2"; shift 2 ;;
        --read-only) readonly_mode=1; shift ;;
        --skip-skill) skip_skill=1; shift ;;
        -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

repo="$(cd "$(dirname "$0")" && pwd)"
cd "$repo"

if ! command -v go >/dev/null 2>&1; then
    cat >&2 <<'EOF'
Go is not installed.

Install it from https://go.dev/dl/ and run this script again, or download a
prebuilt binary from the Releases page of this repository.
EOF
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
    *) echo "Not on your PATH yet. Add it: export PATH=\"$target:\$PATH\"" ;;
esac

# Register with Claude Code. Re-running the script re-points an existing entry
# instead of failing on a duplicate name.
if command -v claude >/dev/null 2>&1; then
    claude mcp remove "$name" -s user >/dev/null 2>&1 || true
    if [ "$readonly_mode" -eq 1 ]; then
        claude mcp add "$name" --scope user --env AH_READONLY=1 -- "$target/appie-mcp"
    else
        claude mcp add "$name" --scope user -- "$target/appie-mcp"
    fi
    echo "Registered MCP server '$name' with Claude Code."
else
    echo "Claude Code CLI not found on PATH. Register the server yourself:"
    echo "  claude mcp add $name --scope user -- $target/appie-mcp"
fi

# Install the skill so the agent knows how to use the tools.
if [ "$skip_skill" -eq 0 ] && [ -d "$repo/skills/albert-heijn" ]; then
    dst="$HOME/.claude/skills/albert-heijn"
    mkdir -p "$dst"
    cp -R "$repo/skills/albert-heijn/." "$dst/"
    echo "Installed skill to $dst"
fi

cat <<EOF

Done. One thing left, and only you can do it:

  $target/appie-cli login

That opens your browser and logs you in to your own Albert Heijn account.
Check it worked with:

  $target/appie-cli status

Then restart Claude Code so it picks up the new server.
EOF
