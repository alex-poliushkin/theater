# Expectations

An expectation checks one selected subject with one matcher.

The compact Theater DSL form keeps the subject and matcher on one line:
`expect profile-id: field(stdout) | decode(json) | path("/data/id") == "user-123"`.

YAML writes the same model with explicit fields:

- `id` names the expectation.
- `subject` selects the value to check.
- `assert.ref` names the matcher.
- `assert.args` passes matcher arguments.

## Equality

The checked docs currently use equality as the first matcher because it is the
smallest useful check. In Theater DSL, `== "user-123"` lowers to the canonical
YAML matcher `expectation.equal` with `args.expected: user-123`.

## Subject

The default subject source is the current action output. YAML writes that as
`subject.field`. Common additions are `subject.decode: json` and
`subject.path: /data/id`.

## Assert

`assert.ref` may point to a built-in matcher or a matcher provided by the host.
This page intentionally does not list the full matcher catalog. Use the YAML
source-of-truth reference when you need every shipped matcher form.

For a runnable expectation example, use
[Check Values](../tutorial/05-check-values.md).
