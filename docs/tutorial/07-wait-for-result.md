# Wait For Result

This page adds two ideas after the HTTP flow: save a checked value in local
state, then poll a status endpoint until it becomes ready.

The local fixture server returns `ready: false` once and `ready: true` on the
next poll. That makes `eventually` visible without waiting on a real service.
Read the page in two passes if it feels dense: run the example first, then come
back to the state and polling lines.

## Theater DSL

<!-- theater-doc: source id=http-state-thtr kind=thtr path=../examples/http-state/profile-state.thtr pair=http-state checks=fmt,lower,validate -->
```thtr
stage http-state-profile

http
  session browser = http.session.browser()

state
  backend local = state.backend.file(root: "/tmp/theater-doc-state")
  record profile_cache = state.record
    backend: local
    record: "profile-cache"
    min_guarantee: local-atomic

scenario http/profile-state
  act login
    prop login_url = inventory.env(name: "THEATER_DOC_LOGIN_URL")
    do action.http(method: "GET", url: $login_url, session: "browser")
    expect login-ok: field(status_code) == 200
    on pass -> fetch-profile

  act fetch-profile
    prop profile_url = inventory.env(name: "THEATER_DOC_PROFILE_URL")
    do action.http(method: "GET", url: $profile_url, session: "browser")
    expect status-ok: field(status_code) == 200
    expect active: field(body) | decode(json) | path("/data/status") == "active"
    export profile_id = field(body) | decode(json) | path("/data/id")
    on pass -> read-cache

  act read-cache
    do state.read(record: profile_cache)
    export cache_version = field(version)
    on pass -> cache-profile

  act cache-profile
    do state.update
      record: profile_cache
      if_version: $cache_version
      value: object { profile_id: $profile_id, status: "cached" }
    expect cache-version: field(version) matches r"^[0-9]+$"
    on pass -> wait-ready

  act wait-ready
    eventually 3s every 100ms
    prop status_url = inventory.env(name: "THEATER_DOC_STATUS_URL")
    do repeatable action.http(method: "GET", url: $status_url, session: "none")
    expect ready: field(body) | decode(json) | path("/ready") == true

call run = http/profile-state()
```

Read the new part in order:

- `state.backend.file` stores state in a local directory.
- `state.read` returns the current record version.
- `state.update` writes only when that version still matches.
- `eventually 3s every 100ms` can repeat the whole `wait-ready` act until its
  expectation passes or the timeout expires.
- `do repeatable action.http(...)` marks the HTTP call as safe to repeat. Without
  `repeatable`, `eventually` cannot retry the action call.

Run the Theater DSL flow:

<!-- theater-doc: command id=http-state-run-thtr cwd=../.. expect-stdout=passed expect-stdout-2="eventually: converged_acts=1" -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-state/profile-state.thtr --live off
```

The output includes `eventually: converged_acts=1`, which means at least one act
needed polling before it passed. The passing run also proves the profile id was
written through the file state backend before polling started.

## YAML

The YAML file is longer because state properties and action inputs are explicit.
It is the same flow, not a different feature.

<!-- theater-doc: source id=http-state-yaml kind=yaml path=../examples/http-state/profile-state.yaml pair=http-state checks=validate -->
```yaml
id: http-state-profile
http:
  sessions:
    browser: {}
state:
  backends:
    local:
      use: state.backend.file
      with:
        root: /tmp/theater-doc-state
scenarios:
  - id: http/profile-state
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
          - id: active
            subject:
              field: body
              decode: json
              path: /data/status
            assert:
              ref: expectation.equal
              args:
                expected: active
        exports:
          - as: profile_id
            field: body
            decode: json
            path: /data/id
        transitions:
          - on: on_pass
            to: read-cache
      - id: read-cache
        properties:
          profile_cache:
            inventory:
              use: inventory.state.record
              with:
                backend: local
                record: profile-cache
                min_guarantee: local-atomic
        action:
          use: action.state.read
          with:
            record:
              kind: ref
              ref: profile_cache
        exports:
          - as: cache_version
            field: version
        transitions:
          - on: on_pass
            to: cache-profile
      - id: cache-profile
        properties:
          profile_cache:
            inventory:
              use: inventory.state.record
              with:
                backend: local
                record: profile-cache
                min_guarantee: local-atomic
        action:
          use: action.state.update
          with:
            record:
              kind: ref
              ref: profile_cache
            expected_version:
              kind: ref
              ref: cache_version
            value:
              kind: object
              object:
                profile_id:
                  kind: ref
                  ref:
                    name: profile_id
                status: cached
        expectations:
          - id: cache-version
            subject:
              field: version
            assert:
              ref: expectation.matches
              args:
                pattern: ^[0-9]+$
        transitions:
          - on: on_pass
            to: wait-ready
      - id: wait-ready
        eventually:
          timeout: 3s
          interval: 100ms
        properties:
          status_url:
            inventory:
              use: inventory.env
              with:
                name: THEATER_DOC_STATUS_URL
        action:
          use: action.http
          repeatable: true
          with:
            method: GET
            url:
              kind: ref
              ref: status_url
            session: none
        expectations:
          - id: ready
            subject:
              field: body
              decode: json
              path: /ready
            assert:
              ref: expectation.equal
              args:
                expected: true
scenario_calls:
  - id: run
    scenario_id: http/profile-state
```

Run the YAML flow:

<!-- theater-doc: command id=http-state-run-yaml cwd=../.. expect-stdout=passed expect-stdout-2="eventually: converged_acts=1" -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-state/profile-state.yaml --live off
```

You now have a complete local example with HTTP, session reuse, persistent
state, and polling.
