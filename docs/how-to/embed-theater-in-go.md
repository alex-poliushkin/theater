# Embed Theater In Go

Use this when a Go tool needs to load or analyze Theater files directly instead
of shelling out to the CLI.

Run the checked Go example:

<!-- theater-doc: command id=howto-go-embedding-test cwd=../.. expect-stdout=github.com/alex-poliushkin/theater/docs/examples/go-embedding -->
```sh
go test ./docs/examples/go-embedding
```

Use `thtr.Analyze` when an editor or authoring tool needs canonical YAML and
source-map data. Use `yaml.Parse` when a Go tool already has YAML bytes and
only needs the `StageSpec`.

For exact package lookup, open [Go API](../reference/go-api.md).
