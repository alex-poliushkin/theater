# Validate, Run, Report

Theater separates checking the file from executing the file.

Validate is like reading the checklist before rehearsal starts. Theater checks
that the file can be loaded and that the declared pieces make sense before any
action runs.

Run is the rehearsal. Theater executes the selected scenario calls, records what
happened, and evaluates expectations.

The report is the receipt. It says whether the run passed, which scenario and
act were involved, and where to look when an expectation failed. JSON run output
wraps the report in a run document with a schema version, Theater version, run
identifier, optional diagnostics, and the final report.

This split matters because a file can be wrong before the system under test is
ever touched. A fast validation failure is usually easier to understand than a
runtime failure.

The first tutorial shows this order directly:

- [First Run](../tutorial/01-first-run.md) validates before running.
- [Edit And Fix](../tutorial/03-edit-and-fix.md) shows how a failed run points
  back to one expectation.

Use [reference](../reference/index.md) for exact CLI flags and output formats.
