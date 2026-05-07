# Core HTTP And State Flow

This complete example combines the pieces from the tutorial path:

- HTTP session reuse
- JSON response checks
- a file-backed state record
- an `eventually` polling act

Source files:

- [Theater DSL](http-state/profile-state.thtr)
- [YAML](http-state/profile-state.yaml)
- [Local fixture wrapper](http-state/fixture/main.go)

Run the complete Theater DSL example:

<!-- theater-doc: command id=core-http-state-run-thtr cwd=../.. expect-stdout=passed expect-stdout-2="eventually: converged_acts=1" -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-state/profile-state.thtr --live off
```

Run the complete YAML example:

<!-- theater-doc: command id=core-http-state-run-yaml cwd=../.. expect-stdout=passed expect-stdout-2="eventually: converged_acts=1" -->
```sh
go run ./docs/examples/http-state/fixture -- go run ./cmd/theater run docs/examples/http-state/profile-state.yaml --live off
```

The fixture wrapper starts a local server, provides URLs to Theater, resets the
temporary file state root, and removes it after the run.
