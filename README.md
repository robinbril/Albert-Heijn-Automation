# Albert Heijn agent

> **Unofficial.** Not affiliated with, endorsed by, or connected to Albert Heijn or Ahold Delhaize.
> It speaks the same mobile API the Appie app uses, which is undocumented and can change without
> notice. Use at your own risk.

Turns an AI assistant into a grocery agent for [Albert Heijn](https://ah.nl): it reads what you
actually buy, matches this week's bonus folder against your habits, finds recipes, and manages your
shopping list and cart.

Ships as two binaries on one shared core:

| Binary | What it is |
|---|---|
| `appie-cli` | Command line client. Every command prints JSON. |
| `appie-mcp` | MCP server, so Claude Code or any MCP client can use the same 35 tools. |

Built on [appie-go](https://github.com/gwillem/appie-go) by [@gwillem](https://github.com/gwillem).

## Install

Requires [Go 1.24+](https://go.dev/dl/).

```bash
go build -o bin/ ./cmd/...
```

Then log in once. This opens your browser, and the tokens are stored in your user config directory
(`%AppData%\appie\tokens.json` on Windows, `~/.config/appie/tokens.json` elsewhere) with mode 0600.

```bash
./bin/appie-cli login
```

Tokens refresh themselves after that. Check the state at any time:

```bash
./bin/appie-cli status
```

## Connect the MCP server

```bash
claude mcp add ah -- /absolute/path/to/bin/appie-mcp
```

To let the assistant look but never touch, add `--env AH_READONLY=1`.

## What it can do

**Your history** — `purchases` merges online orders and in-store receipts into one timeline.
`purchase` reads a single one line by line with prices. `ordered-items` ranks what you buy most.
`spend` totals it per month. `patterns` derives your shopping cadence, favourite shopping day,
category mix and which products are staples rather than one-offs.

**Deals** — `bonus` is the whole bonus folder, `bonus-matches` narrows it to promotions on things
you already buy, and `bargains` finds the marked-down, about-to-expire items in a specific store.

**Recipes** — `recipes` searches Allerhande with a cap on hands-on time, counting neither oven nor
waiting time since you are not standing there for those. `recipe` reads one in full, and `plan`
merges several into a single deduplicated ingredient list so shared onions are bought once.

**Shopping list and cart** — read with prices and totals, add products or free-text notes, change
quantities, cross items off, delete lines, and copy the list into the cart.

Run `appie-cli` with no arguments for the full command list.

## Safety

Writes are gated in two independent places.

1. **The MCP confirm flag.** Every mutating tool refuses to run unless the caller passes
   `confirm=true`. The check is applied when the tool is registered, not inside each handler, so a
   tool added later cannot forget it.
2. **`AH_READONLY=1`.** This blocks mutating requests in the HTTP transport, below every call site.
   GraphQL bodies are inspected for a `mutation` operation, and anything unparseable is treated as a
   write, so the guard fails closed.

Writes are also never retried. A duplicated add-to-list is worse than one failure you can see.

Your name, email, phone, address and loyalty card numbers are withheld from `member` output unless
you pass `-pii` (`include_personal_details` in MCP), because that data otherwise lands in an
assistant's context for no benefit. The segmentation AH derived about you is always shown.

## Notes on the API

- `previouslyBought` on `productSearch` is undocumented and returns everything you have ever bought.
- `customerProfileProperties` exposes AH's own profiling: life stage, food profile, price segment,
  favourite shopping day.
- The shopping list in the app is `shoppinglist/v2`, which is a different collection from the
  favourite lists in `lists/v3`. Items added to one do not appear in the other.
- Deleting a list item means sending `quantity: 0`. Setting `strikeThrough` only crosses it off.
- Paying and picking a delivery slot have no public API. Finish an order in the AH app.
- GraphQL introspection is disabled, and Allerhande's schema drifts. When a recipe query starts
  returning `GRAPHQL_VALIDATION_FAILED`, `appie-cli graphql` runs raw queries so you can find the
  new shape from the validation errors.

## Development

```bash
go test ./...
go vet ./...
```

`internal/ah` holds the client, transport guard and the endpoints appie-go does not cover;
`cmd/appie-cli` and `cmd/appie-mcp` are thin layers over it.

## License

AGPL-3.0
