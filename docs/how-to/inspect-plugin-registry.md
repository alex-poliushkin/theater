# Inspect A Plugin Registry

Use a plugin registry file when a flow depends on plugin-backed capabilities.
Inspect it before creating the lock file required by `validate` and `run`.

The example registry points at the repository's `hello-world` plugin manifest
and allows two capability names.

<!-- theater-doc: source id=howto-plugin-registry-json kind=json path=../examples/plugin-registry/hello-world.plugins.json -->
```json
{
  "schema": "theater.plugin.registry/v1alpha1",
  "plugins": {
    "hello-world": {
      "manifest": "../../../plugins/hello-world/manifest.json",
      "exec": {
        "command": [
          "go",
          "run",
          "./plugins/hello-world/main.go"
        ]
      },
      "allow_capabilities": [
        "inventory.hello_world.message",
        "action.hello_world.echo"
      ]
    }
  }
}
```

Inspect the registry:

<!-- theater-doc: command id=howto-plugin-inspect cwd=../.. expect-stdout=hello-world expect-stdout-2=inventory.hello_world.message -->
```sh
go run ./cmd/theater plugins inspect --plugins-config docs/examples/plugin-registry/hello-world.plugins.json --format json
```

The JSON output includes the `hello-world` plugin id and the allowed capability
names.

Run the readiness check:

<!-- theater-doc: command id=howto-plugin-doctor cwd=../.. expect-stdout=ready -->
```sh
go run ./cmd/theater plugins doctor --plugins-config docs/examples/plugin-registry/hello-world.plugins.json
```

`plugin registry: ready` means the registry is valid and the manifest and
executable path are reachable. When a lock file is supplied, `plugins doctor`
also checks lock drift. Use `plugins inspect` to review allowed capability
names.

Check that the example plugin process can start from this checkout:

<!-- theater-doc: command id=howto-plugin-process-smoke cwd=../.. expect-stdout="plugin process smoke: ok" -->
```sh
go run ./docs/examples/plugin-registry/process-smoke
```

Use `plugins inspect` and `plugins doctor` as preflight with only
`--plugins-config`. Before a plugin-backed stage can be used with `validate` or
`run`, create a lock file with `plugins lock` and pass both `--plugins-config`
and `--plugins-lock`.
