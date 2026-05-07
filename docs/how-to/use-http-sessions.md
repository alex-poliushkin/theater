# Use HTTP Sessions

Use an HTTP session when several HTTP acts should share cookies.

The checked HTTP tutorial uses a local fixture server. The first act calls
`/login`, receives a cookie, and the second act calls `/profile` with the same
`browser` session.

Run the Theater DSL session example:

<!-- theater-doc: command id=howto-http-session-run-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-flow/profile.thtr --live off
```

Run the YAML session example:

<!-- theater-doc: command id=howto-http-session-run-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-flow/profile.yaml --live off
```

A passing run proves `/login` stored a cookie in the `browser` session and
`/profile` reused that cookie when checking the profile response.

The reusable idea is small:

- declare a named session
- pass the same session name to related HTTP acts
- keep `session: none` for HTTP acts that should not share cookies

For the run and report model, open
[Validate, Run, Report](../concepts/validate-run-report.md). For the guided
explanation, open [HTTP Flow](../tutorial/06-http-flow.md).
