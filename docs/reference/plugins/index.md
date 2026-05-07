# Plugin Reference

Plugins extend the same capability catalogs used by built-ins. A plugin
registry names local plugin manifests, executable commands, and the exact
capabilities the host may expose.

Source of truth:

- `plugin/manifest/`
- `plugin/registry/`
- `plugin/protocol/`
- `plugin/sdk/`
- `go run ./cmd/theater help plugins`
- [CLI Reference](../cli/index.md)

## Checked Registry Example

<!-- theater-doc: source id=reference-plugin-registry-json kind=json path=../../examples/plugin-registry/hello-world.plugins.json -->
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

<!-- theater-doc: command id=reference-plugin-help cwd=../../.. expect-stdout="theater plugins <command>" expect-stdout-2="plugins lock" expect-stdout-3="plugins doctor" -->
```sh
go run ./cmd/theater help plugins
```

<!-- theater-doc: command id=reference-plugin-inspect cwd=../../.. expect-stdout=hello-world expect-stdout-2=inventory.hello_world.message expect-stdout-3=action.hello_world.echo -->
```sh
go run ./cmd/theater plugins inspect --plugins-config docs/examples/plugin-registry/hello-world.plugins.json
```

<!-- theater-doc: command id=reference-plugin-process-smoke cwd=../../.. expect-stdout="plugin process smoke: ok" -->
```sh
go run ./docs/examples/plugin-registry/process-smoke
```

## Registry And Lock Files

| File | Schema |
| --- | --- |
| Plugin registry config | `theater.plugin.registry/v1alpha1` |
| Plugin lock file | `theater.plugin.lock/v1alpha1` |
| Plugin manifest | `theater.plugin.manifest/v1alpha1` |

The registry config is the editable input. The lock file freezes manifest and
executable checksums for validation and run automation.

## Lifecycle

| Step | Command | Purpose |
| --- | --- | --- |
| Inspect | `theater plugins inspect --plugins-config <path>` | Resolve plugin ids, manifests, executables, and allowed capabilities |
| Digest | `theater plugins digest --manifest <manifest> --write` | Refresh descriptor digest after intentional descriptor changes |
| Lock | `theater plugins lock --plugins-config <path> --plugins-lock <path>` | Freeze checksums into the lock file |
| Diagnose | `theater plugins doctor --plugins-config <path> [--plugins-lock <path>]` | Check registry readiness and optional lock drift |
| Validate | `theater validate <stage> --plugins-config <path> --plugins-lock <path>` | Validate stage refs against the plugin descriptors |
| Run | `theater run <stage> --plugins-config <path> --plugins-lock <path>` | Execute against the same locked plugin set |

## Capability Kinds

| Manifest kind | Host catalog family |
| --- | --- |
| `action` | `action` |
| `inventory` | `inventory` |
| `transform` | `transform` |
| `matcher` | `matcher` |
| `state_backend` | `state-backend` |
| `report_exporter` | `report-exporter` |

For a task recipe, use
[Inspect A Plugin Registry](../../how-to/inspect-plugin-registry.md).
