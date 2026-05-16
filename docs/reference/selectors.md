# Selectors

Selectors choose one value before an expectation, export, or ref binding uses
it.

The common shape is:

- choose a root: current action output with `field`, a ref binding with
  `ref.name`, or a property subject with `from: property` and `ref`
- optionally parse it with `decode`
- optionally traverse it with `path`
- optionally continue through additional selector steps

## Current Action Output

`field` selects a named output from the current action. Examples from the
checked docs use `field(stdout)`, `field(exit_code)`, and `field(values)` in
Theater DSL. YAML writes current-action roots as `subject.field` for
expectations or `exports.field` for act exports. Act exports can also use
`exports.ref` when the exported value comes from an already-available scope
value instead of the action output map.

The built-in action output fields are:

| Action | Valid `field(...)` names |
| --- | --- |
| `action.command` | `exit_code`, `stdout`, `stderr` |
| `action.generate` | `values` |
| `action.http` | `status_code`, `status`, `headers`, `body` |
| `action.state.read` | `value`, `version` |
| `action.state.update` | `value`, `version` |
| `action.state.claim` | `item`, `claim` |
| `action.state.renew` | `claim` |
| `action.state.release` | none |
| `action.state.consume` | none |

Open [Action Capabilities](capabilities/actions.md#action-output-fields) for
the meaning and value kind of each field.

A ref binding starts from `ref.name` instead. It can still use selector fields
such as `decode`, `path`, and `through`, but it does not use `field`.

## Decode

`decode(json)` parses a string or byte value as JSON. YAML writes this as
`decode: json`.

## Path

`path("/data/id")` selects an RFC 6901 JSON Pointer path. YAML writes this as
`path: /data/id`.

Theater uses JSON Pointer traversal, not JSONPath or JMESPath. A path starts
with `/`.

## Through

Longer selectors can continue with `through` steps after the first root,
`decode`, and `path`. Shipped `through` steps include additional `path`,
exact-one `pick`, `regexp` extraction, and pure transform calls.

YAML writes a transform selector step explicitly:

```
through:
  - transform:
      use: transform.jwt.claims
      with:
        audience: mobile
  - path: /uid
```

Theater DSL writes the same selector as a normal pipeline call:

```
field(body) | decode(json) | path("/token") | transform.jwt.claims(audience: "mobile") | path("/uid")
```

Transform selector steps do not publish values into scope. They only convert the
selected value before the next selector step, expectation, export, log, or
binding use.

## Collection Checks And Selection

`has item where` is a matcher form. It asserts that a selected list contains at
least one item matching the relative clauses, but it does not expose the matched
item to later selector steps.

`pick where` is a selector step. It selects exactly one list item matching the
same relative clause shape, then the pipeline can continue through ordinary
selector steps:

```
field(body) | decode(json) | path("/items") | pick where path("/id") == $item_id | path("/status")
```

Use `has item where` when the flow only needs to prove existence. Use
`pick where` when a later expectation, export, log, or binding must read from
the selected object. `pick where` fails if the input is not a list, if no item
matches, or if multiple items match. Theater does not silently choose the first
matching item.

The compact exact-one selector is still available for a single equality check:

```
field(body) | decode(json) | path("/items") | pick(at: "/id", equals: $item_id) | path("/status")
```

If `pick where` succeeds and a later selector step fails, the report cause names
the later step, for example a missing `path("/status")`, rather than reporting a
selection failure.

For a runnable selector example, use
[Check Values](../tutorial/05-check-values.md).
