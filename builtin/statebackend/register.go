package statebackend

import "github.com/alex-poliushkin/theater"

const FileBackendRef = "state.backend.file"

func Register(catalog theater.StateBackendRegistrar) error {
	return catalog.RegisterStateBackend(FileBackendRef, fileBackendDefinition())
}
