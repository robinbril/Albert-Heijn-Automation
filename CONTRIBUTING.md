# Contributing

## Before you open a PR

```bash
go build ./...
go vet ./...
go test ./...
```

All three must pass. CI runs the same plus `govulncheck`.

## Things that will get a PR rejected

- **Anything personal.** No receipts, no order ids, no member ids, no postal codes, no absolute
  paths from your machine, no tokens. Check your diff before you push, including test fixtures.
- **A write path that skips the guard.** Mutations go through the transport in `internal/ah`, which
  fails closed when `AH_READONLY=1`. Do not add a call site that builds its own request.
- **An MCP write tool registered with `addRead`.** Writes are registered with `addWrite`, which is
  what attaches the `confirm=true` requirement. That is the whole mechanism.
- **Retrying a write.** A duplicated cart line is worse than one visible error.

## The API drifts

Allerhande's GraphQL schema changes and introspection is disabled, so queries break without warning.
When one does, `appie-cli graphql` runs raw queries and the validation errors name the type that
changed. Fix the query, and add a note to the README if the shape is surprising enough that the next
person would lose an hour to it.

## Style

Match what is there. Comments explain why something is the way it is, not what the line does. If a
function needs a paragraph to explain itself, it is probably two functions.
