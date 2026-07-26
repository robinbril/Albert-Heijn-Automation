# Security

## Reporting

Report anything you find through GitHub's private vulnerability reporting on this repository
(Security → Report a vulnerability). Please do not open a public issue for something exploitable.

Include what you did, what happened, and what you expected. A proof of concept helps. Test against
your own Albert Heijn account only.

## What this software touches

- **Your AH session.** Login runs OAuth in your browser. The resulting tokens are written to your
  user config directory (`%AppData%\appie\tokens.json` on Windows, `~/.config/appie/tokens.json`
  elsewhere) with mode 0600, and refreshed automatically. They are not stored in this repository and
  are never printed by any command.

  The login flow is implemented in `internal/ah/login.go` rather than taken from the upstream
  library, because the upstream one accepts an authorization code at a fixed `/callback` path on a
  loopback port with nothing tying that callback to the session that started it. Any page open in
  another tab could scan the loopback ports and hand in its own code, leaving you logged in to
  somebody else's account without noticing. Here the callback path carries 256 bits from
  `crypto/rand`, an OAuth `state` is sent and checked when the provider echoes it back, and both the
  callback and the proxy refuse requests that fetch metadata marks as cross-site.

  The flow does proxy the AH login page over plain HTTP on loopback, which is what makes the custom
  `appie://` redirect reachable from a browser. `Content-Security-Policy` and
  `Strict-Transport-Security` are dropped for that reason (an HSTS pin on `127.0.0.1` would break
  every other local server you run). Cookies keep `SameSite=Lax`, because loopback cookies are
  scoped by host and not by port. The proxy is only alive while a login is in progress and times out
  after ten minutes.
- **Your shopping list and cart.** These are real. A bug here costs money or ruins dinner.
- **Your purchase history**, which is a detailed record of a household. Treat exported output the
  same way you would treat a bank statement.

## Two locks on writes

1. Every MCP tool that mutates something rejects the call unless `confirm=true`. The check lives in
   the tool registration, not in the individual handlers, so a tool added later inherits it.
2. Setting `AH_READONLY=1` blocks mutations in the HTTP transport itself, below every call site.
   GraphQL request bodies are parsed for a `mutation` operation; a body that cannot be parsed is
   treated as a write and blocked. It fails closed.

Failed writes are never retried, because a silently duplicated cart line is worse than one visible
error.

Run the agent with `AH_READONLY=1` if you want it to plan and advise but never touch anything. The
installers take `--read-only` / `-ReadOnly` for exactly this.

## If you use this with an LLM

The model sees whatever the tools return, and product names, list items and recipe text are strings
that other people control. Treat them as data, never as instructions. Do not wire the write tools
into an unattended loop: `confirm=true` is meant to be a human deciding, and it stops being a
safeguard the moment an agent sets it on its own.

## Scope

Unofficial client for an undocumented API. No affiliation with Albert Heijn or Ahold Delhaize. It
authenticates as you, so use it with your own account, at a sane request rate, for your own
household.
