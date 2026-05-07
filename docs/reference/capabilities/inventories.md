# Inventory Capabilities

Inventory capabilities resolve values before an action runs.

Source of truth:

- `go run ./cmd/theater explain inventory`
- `go run ./cmd/theater explain inventory <inventory-ref>`

## Checked Inventory Catalog

<!-- theater-doc: command id=reference-inventory-family cwd=../../.. expect-stdout="Capabilities (5):" expect-stdout-2="inventory.env" expect-stdout-3="inventory.state.record" -->
```sh
go run ./cmd/theater explain inventory
```

## Built-In Inventories

| Ref | Produces | Purpose |
| --- | --- | --- |
| `inventory.env` | `string` | Read a single environment variable |
| `inventory.file` | `bytes` | Read a file as raw bytes |
| `inventory.http.get` | `bytes` | Fetch a remote HTTP resource body with GET |
| `inventory.state.record` | opaque record handle | Build a persistent-state record handle |
| `inventory.state.pool` | opaque pool handle | Build a persistent-state fixture-pool handle |

Inventories populate properties before the action runs. They do not publish
current-action output fields, so do not select them with `field(...)`. In
Theater DSL, use the property name as a ref such as `$token`; in YAML, use
`kind: ref` with the property name.

State inventory inputs `backend`, `record` or `pool`, and `min_guarantee` are
literal-only in v1.

For HTTP fetch details, open [HTTP Capabilities](http.md). For state handles,
open [State Capabilities](state.md).

For procedures, use [HTTP Sessions](../../how-to/use-http-sessions.md) and
[Persistent State](../../how-to/use-persistent-state.md).
