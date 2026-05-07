# Edit And Fix

This page shows one safe mistake. The stage still generates `hello`, but the
check expects `goodbye`. That mismatch is intentional.

A Theater failure report is like a note pinned to the exact checklist line that
failed. Read the short summary first, then use the address to find the failed
expectation.

## Theater DSL Failure

Run the intentionally wrong Theater DSL file:

<!-- theater-doc: command id=edit-fix-fail-thtr cwd=../.. expect-exit=1 expect-stdout="actual hello does not equal expected goodbye" expect-stdout-2=stage.docs-first/call.run/act.say-hello/expectation.message -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage-wrong.thtr --live off
```

The important part of the output is `actual hello does not equal expected
goodbye`. Theater also prints the failed expectation address:
`stage.docs-first/call.run/act.say-hello/expectation.message`.

The last segment, `expectation.message`, points back to the `expect message`
check from the Theater DSL file and the `id: message` expectation from the YAML
file.

Fixing the mistake means making the generated value and expected value match
again. In the wrong Theater DSL file, the concrete edit is changing the expected
word from `goodbye` back to `hello`. The original Theater DSL file already has
that fix:

<!-- theater-doc: command id=edit-fix-pass-thtr cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.thtr --live off
```

## YAML Failure

The YAML wrong file makes the same safe mistake: `args.expected` is `goodbye`
while `object.message` is still `hello`.

<!-- theater-doc: command id=edit-fix-fail-yaml cwd=../.. expect-exit=1 expect-stdout="actual hello does not equal expected goodbye" expect-stdout-2=stage.docs-first/call.run/act.say-hello/expectation.message -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage-wrong.yaml --live off
```

Fixing YAML uses the same rule: make the generated value and expected value
match. In the wrong YAML file, the concrete edit is changing `args.expected`
from `goodbye` back to `hello`. The original YAML file already has that fix:

<!-- theater-doc: command id=edit-fix-pass-yaml cwd=../.. expect-stdout=passed -->
```sh
go run ./cmd/theater run docs/examples/first-stage/stage.yaml --live off
```

You have now seen the full first loop: run a passing file, understand the
smallest stage, make one mistake, read the failure, and fix it.
