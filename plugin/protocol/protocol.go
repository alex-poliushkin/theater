package protocol

import (
	"time"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	reportmodel "github.com/alex-poliushkin/theater/report"
	statemodel "github.com/alex-poliushkin/theater/state"
)

const (
	JSONRPCVersion = "2.0"

	MethodInitialize       = "theater.initialize"
	MethodValidate         = "theater.validate"
	MethodPrepare          = "theater.prepare"
	MethodInventoryResolve = "theater.inventory.resolve"
	MethodActionInvoke     = "theater.action.invoke"
	MethodReportExport     = "theater.report.export"
	MethodStateRead        = "theater.state.read"
	MethodStateCAS         = "theater.state.cas"
	MethodStateClaim       = "theater.state.claim"
	MethodStateRenew       = "theater.state.renew"
	MethodStateRelease     = "theater.state.release"
	MethodStateConsume     = "theater.state.consume"
	MethodTransformApply   = "theater.transform.apply"
	MethodMatcherCheck     = "theater.matcher.check"
	MethodShutdown         = "theater.shutdown"
	MethodCancel           = "theater.cancel"
	MethodLog              = "theater.log"
	MethodProgress         = "theater.progress"

	ErrorCodePluginFailure = -32001
	ErrorCodeInvalidParams = -32602
	ErrorCodeInternal      = -32603
)

type SessionMode string

const (
	SessionModeValidate SessionMode = "validate"
	SessionModeRun      SessionMode = "run"
)

type Version struct {
	Name  string `json:"name"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
}

type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      string         `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   *ResponseError `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    ErrorData `json:"data,omitempty"`
}

type ErrorData struct {
	TheaterCode    string         `json:"theater_code,omitempty"`
	PartialOutputs map[string]any `json:"partial_outputs,omitempty"`
}

type InitializeParams struct {
	Protocol            Version        `json:"protocol"`
	Mode                SessionMode    `json:"mode"`
	SessionConfig       map[string]any `json:"session_config,omitempty"`
	AllowedCapabilities []string       `json:"allowed_capabilities,omitempty"`
	Grants              HostGrants     `json:"grants,omitempty"`
}

type InitializeResult struct {
	Plugin             pluginmanifest.Plugin `json:"plugin"`
	Protocol           Version               `json:"protocol"`
	DescriptorDigest   string                `json:"descriptor_digest"`
	ActiveCapabilities []string              `json:"active_capabilities,omitempty"`
}

type HostGrants struct {
	ObserveLog      bool              `json:"observe_log,omitempty"`
	ObserveProgress bool              `json:"observe_progress,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
}

type ValidateParams struct {
	Capability string         `json:"capability"`
	Properties map[string]any `json:"properties,omitempty"`
	// DynamicPaths names property JSON Pointer paths that are present in the
	// authored call but unavailable during static validation.
	DynamicPaths []string `json:"dynamic_paths,omitempty"`
}

type ValidateResult struct {
	Diagnostics []ValidationDiagnostic `json:"diagnostics,omitempty"`
}

type ValidationDiagnostic struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type PrepareParams struct {
	Capability string         `json:"capability"`
	Properties map[string]any `json:"properties,omitempty"`
	// DynamicPaths has the same shape as ValidateParams.DynamicPaths. Prepare
	// hooks must not assume dynamic paths are resolved before live execution.
	DynamicPaths []string `json:"dynamic_paths,omitempty"`
}

type PrepareResult struct{}

type CallContext struct {
	StageID        string     `json:"stage_id,omitempty"`
	StagePath      string     `json:"stage_path,omitempty"`
	ScenarioID     string     `json:"scenario_id,omitempty"`
	ScenarioCallID string     `json:"scenario_call_id,omitempty"`
	ActID          string     `json:"act_id,omitempty"`
	Path           string     `json:"path,omitempty"`
	Attempt        int        `json:"attempt,omitempty"`
	Deadline       *time.Time `json:"deadline,omitempty"`
}

type InventoryResolveParams struct {
	Capability string         `json:"capability"`
	Context    CallContext    `json:"context"`
	Properties map[string]any `json:"properties,omitempty"`
}

type InventoryResolveResult struct {
	Value any `json:"value,omitempty"`
}

type ActionInvokeParams struct {
	Capability string         `json:"capability"`
	Context    CallContext    `json:"context"`
	Properties map[string]any `json:"properties,omitempty"`
}

type ActionInvokeResult struct {
	Outputs map[string]any `json:"outputs,omitempty"`
}

type ReportExportParams struct {
	Capability string                  `json:"capability"`
	Properties map[string]any          `json:"properties,omitempty"`
	Document   reportmodel.RunDocument `json:"document"`
}

type ReportExportResult struct{}

type StateReadParams struct {
	Capability string         `json:"capability"`
	Config     map[string]any `json:"config,omitempty"`
	Key        string         `json:"key"`
}

type StateReadResult struct {
	Snapshot statemodel.RecordSnapshot `json:"snapshot"`
}

type StateCASParams struct {
	Capability      string         `json:"capability"`
	Config          map[string]any `json:"config,omitempty"`
	Key             string         `json:"key"`
	ExpectedVersion string         `json:"expected_version,omitempty"`
	Value           map[string]any `json:"value,omitempty"`
}

type StateCASResult struct {
	Snapshot statemodel.RecordSnapshot `json:"snapshot"`
}

type StateClaimParams struct {
	Capability string               `json:"capability"`
	Config     map[string]any       `json:"config,omitempty"`
	Pool       string               `json:"pool"`
	Selector   statemodel.Selector  `json:"selector,omitempty"`
	Lease      statemodel.LeaseSpec `json:"lease"`
}

type StateClaimResult struct {
	Result statemodel.ClaimResult `json:"result"`
}

type StateRenewParams struct {
	Capability string                 `json:"capability"`
	Config     map[string]any         `json:"config,omitempty"`
	Claim      statemodel.ClaimHandle `json:"claim"`
	TTL        time.Duration          `json:"ttl"`
}

type StateRenewResult struct {
	Claim statemodel.ClaimHandle `json:"claim"`
}

type StateReleaseParams struct {
	Capability string                 `json:"capability"`
	Config     map[string]any         `json:"config,omitempty"`
	Claim      statemodel.ClaimHandle `json:"claim"`
	Reason     string                 `json:"reason,omitempty"`
}

type StateReleaseResult struct{}

type StateConsumeParams struct {
	Capability string                 `json:"capability"`
	Config     map[string]any         `json:"config,omitempty"`
	Claim      statemodel.ClaimHandle `json:"claim"`
	Reason     string                 `json:"reason,omitempty"`
	Tombstone  map[string]any         `json:"tombstone,omitempty"`
}

type StateConsumeResult struct{}

type TransformApplyParams struct {
	Capability string         `json:"capability"`
	Properties map[string]any `json:"properties,omitempty"`
	Value      any            `json:"value,omitempty"`
}

type TransformApplyResult struct {
	Value any `json:"value,omitempty"`
}

type MatcherCheckParams struct {
	Capability string         `json:"capability"`
	Properties map[string]any `json:"properties,omitempty"`
	Actual     any            `json:"actual,omitempty"`
}

type MatcherCheckResult struct{}

type CancelParams struct {
	ID string `json:"id"`
}

type LogParams struct {
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type ProgressParams struct {
	Phase         string   `json:"phase,omitempty"`
	Message       string   `json:"message,omitempty"`
	Current       *int64   `json:"current,omitempty"`
	Total         *int64   `json:"total,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	Percent       *float64 `json:"percent,omitempty"`
	Indeterminate bool     `json:"indeterminate,omitempty"`
}
