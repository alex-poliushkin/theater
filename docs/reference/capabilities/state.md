# State Capabilities

State capabilities provide persistent records and fixture pools for runs that
need observable changes across acts or retries.

Source of truth:

- `go run ./cmd/theater explain state-backend`
- `go run ./cmd/theater explain state-backend file`
- [YAML stage schema](../yaml/stage-schema.md)

## Checked State Descriptors

<!-- theater-doc: command id=reference-state-family cwd=../../.. expect-stdout="Capabilities (1):" expect-stdout-2="state.backend.file" -->
```sh
go run ./cmd/theater explain state-backend
```

<!-- theater-doc: command id=reference-state-file-backend cwd=../../.. expect-stdout="Capability: state.backend.file" expect-stdout-2="root  string; required" expect-stdout-3="guarantee: local-atomic" -->
```sh
go run ./cmd/theater explain state-backend file
```

## Backend

| Ref | Params | Guarantee | Operations |
| --- | --- | --- | --- |
| `state.backend.file` | `root` required string | `local-atomic` | `cas`, `claim`, `renew`, `release`, `consume` |

## Handles And Actions

| Ref | Purpose | Output fields |
| --- | --- | --- |
| `inventory.state.record` | Build a persistent record handle | Produces a property value, not an action output field |
| `inventory.state.pool` | Build a fixture-pool handle | Produces a property value, not an action output field |
| `action.state.read` | Read record `value` and `version` | `value`, `version` |
| `action.state.update` | Compare-and-set record update with `expected_version` | `value`, `version` |
| `action.state.claim` | Claim a pool item with a lease | `item`, `claim` |
| `action.state.renew` | Renew a claim lease | `claim` |
| `action.state.release` | Release a claim | none |
| `action.state.consume` | Consume a claim, optionally with a tombstone | none |

## State Action Output Fields

These are the `field(...)` names available after the matching state action.

| Action | Field | Kind | Meaning |
| --- | --- | --- | --- |
| `action.state.read` | `value` | object | Current record value. |
| `action.state.read` | `version` | string | Current record version returned by the backend. |
| `action.state.update` | `value` | object | Stored record value after the compare-and-set update succeeds. |
| `action.state.update` | `version` | string | New record version returned by the backend. |
| `action.state.claim` | `item` | object | Claimed pool item. |
| `action.state.claim` | `claim` | any | Opaque claim handle for later `renew`, `release`, or `consume` actions. |
| `action.state.renew` | `claim` | any | Renewed opaque claim handle for later state actions. |
| `action.state.release` | none | - | This action publishes no output fields. |
| `action.state.consume` | none | - | This action publishes no output fields. |

Mutating state actions are invalid inside an `eventually` act.

For task flow, use [Persistent State](../../how-to/use-persistent-state.md).
