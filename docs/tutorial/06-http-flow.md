# HTTP Flow

This page adds one new idea: a Theater act can call HTTP and check the response.
The example uses a tiny local fixture server started by one command, so there is
no service to install and no environment variable to export.

The flow logs in first, then fetches a profile with the same HTTP session. Think
of the session as a small browser jar: the first act receives a cookie, and the
second act reuses it.

## Theater DSL

<!-- theater-doc: source id=http-flow-thtr kind=thtr path=../examples/http-flow/profile.thtr pair=http-flow checks=fmt,lower,validate -->
```thtr
stage http-profile

http
  session browser = http.session.browser()

scenario http/profile
  act login
    prop login_url = inventory.env(name: "THEATER_DOC_LOGIN_URL")
    do action.http(method: "GET", url: $login_url, session: "browser")
    expect login-ok: field(status_code) == 200
    on pass -> fetch-profile

  act fetch-profile
    prop profile_url = inventory.env(name: "THEATER_DOC_PROFILE_URL")
    do action.http(method: "GET", url: $profile_url, session: "browser")
    expect status-ok: field(status_code) == 200
    expect profile-id: field(body) | decode(json) | path("/data/id") == "user-123"

call run = http/profile()
```

New pieces:

- `http.session.browser()` creates a named cookie session.
- `prop login_url = inventory.env(...)` reads the fixture URL supplied by the
  wrapper command.
- `action.http` performs the request.
- `field(body) | decode(json) | path("/data/id")` checks the response body.

Validate the Theater DSL file:

<!-- theater-doc: command id=http-flow-validate-thtr cwd=../.. expect-stdout=valid reject-stdout=hint -->
```sh
go run ./cmd/theater validate docs/examples/http-flow/profile.thtr
```

Run it with the local fixture. The fixture starts the HTTP server, fills the
`THEATER_DOC_*` variables for the child process, and runs everything after
`--` as the Theater command:

<!-- theater-doc: command id=http-flow-run-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-flow/profile.thtr --live off
```

A passing result proves the cookie from `login` was reused by `fetch-profile`
and the JSON profile id matched.

## YAML

<!-- theater-doc: source id=http-flow-yaml kind=yaml path=../examples/http-flow/profile.yaml pair=http-flow checks=validate -->
```yaml
id: http-profile
http:
  sessions:
    browser: {}
scenarios:
  - id: http/profile
    acts:
      - id: login
        properties:
          login_url:
            inventory:
              use: inventory.env
              with:
                name: THEATER_DOC_LOGIN_URL
        action:
          use: action.http
          with:
            method: GET
            url:
              kind: ref
              ref: login_url
            session: browser
        expectations:
          - id: login-ok
            subject:
              field: status_code
            assert:
              ref: expectation.equal
              args:
                expected: 200
        transitions:
          - on: on_pass
            to: fetch-profile
      - id: fetch-profile
        properties:
          profile_url:
            inventory:
              use: inventory.env
              with:
                name: THEATER_DOC_PROFILE_URL
        action:
          use: action.http
          with:
            method: GET
            url:
              kind: ref
              ref: profile_url
            session: browser
        expectations:
          - id: status-ok
            subject:
              field: status_code
            assert:
              ref: expectation.equal
              args:
                expected: 200
          - id: profile-id
            subject:
              field: body
              decode: json
              path: /data/id
            assert:
              ref: expectation.equal
              args:
                expected: user-123
scenario_calls:
  - id: run
    scenario_id: http/profile
```

Run the YAML version with the same fixture. The command shape is the same:
fixture first, Theater command after `--`.

<!-- theater-doc: command id=http-flow-run-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-flow/profile.yaml --live off
```

Next: [Wait For Result](07-wait-for-result.md) adds persistent state and
`eventually` polling.
