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
      ],
      "grants": {
        "env_from_host": [
          "PATH"
        ]
      }
    }
  }
}
```

Inspect the registry:

<!-- theater-doc: command id=howto-plugin-inspect cwd=../.. expect-stdout=hello-world expect-stdout-2=inventory.hello_world.message expect-stdout-3="\"grants\"" expect-stdout-4="\"env_from_host\"" -->
```sh
go run ./cmd/theater plugins inspect --plugins-config docs/examples/plugin-registry/hello-world.plugins.json --format json
```

The JSON output includes the `hello-world` plugin id, allowed capability names,
and grant names. Environment values are not printed.

Run the readiness check:

<!-- theater-doc: command id=howto-plugin-doctor cwd=../.. expect-stdout=ready expect-stdout-2="host environment grants" expect-stdout-3="env from host PATH" -->
```sh
go run ./cmd/theater plugins doctor --plugins-config docs/examples/plugin-registry/hello-world.plugins.json
```

`plugin registry: ready` means the registry is valid, the manifest and
executable path are reachable, and copied host environment grants are available.
When a lock file is supplied, `plugins doctor` also checks lock drift. Use
`plugins inspect` to review allowed capability names and plugin grant shape.

For static checks on a registry that declares runtime-only host secrets, use
descriptor readiness:

<!-- theater-doc: command id=howto-plugin-doctor-descriptor cwd=../.. expect-stdout="readiness: descriptor" expect-stdout-2="host environment grants: skipped in descriptor readiness" -->
```sh
go run ./cmd/theater plugins doctor --plugins-config docs/examples/plugin-registry/hello-world.plugins.json --plugins-readiness descriptor
```

Descriptor readiness validates registry and manifest descriptors without
launching the plugin process or resolving `env_from_host`. Before a live run or
runtime validate hook check, run doctor again with the default runtime
readiness.

Check that the example plugin process can start from this checkout:

<!-- theater-doc: command id=howto-plugin-process-smoke cwd=../.. expect-stdout="plugin process smoke: ok" -->
```sh
go run ./docs/examples/plugin-registry/process-smoke
```

Use `plugins inspect` and `plugins doctor` as preflight with only
`--plugins-config`. Before a plugin-backed stage can be used with `validate` or
`run`, create a lock file with `plugins lock` and pass both `--plugins-config`
and `--plugins-lock`.

For source-safe registries, prefer `grants.env_from_host` over literal
`grants.env` secret values. A registry such as
`"env_from_host": ["SERVICE_CLIENT_ID"]` copies only that named host variable
into the plugin process. Theater does not pass through the full ambient
environment.
