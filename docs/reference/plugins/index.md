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

<!-- theater-doc: command id=reference-plugin-help cwd=../../.. expect-stdout="theater plugins <command>" expect-stdout-2="plugins lock" expect-stdout-3="plugins doctor" -->
```sh
go run ./cmd/theater help plugins
```

<!-- theater-doc: command id=reference-plugin-inspect cwd=../../.. expect-stdout=hello-world expect-stdout-2=inventory.hello_world.message expect-stdout-3=action.hello_world.echo expect-stdout-4="grant: env from host PATH" -->
```sh
go run ./cmd/theater plugins inspect --plugins-config docs/examples/plugin-registry/hello-world.plugins.json
```

<!-- theater-doc: command id=reference-plugin-doctor-descriptor cwd=../../.. expect-stdout="readiness: descriptor" expect-stdout-2="plugin descriptor load" expect-stdout-3="host environment grants: skipped in descriptor readiness" -->
```sh
go run ./cmd/theater plugins doctor --plugins-config docs/examples/plugin-registry/hello-world.plugins.json --plugins-readiness descriptor
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

## Value Contracts

Plugin metadata uses value contracts to describe transform inputs and outputs,
matcher actual values, inventory results, and other explained value surfaces.

Use `type` when a value has one shape:

<!-- theater-doc: source id=reference-plugin-value-contract-string-json kind=json path=../../examples/plugin-registry/value-contract-string.json -->
```json
{
  "type": "string"
}
```

Use `kinds` when a value accepts a small union of shapes. The value is an array
of canonical Theater kinds:

<!-- theater-doc: source id=reference-plugin-value-contract-union-json kind=json path=../../examples/plugin-registry/value-contract-union.json -->
```json
{
  "kinds": [
    "string",
    "object"
  ],
  "fields": {
    "otp": {
      "type": "string",
      "required": true
    }
  }
}
```

The canonical kind names are `any`, `bytes`, `string`, `number`, `bool`,
`object`, `list`, and `null`. `fields` constrains known object members when the
contract includes `object`; `elem` constrains list elements or unconstrained
object members. `theater explain` renders unions with `|`, for example
`string|object`.

Transform manifests commonly use this shape when a pure transform accepts either
a scalar value or a small object wrapper:

<!-- theater-doc: source id=reference-plugin-transform-union-json kind=json path=../../examples/plugin-registry/transform-union-annotation.json -->
```json
{
  "annotations": {
    "transform": {
      "accepts": {
        "kinds": [
          "string",
          "object"
        ],
        "fields": {
          "otp": {
            "type": "string",
            "required": true
          }
        }
      },
      "produces": {
        "type": "string"
      }
    }
  }
}
```

## Registry Grants

Plugin processes do not inherit the full Theater host environment. A registry
must declare every environment value the plugin process may receive.

| Field | Value | Meaning |
| --- | --- | --- |
| `grants.observe_log` | boolean | Allow plugin-owned live log notifications |
| `grants.observe_progress` | boolean | Allow plugin-owned live progress notifications |
| `grants.env` | object of string values | Pass literal environment values to the plugin process |
| `grants.env_from_host` | array of names | Copy named variables from the Theater host environment into the plugin process |

Use `grants.env_from_host` for source-safe local and CI registries. For example,
`"env_from_host": ["SERVICE_CLIENT_ID"]` copies the host value of
`SERVICE_CLIENT_ID` at launch time without storing the value in the registry
file.
Missing host variables fail default runtime readiness checks for
`plugins doctor`, `validate`, and `run` with a diagnostic that names the missing
variable but does not print its value.

`plugins inspect` and `plugins doctor` render literal environment grant names
and copied host variable names. They do not print environment values. Plugin
lock files contain only manifest and executable checksums, so lock files also
do not store environment values.

Use `requirements inspect` to list plugin host environment requirements for a
specific stage and registry. Only plugins that own stage-referenced
capabilities are included. `--check-env` checks whether the named host variables
are set, but it still reports only names and readiness:

<!-- theater-doc: command id=reference-plugin-requirements cwd=../../.. expect-stdout="\"kind\": \"plugin_env_from_host\"" expect-stdout-2="\"name\": \"PATH\"" expect-stdout-3="\"readiness\": \"available\"" -->
```sh
go run ./cmd/theater requirements inspect docs/examples/plugin-registry/hello-world-stage.thtr --plugins-config docs/examples/plugin-registry/hello-world.plugins.json --check-env --format json
```

Theater reports requirements and readiness. It does not print, persist, rotate,
broker, or manage secrets. `--check-env` performs presence checks and discards
returned values.

## Readiness Modes

Plugin-aware `validate` and `plugins doctor` accept
`--plugins-readiness runtime|descriptor`.

`runtime` is the default. It checks plugin executable readiness, copied host
environment grants, plugin session initialization, and validate hooks for
referenced capabilities that opt into `theater.validate`. Use runtime readiness
before live validation or `run`.

`descriptor` is a static-analysis mode. It loads the registry and manifests,
checks registry config against plugin config schemas, validates allowed
capability refs against descriptors, and checks manifest lock metadata when a
lock file is supplied. It does not resolve `env_from_host`, launch plugin
processes, run validate hooks, run prepare hooks, or inject placeholder
credentials. A descriptor-ready result proves descriptor-backed stage structure,
not live execution readiness.

## Sensitive Values

Plugin manifests can mark JSON Pointer paths in `sensitive_input_paths` and
`sensitive_output_paths`. Input paths build a call redactor for
plugin-originated logs, diagnostics, and errors. Output paths protect returned
values before reports, exports, debug views, or later selectors see them.

Use RFC 6901 string pointers such as `/token`, `/body/accessToken`, or
`/items/0/id` for object and list members. Use the empty string `""` to mark
the whole annotated value. Root pointers are useful when an inventory or
transform capability returns a scalar secret, for example
`"sensitive_output_paths": [""]`.

Action, inventory, report exporter, and state backend input paths select their
object-shaped properties or config. For transform and matcher input redaction,
non-root paths select configuration properties and the root pointer `""` selects
the transform value or matcher actual value.

Object-shaped output surfaces, such as action outputs and state snapshots,
preserve their object shape when the root output pointer is used. Theater marks
the top-level values as secret instead of replacing the whole object with a
scalar redaction marker. Object keys remain visible and should be stable schema
field names, not data-bearing secrets.

Invalid pointer forms such as `#/token` are rejected. The string `"/"` selects
an object member whose key is empty; omit the path with `""` when the entire
value is sensitive.

## Validate And Prepare Hooks

Stage-referenced action, inventory, transform, matcher, and state backend
capabilities with `supports_validate` receive `theater.validate` during stage
validation when their handler kind supports validation. Stage-referenced
capabilities with `supports_prepare` receive `theater.prepare` while a run is
being prepared. Report exporters are prepared later during report export and
receive static exporter config. Validate and prepare calls use the same binding
shape:

- `properties` contains only statically known values from the authored
  capability call.
- `dynamic_paths` contains RFC 6901 paths for authored values that exist but
  cannot be resolved at that phase, such as `$ref` bindings, generator bindings,
  or string/object/list values that contain dynamic children. Paths may be
  nested.
- omitted keys are absent from both `properties` and `dynamic_paths`.

Object properties can be partially static: static children appear under
`properties`, while dynamic children are listed under `dynamic_paths`. Lists with
dynamic children are omitted as a whole because partial list materialization
would change indexes; Theater reports the dynamic child path and the list path.

Plugin hooks must treat a path that appears in `dynamic_paths` as authored but
runtime-resolved. They may validate literal-only constraints for keys present in
`properties`, but they must not reject a required runtime value only because its
path is dynamic. Theater does not resolve scenario inputs, prior exports,
generators, or secret runtime values before calling validate or prepare hooks.

Descriptor readiness skips plugin hook calls entirely. Use runtime readiness
when plugin-authored validate or prepare semantics must run.

## Lifecycle

| Step | Command | Purpose |
| --- | --- | --- |
| Inspect | `theater plugins inspect --plugins-config <path>` | Resolve plugin ids, manifests, executables, and allowed capabilities |
| Requirements | `theater requirements inspect <stage> --plugins-config <path> [--check-env]` | List value-free runtime requirements and optional host environment readiness |
| Digest | `theater plugins digest --manifest <manifest> --write` | Refresh descriptor digest after intentional descriptor changes |
| Lock | `theater plugins lock --plugins-config <path> --plugins-lock <path>` | Freeze checksums into the lock file |
| Diagnose | `theater plugins doctor --plugins-config <path> [--plugins-lock <path>] [--plugins-readiness runtime\|descriptor]` | Check registry readiness and optional lock drift |
| Validate | `theater validate <stage> --plugins-config <path> --plugins-lock <path> [--plugins-readiness runtime\|descriptor]` | Validate stage refs against the plugin descriptors |
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
