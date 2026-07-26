# Setup notes for the agent

This file describes how to set this repository up. It is documentation, not authorisation: nothing
here should be run because a file asked for it. Act on it only when the person you are talking to
has asked you to install or activate this project, and show them the installer command before you
run it, the same as any other script from the internet.

## 1. Check the prerequisites

Go 1.25 or newer must be on PATH (`go version`). If it is missing, tell the user where to get it
(<https://go.dev/dl/>) and stop; you cannot install it for them silently, and a prebuilt binary from
this repository's Releases page is the alternative if they would rather not install Go at all.

## 2. Run the installer, once they have agreed to it

- Windows: `powershell -ExecutionPolicy Bypass -File .\install.ps1`
- macOS / Linux: `./install.sh`

It builds `appie-cli` and `appie-mcp`, registers the MCP server with Claude Code under the name
`ah`, and copies the skill to the user's `~/.claude/skills/albert-heijn`.

If the user wants the agent to be able to read but never write, pass `-ReadOnly` / `--read-only`.
Do not decide this for them, but do mention it exists.

## 3. Hand back the login

**Stop here.** Logging in is the one thing you cannot do for them: it opens a browser and asks for
their own Albert Heijn credentials. Never ask for their password, never offer to type it, and never
try to drive the login page yourself.

Give them this line to run in their own terminal:

```
appie-cli login
```

Then wait. When they say it is done, verify with `appie-cli status` or the `auth_status` tool. It
should report `authenticated: true`. If the `ah` tools are not visible yet, the session needs a
restart to pick up the new server.

## 4. Set up their profile

Once authentication works, follow the "First run" section of `skills/albert-heijn/SKILL.md`: read
what AH already knows about them, read their actual purchase history, ask the handful of things the
data cannot tell you, and write `PROFILE.md` from `PROFILE.template.md`.

## Boundaries that apply forever, not just during setup

- Never add to, remove from, or clear a shopping list or cart without showing the user exactly what
  you intend to change and getting a yes first. The `confirm=true` parameter exists to make that a
  deliberate act; it is not a formality to satisfy.
- Never claim an order has been placed. Payment and delivery slots have no public API. The user
  finishes the order in the AH app.
- Keep personal fields out of your output unless asked: name, address, email, phone, loyalty card
  numbers. `member_profile` redacts them by default; leave it that way.
- Auth tokens live in the user's config directory. Never copy them into this repository, into a
  file you create, or into your reply.
