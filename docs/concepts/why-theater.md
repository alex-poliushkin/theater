# Why Theater

Theater is for repeatable checks of API and workflow behavior from files. A
Theater file says what should happen, how to run the check, and what result
counts as passing.

The useful analogy is a rehearsal checklist. A team can run the same checklist
before every release, see exactly which line failed, and improve the checklist
without turning it into application code.

Use Theater when a check needs to be:

- repeatable by another person or CI job
- readable without opening application source code
- precise about which step or expectation failed
- portable between a compact Theater DSL file and an explicit YAML file

The first tutorial loop shows the shape:

- [First Run](../tutorial/01-first-run.md) runs the smallest checked example.
- [First Stage](../tutorial/02-first-stage.md) explains the file shape.
- [Edit And Fix](../tutorial/03-edit-and-fix.md) shows a failure and the fix.

Theater is not a general application framework. It is a runner for verification
scenarios. Keep production behavior in the system under test; keep repeatable
checks in Theater files.

For exact syntax and contracts, use [reference](../reference/index.md).
