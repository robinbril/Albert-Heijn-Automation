# Albert Heijn Automation

An agent that does your groceries and reads your buying habits back to you. Talks to the mobile API
of the Albert Heijn app through a CLI and an MCP server, so Claude (or any MCP client) can plan
meals, cross the bonus folder with what you actually buy, and fill your shopping list.

> Unofficial. No affiliation with Albert Heijn or Ahold Delhaize. The API is undocumented and can
> change without notice. Use at your own risk, with your own account.

Built on [appie-go](https://github.com/gwillem/appie-go) by [@gwillem](https://github.com/gwillem).

## Quickstart

Paste this into Claude Code and it will do the rest:

> Download and activate this repo, then hand me the Albert Heijn login for the one-time link:
> `https://github.com/robinbril/Albert-Heijn-Automation.git`

In Dutch, if that is how you talk to it:

> Download en activeer deze repo via deze chat, en geef me het AH-inlogscherm voor de eenmalige
> koppeling: `https://github.com/robinbril/Albert-Heijn-Automation.git`

The repository ships a `CLAUDE.md` with the setup steps, so the agent builds the binaries, registers
the MCP server, installs the skill, and then stops and hands the login back to you. Logging in is
the one step nobody can do for you.

### Or by hand

Needs Go 1.25+. Without Go, take a prebuilt binary from
[Releases](https://github.com/robinbril/Albert-Heijn-Automation/releases) and skip to `login`.

```bash
git clone https://github.com/robinbril/Albert-Heijn-Automation.git
cd Albert-Heijn-Automation

./install.sh                                        # macOS / Linux
powershell -ExecutionPolicy Bypass -File install.ps1 # Windows

appie-cli login     # opens your browser, logs in to your own AH account
appie-cli status    # check it worked
```

The installer builds both binaries, registers the MCP server with Claude Code as `ah`, and copies
the skill to `~/.claude/skills/albert-heijn`. Restart Claude Code afterwards so it sees the server.

Want a read-only agent? `./install.sh --read-only` (or `-ReadOnly` on Windows). It sets
`AH_READONLY=1`, which blocks every mutation in the HTTP transport, under every call site.

Tokens land in your user config (`%AppData%\appie\tokens.json` on Windows,
`~/.config/appie/tokens.json` elsewhere) with mode 0600, and refresh themselves. Never in this repo.

## What it does

**Groceries.** `bonus` returns the whole folder, `bonus-matches` filters it down to what you already
buy. `recipes` searches Allerhande with a cap on hands-on time, so oven and resting minutes do not
count against a weeknight. `plan` merges the ingredients of several recipes, so you buy onions once
instead of three times. Your list and your online cart can be read, extended, adjusted and emptied.

**Buying habits.** `purchases` merges online orders and in-store receipts into one timeline.
`patterns` derives your shopping rhythm, your fixed day and your repeat products. `health` reads the
basket back as an eating pattern: food groups, Nutri-Score distribution, and the D and E products
that keep returning. `swap` finds a better-scoring alternative in the same category, with the price
difference. Both describe a shopping basket. Neither is medical advice.

`appie-cli` with no arguments prints the full command list. The MCP server exposes 37 tools:
27 that only read, 10 that write.

**Not included:** paying and picking a delivery slot. There is no public API for either, so you
finish the order in the AH app. Anything that claims otherwise is lying to you.

## The skill

`skills/albert-heijn/` holds the operating manual the agent follows: how to read your history before
proposing anything, how to merge recipe ingredients, and the rule that it never touches your list
without showing you first. The installer copies it into `~/.claude/skills/`.

Next to it is `PROFILE.template.md`. Copy it to `PROFILE.md` and fill it in once: household size,
hands-on cooking time, allergies, hard rules, what you buy at the butcher instead. The fastest way
is to hold the voice-memo button, say what you like eating, and then ask the agent to check that
against your actual purchase history and write the profile for you. The gap between what people say
they eat and what their receipts say is usually the interesting part.

## Safety

Two independent locks on anything that writes.

1. Every MCP tool that changes something refuses without `confirm=true`, and that check sits in the
   registration rather than in the handlers, so a tool added later cannot forget it.
2. `AH_READONLY=1` blocks mutations in the HTTP transport, below every call site. GraphQL bodies are
   parsed for a `mutation` operation, and anything unparseable counts as a write.

Writes are never retried. A product added twice is worse than one visible error.

Your name, email, phone number, address and card numbers stay out of `member` unless you pass
`-pii`, and `deliveries` withholds the delivery address the same way. AH's own segmentation is
returned, but only the keys about food and shopping; anything else AH decides to put in that map is
reported by name and left out, because the day it starts returning a birth year should not be the
day your agent quietly learns it.

The browser login is implemented here rather than taken from the upstream library, which accepted an
authorization code on a guessable loopback path. See [SECURITY.md](SECURITY.md).

## Notes from building it

- `previouslyBought` on `productSearch` is undocumented and returns everything you have ever bought.
- `customerProfileProperties` exposes AH's own profiling: life stage, food profile, price segment,
  fixed shopping day.
- The app's list is `shoppinglist/v2`; favourite lists are `lists/v3`. Two separate collections, and
  what you put in one does not appear in the other.
- Deleting a line takes `quantity: 0`. `strikeThrough` only crosses it out and leaves it there.
- Receipts carry no product ids, only descriptions, so `health` runs on the purchase history rather
  than on receipt lines.
- One broken bonus category made `appie-go` discard the entire folder. Here it continues and reports
  what it skipped.
- GraphQL introspection is disabled and the Allerhande schema drifts. On `GRAPHQL_VALIDATION_FAILED`,
  run raw queries with `appie-cli graphql` and read the new shape out of the validation errors.

## Development

```bash
go test ./...
go vet ./...
```

`internal/ah` is the client, the transport guard and everything appie-go does not cover.
`cmd/appie-cli` and `cmd/appie-mcp` are thin layers on top.

Contributions welcome. Do not commit tokens, receipts, or anything with your own name in it.

AGPL-3.0
