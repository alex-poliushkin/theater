package theater

import (
	statemodel "github.com/alex-poliushkin/theater/state"
)

const (
	StateGuaranteeReadOnly         StateGuaranteeTier = statemodel.GuaranteeReadOnly
	StateGuaranteeLocalAtomic      StateGuaranteeTier = statemodel.GuaranteeLocalAtomic
	StateGuaranteeSharedOptimistic StateGuaranteeTier = statemodel.GuaranteeSharedOptimistic
	StateGuaranteeSharedAtomic     StateGuaranteeTier = statemodel.GuaranteeSharedAtomic

	StateExpiryStale   StateExpiryPolicy = statemodel.ExpiryStale
	StateExpiryReclaim StateExpiryPolicy = statemodel.ExpiryReclaim
)

type StateGuaranteeTier = statemodel.GuaranteeTier

type StateExpiryPolicy = statemodel.ExpiryPolicy

type StateRecordHandle = statemodel.RecordHandle

type StatePoolHandle = statemodel.PoolHandle

type StateRecordSnapshot = statemodel.RecordSnapshot

type StateClaimHandle = statemodel.ClaimHandle

type StateClaimResult = statemodel.ClaimResult

type StateSelector = statemodel.Selector

type StateLeaseSpec = statemodel.LeaseSpec

type StateDescriptor = statemodel.Descriptor

type StateBackend = statemodel.Backend

type StateManager = statemodel.Manager

type StateBackendDef struct {
	Params   []ParamSpec
	Describe func(config Values) (StateDescriptor, error)
	Open     func(config Values) (StateBackend, error)
}

type StateBackendRegistrar interface {
	RegisterStateBackend(ref string, backend StateBackendDef) error
}

type StateBackendResolver interface {
	ResolveStateBackend(ref string) (StateBackendDef, error)
}
