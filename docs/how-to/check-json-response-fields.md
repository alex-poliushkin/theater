# Check JSON Response Fields

Use this recipe when an action returns a JSON string and you need to check one
field inside it.

The runnable docs example uses `action.command` and `stdout` as a deterministic
response body. In an HTTP flow, the same selector usually starts from
`field(body)` instead of `field(stdout)`.

## Steps

1. Choose the action output field that contains JSON.
2. Decode it with `decode(json)`.
3. Select one RFC 6901 path.
4. Compare the selected value with a matcher.

The checked Theater DSL example is
`docs/examples/check-values/profile.thtr`. It checks `stdout` with
`field(stdout) | decode(json) | path("/data/id") == "user-123"`.

Run the task example:

<!-- theater-doc: command id=howto-check-json-run-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.thtr --live off
```

A passing result means Theater decoded the JSON string, selected `/data/id`, and
checked the expected value before the next act reused the exported id.

The YAML equivalent is `docs/examples/check-values/profile.yaml`. The same check
is written with `subject.field`, `subject.decode`, `subject.path`, and
`assert.ref`.

Run the YAML task example:

<!-- theater-doc: command id=howto-check-json-run-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/check-values/profile.yaml --live off
```

Use [Selectors](../reference/selectors.md) for exact selector fields and
[Expectations](../reference/expectations.md) for exact expectation fields.
