# Matchers

Matchers evaluate selected values and produce expectation results.

Source of truth:

- `go run ./cmd/theater explain matcher`
- `go run ./cmd/theater explain matcher <matcher-ref>`
- [Expectations](../expectations.md)

## Checked Matcher Catalog

<!-- theater-doc: command id=reference-matcher-family cwd=../../.. expect-stdout="Capabilities (17):" expect-stdout-2="expectation.equal" expect-stdout-3="expectation.present" -->
```sh
go run ./cmd/theater explain matcher
```

<!-- theater-doc: command id=reference-matcher-equal cwd=../../.. expect-stdout="Capability: expectation.equal" expect-stdout-2="expected  any; required" expect-stdout-3="keys: eq" -->
```sh
go run ./cmd/theater explain matcher equal
```

## Built-In Matchers

| Ref | Purpose |
| --- | --- |
| `expectation.equal` | Actual value equals expected value |
| `expectation.not` | Actual value does not match nested assert |
| `expectation.contains` | Actual string or list contains expected value |
| `expectation.matches` | Actual string matches a regular expression |
| `expectation.gt` / `expectation.gte` | Numeric greater-than checks |
| `expectation.lt` / `expectation.lte` | Numeric less-than checks |
| `expectation.between` | Inclusive numeric range check |
| `expectation.present` | Selected value is present, including explicit `null` |
| `expectation.null` / `expectation.not_null` | Explicit null checks |
| `expectation.has_key` / `expectation.lacks_key` | Object member presence checks |
| `expectation.has_entry` | Object member value check with nested assert |
| `expectation.has_item` | At least one list item matches all where clauses |
| `expectation.all_items` | Every list item matches all where clauses |

## Theater DSL Sugar

`S` below means the selected subject after `expect <id>:`. The command output's
`Sugar` section describes descriptor/YAML sugar keys; the `.thtr` authoring
surface uses the forms in this table.

| Ref | Theater DSL form |
| --- | --- |
| `expectation.equal` | `S == V` |
| `expectation.not` | `S != V`; `S not <assertion-core>` |
| `expectation.contains` | `S contains V` |
| `expectation.matches` | `S matches P` |
| `expectation.gt` | `S > V` |
| `expectation.gte` | `S >= V` |
| `expectation.lt` | `S < V` |
| `expectation.lte` | `S <= V` |
| `expectation.between` | `S between MIN and MAX` |
| `expectation.present` | `S is present` |
| `expectation.null` | `S is null` |
| `expectation.not_null` | `S is not null` |
| `expectation.has_key` | `S has key(K)` |
| `expectation.lacks_key` | `S lacks key(K)`; `S has no key(K)` |
| `expectation.has_entry` | `S has entry(K) <assertion-core>` |
| `expectation.has_item` | `S has item where C`; `S has item where (C1, C2)` |
| `expectation.all_items` | `S all items where C`; `S all items where (C1, C2)` |

`<assertion-core>` is any assertion form after the subject, for example
`== "ok"`, `matches r"^token-"`, `not < 5`, or
`assert expectation.equal(expected: "ok")`. Collection clause `C` is relative to
the current item and must start with `path(...)` or `decode(...)`; when both are
used, `decode(...)` comes before `path(...)`.

Both `lacks key(K)` and `has no key(K)` are accepted; `theater fmt` renders the
canonical `lacks key(K)` spelling. Negation wraps the inner matcher without
changing its validation rules.
`not != V` and `not is present` are rejected. Use `lacks key(K)` or
`has no key(K)` on the containing object to assert an absent object member.

Checked Theater DSL example using the built-in matcher sugar forms:

<!-- theater-doc: source id=reference-matchers-thtr kind=thtr path=../../examples/reference/matchers.thtr checks=fmt,lower,validate,run -->
```thtr
stage reference-matchers

scenario inspect
  act check
    do action.generate
      outputs:
        status: "ok"
        message: "hello example"
        code: "A123"
        count: 3
        retry_count: 2
        score: 8
        payload: object {
          session_token: "token-123",
          deleted_at: null,
          profile: object { status: "active" }
        }
        users: list [
          object { id: "user-123", email: "demo@example.test", active: true },
          object { id: "user-456", email: "other@example.test", active: true }
        ]
    expect equal: field(values) | path("/status") == "ok"
    expect not-equal: field(values) | path("/status") != "error"
    expect contains: field(values) | path("/message") contains "example"
    expect matches: field(values) | path("/code") matches r"^[A-Z][0-9]{3}$"
    expect gt: field(values) | path("/score") > 5
    expect gte: field(values) | path("/score") >= 8
    expect lt: field(values) | path("/count") < 10
    expect lte: field(values) | path("/retry_count") <= 2
    expect between: field(values) | path("/retry_count") between 1 and 3
    expect present: field(values) | path("/payload/session_token") is present
    expect null: field(values) | path("/payload/deleted_at") is null
    expect not-null: field(values) | path("/payload/session_token") is not null
    expect has-key: field(values) | path("/payload") has key("session_token")
    expect lacks-key: field(values) | path("/payload") lacks key("error")
    expect has-no-key: field(values) | path("/payload") lacks key("error")
    expect has-entry: field(values) | path("/payload") has entry("session_token") matches r"^token-"
    expect has-item: field(values) | path("/users") has item where path("/id") == "user-123"
    expect all-items: field(values) | path("/users") all items where path("/email") contains "@example.test"
    expect grouped-items: field(values) | path("/users") all items where (
      path("/email") contains "@example.test",
      path("/active") == true
    )
    expect not-wrapper: field(values) | path("/score") not < 5
    expect canonical-call: field(values) | path("/status") assert expectation.equal(expected: "ok")

call run = inspect()
```

For the broader `.thtr` grammar, use
[Theater DSL](../theater-dsl/index.md). For YAML descriptor sugar, use
[YAML](../yaml/index.md).

For procedures, use
[Check JSON Response Fields](../../how-to/check-json-response-fields.md).
