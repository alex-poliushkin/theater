# Capabilities

Capabilities are the named runtime contracts used by actions, inventories,
decorators, generators, matchers, state backends, and plugin-provided
extensions.

Source of truth:

- `go run ./cmd/theater explain`
- Built-in descriptors registered by the runtime catalogs
- Plugin descriptors loaded by `--plugins-config`

## Checked Catalog Surface

<!-- theater-doc: command id=reference-capabilities-explain cwd=../../.. expect-stdout="Capability families:" expect-stdout-2="action           9" expect-stdout-3="matcher          17" expect-stdout-4="state-backend    1" -->
```sh
go run ./cmd/theater explain
```

<!-- theater-doc: command id=reference-capabilities-explain-http cwd=../../.. expect-stdout="Matches for \"http\":" expect-stdout-2="action.http" expect-stdout-3="theater explain action http" expect-stdout-4="theater explain inventory http.get" -->
```sh
go run ./cmd/theater explain http
```

## Families

| Family | Built-in count | Reference |
| --- | --- | --- |
| `action` | 9 | [Actions](actions.md) |
| `inventory` | 5 | [Inventories](inventories.md) |
| `transform` | 2 | [Decorators And Transforms](decorators.md) |
| `generator` | 8 | [Generators](generators.md) |
| `matcher` | 17 | [Matchers](matchers.md) |
| `state-backend` | 1 | [State](state.md) |
| `report-exporter` | 0 built in | Plugin-provided only |

Use `theater explain <family>` to list one family,
`theater explain <family> <ref>` to inspect a single descriptor, and
`theater explain <query>` to list matching descriptors across families when the
single target is not a family or built-in topic.

HTTP-specific action, inventory, session, auth, and identity references are
grouped in [HTTP Capabilities](http.md).

For procedures, use [Check JSON Response Fields](../../how-to/check-json-response-fields.md),
[HTTP Sessions](../../how-to/use-http-sessions.md), and
[Persistent State](../../how-to/use-persistent-state.md).
