# Transform Capabilities

YAML calls these values `decorators`; the CLI explain family is `transform`.
Both names refer to the same contract: a pure conversion applied to selected
data.

Source of truth:

- `go run ./cmd/theater explain transform`
- `go run ./cmd/theater explain transform json.decode`
- [YAML reference](../yaml/index.md)

## Checked Transform Catalog

<!-- theater-doc: command id=reference-transform-family cwd=../../.. expect-stdout="Capabilities (2):" expect-stdout-2="csv.decode" expect-stdout-3="json.decode" -->
```sh
go run ./cmd/theater explain transform
```

<!-- theater-doc: command id=reference-transform-json cwd=../../.. expect-stdout="Capability: json.decode" expect-stdout-2="Accepts:" expect-stdout-3="Produces:" -->
```sh
go run ./cmd/theater explain transform json.decode
```

## Built-In Transforms

| Ref | Accepts | Produces |
| --- | --- | --- |
| `json.decode` | `string\|bytes` | `string\|number\|bool\|object\|list\|null` |
| `csv.decode` | `string\|bytes` | list of row objects keyed by header |

Use `decorators[]` to transform inventory property values. Plugin transforms can
also be used as selector steps before later `path`, `pick`, regexp, matcher,
log, or export handling. See [Selectors](../selectors.md).

Plugin transforms declare the same contracts in their manifest metadata.
Single-shape contracts use `type`; union contracts use `kinds`, such as
`"kinds": ["string", "object"]`. `theater explain transform <ref>` renders
that contract as `string|object`.

For procedures, use
[Check JSON Response Fields](../../how-to/check-json-response-fields.md).
