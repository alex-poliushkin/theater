// Package statebackend exposes the built-in Theater persistent-state backends
// together with the Register helper that installs them into a state-backend
// registrar.
//
// Most hosts should start with builtin.NewBundle instead of importing this
// package directly. Open this package when you want finer-grained registration
// than the built-in bundle provides.
package statebackend
