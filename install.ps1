# Builds appie-cli and appie-mcp, registers the MCP server with Claude Code
# and installs the skill. Windows / PowerShell.
#
#   powershell -ExecutionPolicy Bypass -File .\install.ps1
#
# Flags:
#   -Name <name>   MCP server name in Claude Code (default: ah)
#   -ReadOnly      register the server without write access
#   -SkipSkill     do not copy the skill into ~/.claude/skills

[CmdletBinding()]
param(
    [string]$Name = 'ah',
    [switch]$ReadOnly,
    [switch]$SkipSkill
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $repo

function Say($msg) { Write-Host $msg }
function Fail($msg) { Write-Host $msg -ForegroundColor Red; exit 1 }

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Fail @"
Go is not installed.

Install it from https://go.dev/dl/ and run this script again, or download a
prebuilt binary from the Releases page of this repository.
"@
}

$target = Join-Path $env:USERPROFILE 'go\bin'
if (-not (Test-Path $target)) { New-Item -ItemType Directory -Path $target -Force | Out-Null }

Say "Building appie-cli and appie-mcp..."
go build -o "$target\" ./cmd/...
if ($LASTEXITCODE -ne 0) { Fail "Build failed." }

$mcpPath = Join-Path $target 'appie-mcp.exe'
$cliPath = Join-Path $target 'appie-cli.exe'
if (-not (Test-Path $mcpPath)) { Fail "Build reported success but $mcpPath is missing." }
Say "Installed to $target"

# Register with Claude Code. Re-running the script re-points an existing entry
# instead of failing on a duplicate name.
$claude = Get-Command claude -ErrorAction SilentlyContinue
if ($claude) {
    try { & claude mcp remove $Name -s user 2>$null | Out-Null } catch { }
    if ($ReadOnly) {
        & claude mcp add $Name --scope user --env AH_READONLY=1 -- $mcpPath
    } else {
        & claude mcp add $Name --scope user -- $mcpPath
    }
    if ($LASTEXITCODE -eq 0) {
        Say "Registered MCP server '$Name' with Claude Code."
    } else {
        Say "Could not register automatically. Run this yourself:"
        Say "  claude mcp add $Name --scope user -- `"$mcpPath`""
    }
} else {
    Say "Claude Code CLI not found on PATH. Register the server yourself:"
    Say "  claude mcp add $Name --scope user -- `"$mcpPath`""
}

# Install the skill so the agent knows how to use the tools.
if (-not $SkipSkill) {
    $skillSrc = Join-Path $repo 'skills\albert-heijn'
    $skillDst = Join-Path $env:USERPROFILE '.claude\skills\albert-heijn'
    if (Test-Path $skillSrc) {
        New-Item -ItemType Directory -Path $skillDst -Force | Out-Null
        Copy-Item "$skillSrc\*" $skillDst -Recurse -Force
        Say "Installed skill to $skillDst"
    }
}

Say @"

Done. One thing left, and only you can do it:

  $cliPath login

That opens your browser and logs you in to your own Albert Heijn account.
Check it worked with:

  $cliPath status

Then restart Claude Code so it picks up the new server.
"@
