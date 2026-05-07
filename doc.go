// Package theater provides the orchestration API for compiling, validating,
// and running declarative stages.
//
// The root package owns the common runtime entry points and extension seams,
// such as Catalog, Runner, Validator, Action, Inventory, and decorators.
// Canonical authoring model types live in package spec, canonical report
// document types live in package report, and canonical persistent-state types
// live in package state.
package theater
