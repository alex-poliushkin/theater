package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"
	pluginprotocol "github.com/alex-poliushkin/theater/plugin/protocol"
)

const errorCodeInvalidRequest = -32600

type ActionError struct {
	Code           string
	Message        string
	PartialOutputs map[string]any
}

type ActionHandler struct {
	Capability pluginmanifest.Capability
	Validate   func(context.Context, pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error)
	Prepare    func(context.Context, pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error)
	Invoke     func(context.Context, Emitter, pluginprotocol.ActionInvokeParams) (pluginprotocol.ActionInvokeResult, error)
}

type InventoryHandler struct {
	Capability pluginmanifest.Capability
	Validate   func(context.Context, pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error)
	Prepare    func(context.Context, pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error)
	Resolve    func(context.Context, Emitter, pluginprotocol.InventoryResolveParams) (pluginprotocol.InventoryResolveResult, error)
}

type ReportExporterHandler struct {
	Capability pluginmanifest.Capability
	Prepare    func(context.Context, pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error)
	Export     func(context.Context, pluginprotocol.ReportExportParams) (pluginprotocol.ReportExportResult, error)
}

type StateBackendHandler struct {
	Capability pluginmanifest.Capability
	Prepare    func(context.Context, pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error)
	Read       func(context.Context, pluginprotocol.StateReadParams) (pluginprotocol.StateReadResult, error)
	CAS        func(context.Context, pluginprotocol.StateCASParams) (pluginprotocol.StateCASResult, error)
	Claim      func(context.Context, pluginprotocol.StateClaimParams) (pluginprotocol.StateClaimResult, error)
	Renew      func(context.Context, pluginprotocol.StateRenewParams) (pluginprotocol.StateRenewResult, error)
	Release    func(context.Context, pluginprotocol.StateReleaseParams) (pluginprotocol.StateReleaseResult, error)
	Consume    func(context.Context, pluginprotocol.StateConsumeParams) (pluginprotocol.StateConsumeResult, error)
}

type TransformHandler struct {
	Capability pluginmanifest.Capability
	Validate   func(context.Context, pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error)
	Prepare    func(context.Context, pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error)
	Apply      func(context.Context, pluginprotocol.TransformApplyParams) (pluginprotocol.TransformApplyResult, error)
}

type MatcherHandler struct {
	Capability pluginmanifest.Capability
	Validate   func(context.Context, pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error)
	Prepare    func(context.Context, pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error)
	Check      func(context.Context, pluginprotocol.MatcherCheckParams) (pluginprotocol.MatcherCheckResult, error)
}

type Emitter struct {
	write func(pluginprotocol.Request) error
}

type Server struct {
	manifest              pluginmanifest.File
	actionHandlers        map[string]ActionHandler
	inventoryHandlers     map[string]InventoryHandler
	reportExporterHandles map[string]ReportExporterHandler
	stateBackendHandlers  map[string]StateBackendHandler
	transformHandlers     map[string]TransformHandler
	matcherHandlers       map[string]MatcherHandler
	initialize            func(context.Context, pluginprotocol.InitializeParams) error
	mu                    sync.Mutex
	activeCapabilities    map[string]pluginmanifest.CapabilityKind
}

func NewServer(file pluginmanifest.File) *Server {
	return &Server{
		manifest:              file,
		actionHandlers:        make(map[string]ActionHandler),
		inventoryHandlers:     make(map[string]InventoryHandler),
		reportExporterHandles: make(map[string]ReportExporterHandler),
		stateBackendHandlers:  make(map[string]StateBackendHandler),
		transformHandlers:     make(map[string]TransformHandler),
		matcherHandlers:       make(map[string]MatcherHandler),
	}
}

func (s *Server) SetInitializeHandler(fn func(context.Context, pluginprotocol.InitializeParams) error) {
	s.initialize = fn
}

func (s *Server) RegisterAction(handler ActionHandler) error {
	if handler.Capability.Kind != pluginmanifest.CapabilityKindAction {
		return fmt.Errorf("capability %q is not an action", handler.Capability.Name)
	}
	if _, ok := s.actionHandlers[handler.Capability.Name]; ok {
		return fmt.Errorf("action %q is already registered", handler.Capability.Name)
	}

	s.actionHandlers[handler.Capability.Name] = handler
	return nil
}

func (s *Server) RegisterInventory(handler InventoryHandler) error {
	if handler.Capability.Kind != pluginmanifest.CapabilityKindInventory {
		return fmt.Errorf("capability %q is not an inventory", handler.Capability.Name)
	}
	if _, ok := s.inventoryHandlers[handler.Capability.Name]; ok {
		return fmt.Errorf("inventory %q is already registered", handler.Capability.Name)
	}

	s.inventoryHandlers[handler.Capability.Name] = handler
	return nil
}

func (s *Server) RegisterReportExporter(handler ReportExporterHandler) error {
	if handler.Capability.Kind != pluginmanifest.CapabilityKindReportExporter {
		return fmt.Errorf("capability %q is not a report exporter", handler.Capability.Name)
	}
	if _, ok := s.reportExporterHandles[handler.Capability.Name]; ok {
		return fmt.Errorf("report exporter %q is already registered", handler.Capability.Name)
	}

	s.reportExporterHandles[handler.Capability.Name] = handler
	return nil
}

func (s *Server) RegisterStateBackend(handler StateBackendHandler) error {
	if handler.Capability.Kind != pluginmanifest.CapabilityKindStateBackend {
		return fmt.Errorf("capability %q is not a state backend", handler.Capability.Name)
	}
	if _, ok := s.stateBackendHandlers[handler.Capability.Name]; ok {
		return fmt.Errorf("state backend %q is already registered", handler.Capability.Name)
	}

	s.stateBackendHandlers[handler.Capability.Name] = handler
	return nil
}

func (s *Server) RegisterTransform(handler TransformHandler) error {
	if handler.Capability.Kind != pluginmanifest.CapabilityKindTransform {
		return fmt.Errorf("capability %q is not a transform", handler.Capability.Name)
	}
	if _, ok := s.transformHandlers[handler.Capability.Name]; ok {
		return fmt.Errorf("transform %q is already registered", handler.Capability.Name)
	}

	s.transformHandlers[handler.Capability.Name] = handler
	return nil
}

func (s *Server) RegisterMatcher(handler MatcherHandler) error {
	if handler.Capability.Kind != pluginmanifest.CapabilityKindMatcher {
		return fmt.Errorf("capability %q is not a matcher", handler.Capability.Name)
	}
	if _, ok := s.matcherHandlers[handler.Capability.Name]; ok {
		return fmt.Errorf("matcher %q is already registered", handler.Capability.Name)
	}

	s.matcherHandlers[handler.Capability.Name] = handler
	return nil
}

func (s *Server) ServeStdio(ctx context.Context) error {
	return s.Serve(ctx, os.Stdin, os.Stdout)
}

func (s *Server) Serve(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	writer := &serverWriter{writer: stdout}

	for {
		raw, err := pluginprotocol.ReadFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var request pluginprotocol.Request
		if err := json.Unmarshal(raw, &request); err != nil {
			if writeErr := writer.WriteResponse(pluginprotocol.Response{
				JSONRPC: pluginprotocol.JSONRPCVersion,
				Error: &pluginprotocol.ResponseError{
					Code:    errorCodeInvalidRequest,
					Message: err.Error(),
				},
			}); writeErr != nil {
				return writeErr
			}
			continue
		}

		if request.JSONRPC != pluginprotocol.JSONRPCVersion {
			if err := writer.WriteResponse(pluginprotocol.Response{
				JSONRPC: pluginprotocol.JSONRPCVersion,
				ID:      request.ID,
				Error: &pluginprotocol.ResponseError{
					Code:    errorCodeInvalidRequest,
					Message: "jsonrpc must be 2.0",
				},
			}); err != nil {
				return err
			}
			continue
		}

		if request.Method == pluginprotocol.MethodCancel {
			continue
		}

		response := s.handleRequest(ctx, request, writer)
		if request.ID == "" {
			continue
		}
		if err := writer.WriteResponse(response); err != nil {
			return err
		}
		if request.Method == pluginprotocol.MethodShutdown {
			return nil
		}
	}
}

func (e *ActionError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "plugin action failed"
}

func (s *Server) handleRequest(ctx context.Context, request pluginprotocol.Request, writer *serverWriter) pluginprotocol.Response {
	response := pluginprotocol.Response{
		JSONRPC: pluginprotocol.JSONRPCVersion,
		ID:      request.ID,
	}

	switch request.Method {
	case pluginprotocol.MethodInitialize:
		return s.handleInitializeRequest(ctx, request, response)
	case pluginprotocol.MethodValidate:
		return s.handleValidateRequest(ctx, request, response)
	case pluginprotocol.MethodPrepare:
		return s.handlePrepareRequest(ctx, request, response)
	case pluginprotocol.MethodInventoryResolve:
		return s.handleInventoryResolveRequest(ctx, writer, request, response)
	case pluginprotocol.MethodActionInvoke:
		return s.handleActionInvokeRequest(ctx, writer, request, response)
	case pluginprotocol.MethodReportExport:
		return s.handleReportExportRequest(ctx, request, response)
	case pluginprotocol.MethodStateRead:
		return s.handleStateReadRequest(ctx, request, response)
	case pluginprotocol.MethodStateCAS:
		return s.handleStateCASRequest(ctx, request, response)
	case pluginprotocol.MethodStateClaim:
		return s.handleStateClaimRequest(ctx, request, response)
	case pluginprotocol.MethodStateRenew:
		return s.handleStateRenewRequest(ctx, request, response)
	case pluginprotocol.MethodStateRelease:
		return s.handleStateReleaseRequest(ctx, request, response)
	case pluginprotocol.MethodStateConsume:
		return s.handleStateConsumeRequest(ctx, request, response)
	case pluginprotocol.MethodTransformApply:
		return s.handleTransformApplyRequest(ctx, request, response)
	case pluginprotocol.MethodMatcherCheck:
		return s.handleMatcherCheckRequest(ctx, request, response)
	case pluginprotocol.MethodShutdown:
		response.Result = map[string]any{}
		return response
	default:
		response.Error = &pluginprotocol.ResponseError{
			Code:    errorCodeInvalidRequest,
			Message: fmt.Sprintf("method %q is not supported", request.Method),
		}
		return response
	}
}

func (s *Server) handleInitializeRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.InitializeParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	if err := s.handleInitialize(ctx, params); err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = pluginprotocol.InitializeResult{
		Plugin:             s.manifest.Plugin,
		Protocol:           params.Protocol,
		DescriptorDigest:   s.manifest.DescriptorDigest,
		ActiveCapabilities: s.activeCapabilityNames(),
	}
	return response
}

func (s *Server) handleValidateRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.ValidateParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, err := s.handleValidate(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handlePrepareRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.PrepareParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, err := s.handlePrepare(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handleInventoryResolveRequest(
	ctx context.Context,
	writer *serverWriter,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.InventoryResolveParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, err := s.handleInventoryResolve(ctx, writer, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handleActionInvokeRequest(
	ctx context.Context,
	writer *serverWriter,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.ActionInvokeParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, partial, err := s.handleActionInvoke(ctx, writer, params)
	if err != nil {
		response.Error = failureError(err, partial)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handleReportExportRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.ReportExportParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, err := s.handleReportExport(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handleStateReadRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.StateReadParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	result, err := s.handleStateRead(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}
	response.Result = result
	return response
}

func (s *Server) handleStateCASRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.StateCASParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	result, err := s.handleStateCAS(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}
	response.Result = result
	return response
}

func (s *Server) handleStateClaimRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.StateClaimParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	result, err := s.handleStateClaim(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}
	response.Result = result
	return response
}

func (s *Server) handleStateRenewRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.StateRenewParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	result, err := s.handleStateRenew(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}
	response.Result = result
	return response
}

func (s *Server) handleStateReleaseRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.StateReleaseParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	result, err := s.handleStateRelease(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}
	response.Result = result
	return response
}

func (s *Server) handleStateConsumeRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.StateConsumeParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}
	result, err := s.handleStateConsume(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}
	response.Result = result
	return response
}

func (s *Server) handleTransformApplyRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.TransformApplyParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, err := s.handleTransformApply(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handleMatcherCheckRequest(
	ctx context.Context,
	request pluginprotocol.Request,
	response pluginprotocol.Response,
) pluginprotocol.Response {
	var params pluginprotocol.MatcherCheckParams
	if err := decodeParams(request.Params, &params); err != nil {
		response.Error = invalidParamsError(err)
		return response
	}

	result, err := s.handleMatcherCheck(ctx, params)
	if err != nil {
		response.Error = failureError(err, nil)
		return response
	}

	response.Result = result
	return response
}

func (s *Server) handleInitialize(ctx context.Context, params pluginprotocol.InitializeParams) error {
	if params.Protocol.Name != s.manifest.Protocol.Name {
		return fmt.Errorf("unsupported protocol %q", params.Protocol.Name)
	}
	if params.Protocol.Major != s.manifest.Protocol.Major {
		return fmt.Errorf("unsupported protocol major %d", params.Protocol.Major)
	}

	if s.initialize != nil {
		if err := s.initialize(ctx, params); err != nil {
			return err
		}
	}

	active := make(map[string]pluginmanifest.CapabilityKind)
	for _, name := range params.AllowedCapabilities {
		if handler, ok := s.actionHandlers[name]; ok {
			active[name] = handler.Capability.Kind
		}
		if handler, ok := s.inventoryHandlers[name]; ok {
			active[name] = handler.Capability.Kind
		}
		if handler, ok := s.reportExporterHandles[name]; ok {
			active[name] = handler.Capability.Kind
		}
		if handler, ok := s.stateBackendHandlers[name]; ok {
			active[name] = handler.Capability.Kind
		}
		if handler, ok := s.transformHandlers[name]; ok {
			active[name] = handler.Capability.Kind
		}
		if handler, ok := s.matcherHandlers[name]; ok {
			active[name] = handler.Capability.Kind
		}
	}
	s.mu.Lock()
	s.activeCapabilities = active
	s.mu.Unlock()

	return nil
}

func (s *Server) handleValidate(ctx context.Context, params pluginprotocol.ValidateParams) (pluginprotocol.ValidateResult, error) {
	kind, err := s.lookupCapability(params.Capability)
	if err != nil {
		return pluginprotocol.ValidateResult{}, err
	}

	switch kind {
	case pluginmanifest.CapabilityKindAction:
		handler := s.actionHandlers[params.Capability]
		if handler.Validate == nil {
			return pluginprotocol.ValidateResult{}, nil
		}
		return handler.Validate(ctx, params)
	case pluginmanifest.CapabilityKindInventory:
		inventory := s.inventoryHandlers[params.Capability]
		if inventory.Validate == nil {
			return pluginprotocol.ValidateResult{}, nil
		}
		return inventory.Validate(ctx, params)
	case pluginmanifest.CapabilityKindTransform:
		transform := s.transformHandlers[params.Capability]
		if transform.Validate == nil {
			return pluginprotocol.ValidateResult{}, nil
		}
		return transform.Validate(ctx, params)
	case pluginmanifest.CapabilityKindMatcher:
		matcher := s.matcherHandlers[params.Capability]
		if matcher.Validate == nil {
			return pluginprotocol.ValidateResult{}, nil
		}
		return matcher.Validate(ctx, params)
	default:
		return pluginprotocol.ValidateResult{}, nil
	}
}

func (s *Server) handlePrepare(ctx context.Context, params pluginprotocol.PrepareParams) (pluginprotocol.PrepareResult, error) {
	kind, err := s.lookupCapability(params.Capability)
	if err != nil {
		return pluginprotocol.PrepareResult{}, err
	}

	switch kind {
	case pluginmanifest.CapabilityKindAction:
		handler := s.actionHandlers[params.Capability]
		if handler.Prepare == nil {
			return pluginprotocol.PrepareResult{}, nil
		}
		return handler.Prepare(ctx, params)
	case pluginmanifest.CapabilityKindInventory:
		inventory := s.inventoryHandlers[params.Capability]
		if inventory.Prepare == nil {
			return pluginprotocol.PrepareResult{}, nil
		}
		return inventory.Prepare(ctx, params)
	case pluginmanifest.CapabilityKindReportExporter:
		exporter := s.reportExporterHandles[params.Capability]
		if exporter.Prepare == nil {
			return pluginprotocol.PrepareResult{}, nil
		}
		return exporter.Prepare(ctx, params)
	case pluginmanifest.CapabilityKindStateBackend:
		backend := s.stateBackendHandlers[params.Capability]
		if backend.Prepare == nil {
			return pluginprotocol.PrepareResult{}, nil
		}
		return backend.Prepare(ctx, params)
	case pluginmanifest.CapabilityKindTransform:
		transform := s.transformHandlers[params.Capability]
		if transform.Prepare == nil {
			return pluginprotocol.PrepareResult{}, nil
		}
		return transform.Prepare(ctx, params)
	case pluginmanifest.CapabilityKindMatcher:
		matcher := s.matcherHandlers[params.Capability]
		if matcher.Prepare == nil {
			return pluginprotocol.PrepareResult{}, nil
		}
		return matcher.Prepare(ctx, params)
	default:
		return pluginprotocol.PrepareResult{}, nil
	}
}

func (s *Server) handleInventoryResolve(
	ctx context.Context,
	writer *serverWriter,
	params pluginprotocol.InventoryResolveParams,
) (pluginprotocol.InventoryResolveResult, error) {
	if kind, err := s.lookupCapability(params.Capability); err != nil {
		return pluginprotocol.InventoryResolveResult{}, err
	} else if kind != pluginmanifest.CapabilityKindInventory {
		return pluginprotocol.InventoryResolveResult{}, fmt.Errorf("capability %q is not an inventory", params.Capability)
	}

	handler := s.inventoryHandlers[params.Capability]
	if handler.Resolve == nil {
		return pluginprotocol.InventoryResolveResult{}, fmt.Errorf("inventory %q resolve is not implemented", params.Capability)
	}

	return handler.Resolve(ctx, newEmitter(writer), params)
}

func (s *Server) handleActionInvoke(
	ctx context.Context,
	writer *serverWriter,
	params pluginprotocol.ActionInvokeParams,
) (pluginprotocol.ActionInvokeResult, map[string]any, error) {
	if kind, err := s.lookupCapability(params.Capability); err != nil {
		return pluginprotocol.ActionInvokeResult{}, nil, err
	} else if kind != pluginmanifest.CapabilityKindAction {
		return pluginprotocol.ActionInvokeResult{}, nil, fmt.Errorf("capability %q is not an action", params.Capability)
	}

	handler := s.actionHandlers[params.Capability]
	if handler.Invoke == nil {
		return pluginprotocol.ActionInvokeResult{}, nil, fmt.Errorf("action %q invoke is not implemented", params.Capability)
	}

	result, err := handler.Invoke(ctx, newEmitter(writer), params)
	if err == nil {
		return result, nil, nil
	}

	var actionErr *ActionError
	if errors.As(err, &actionErr) {
		return pluginprotocol.ActionInvokeResult{}, actionErr.PartialOutputs, actionErr
	}

	return pluginprotocol.ActionInvokeResult{}, nil, err
}

func (s *Server) handleReportExport(
	ctx context.Context,
	params pluginprotocol.ReportExportParams,
) (pluginprotocol.ReportExportResult, error) {
	kind, err := s.lookupCapability(params.Capability)
	if err != nil {
		return pluginprotocol.ReportExportResult{}, err
	}
	if kind != pluginmanifest.CapabilityKindReportExporter {
		return pluginprotocol.ReportExportResult{}, fmt.Errorf("capability %q is not a report exporter", params.Capability)
	}

	exporter := s.reportExporterHandles[params.Capability]
	if exporter.Export == nil {
		return pluginprotocol.ReportExportResult{}, fmt.Errorf("report exporter %q export is not implemented", params.Capability)
	}

	return exporter.Export(ctx, params)
}

func (s *Server) handleStateRead(
	ctx context.Context,
	params pluginprotocol.StateReadParams,
) (pluginprotocol.StateReadResult, error) {
	backend, err := s.stateBackend(params.Capability)
	if err != nil {
		return pluginprotocol.StateReadResult{}, err
	}
	if backend.Read == nil {
		return pluginprotocol.StateReadResult{}, fmt.Errorf("state backend %q read is not implemented", params.Capability)
	}
	return backend.Read(ctx, params)
}

func (s *Server) handleStateCAS(
	ctx context.Context,
	params pluginprotocol.StateCASParams,
) (pluginprotocol.StateCASResult, error) {
	backend, err := s.stateBackend(params.Capability)
	if err != nil {
		return pluginprotocol.StateCASResult{}, err
	}
	if backend.CAS == nil {
		return pluginprotocol.StateCASResult{}, fmt.Errorf("state backend %q cas is not implemented", params.Capability)
	}
	return backend.CAS(ctx, params)
}

func (s *Server) handleStateClaim(
	ctx context.Context,
	params pluginprotocol.StateClaimParams,
) (pluginprotocol.StateClaimResult, error) {
	backend, err := s.stateBackend(params.Capability)
	if err != nil {
		return pluginprotocol.StateClaimResult{}, err
	}
	if backend.Claim == nil {
		return pluginprotocol.StateClaimResult{}, fmt.Errorf("state backend %q claim is not implemented", params.Capability)
	}
	return backend.Claim(ctx, params)
}

func (s *Server) handleStateRenew(
	ctx context.Context,
	params pluginprotocol.StateRenewParams,
) (pluginprotocol.StateRenewResult, error) {
	backend, err := s.stateBackend(params.Capability)
	if err != nil {
		return pluginprotocol.StateRenewResult{}, err
	}
	if backend.Renew == nil {
		return pluginprotocol.StateRenewResult{}, fmt.Errorf("state backend %q renew is not implemented", params.Capability)
	}
	return backend.Renew(ctx, params)
}

func (s *Server) handleStateRelease(
	ctx context.Context,
	params pluginprotocol.StateReleaseParams,
) (pluginprotocol.StateReleaseResult, error) {
	backend, err := s.stateBackend(params.Capability)
	if err != nil {
		return pluginprotocol.StateReleaseResult{}, err
	}
	if backend.Release == nil {
		return pluginprotocol.StateReleaseResult{}, fmt.Errorf("state backend %q release is not implemented", params.Capability)
	}
	return backend.Release(ctx, params)
}

func (s *Server) handleStateConsume(
	ctx context.Context,
	params pluginprotocol.StateConsumeParams,
) (pluginprotocol.StateConsumeResult, error) {
	backend, err := s.stateBackend(params.Capability)
	if err != nil {
		return pluginprotocol.StateConsumeResult{}, err
	}
	if backend.Consume == nil {
		return pluginprotocol.StateConsumeResult{}, fmt.Errorf("state backend %q consume is not implemented", params.Capability)
	}
	return backend.Consume(ctx, params)
}

func (s *Server) handleTransformApply(
	ctx context.Context,
	params pluginprotocol.TransformApplyParams,
) (pluginprotocol.TransformApplyResult, error) {
	kind, err := s.lookupCapability(params.Capability)
	if err != nil {
		return pluginprotocol.TransformApplyResult{}, err
	}
	if kind != pluginmanifest.CapabilityKindTransform {
		return pluginprotocol.TransformApplyResult{}, fmt.Errorf("capability %q is not a transform", params.Capability)
	}

	handler := s.transformHandlers[params.Capability]
	if handler.Apply == nil {
		return pluginprotocol.TransformApplyResult{}, fmt.Errorf("transform %q apply is not implemented", params.Capability)
	}

	return handler.Apply(ctx, params)
}

func (s *Server) handleMatcherCheck(
	ctx context.Context,
	params pluginprotocol.MatcherCheckParams,
) (pluginprotocol.MatcherCheckResult, error) {
	kind, err := s.lookupCapability(params.Capability)
	if err != nil {
		return pluginprotocol.MatcherCheckResult{}, err
	}
	if kind != pluginmanifest.CapabilityKindMatcher {
		return pluginprotocol.MatcherCheckResult{}, fmt.Errorf("capability %q is not a matcher", params.Capability)
	}

	handler := s.matcherHandlers[params.Capability]
	if handler.Check == nil {
		return pluginprotocol.MatcherCheckResult{}, fmt.Errorf("matcher %q check is not implemented", params.Capability)
	}

	return handler.Check(ctx, params)
}

func (s *Server) lookupCapability(name string) (pluginmanifest.CapabilityKind, error) {
	s.mu.Lock()
	active := make(map[string]pluginmanifest.CapabilityKind, len(s.activeCapabilities))
	for key, kind := range s.activeCapabilities {
		active[key] = kind
	}
	s.mu.Unlock()

	kind, ok := active[name]
	if !ok {
		return "", fmt.Errorf("capability %q is not active", name)
	}

	switch kind {
	case pluginmanifest.CapabilityKindAction:
		if _, ok := s.actionHandlers[name]; ok {
			return kind, nil
		}
	case pluginmanifest.CapabilityKindInventory:
		if _, ok := s.inventoryHandlers[name]; ok {
			return kind, nil
		}
	case pluginmanifest.CapabilityKindReportExporter:
		if _, ok := s.reportExporterHandles[name]; ok {
			return kind, nil
		}
	case pluginmanifest.CapabilityKindStateBackend:
		if _, ok := s.stateBackendHandlers[name]; ok {
			return kind, nil
		}
	case pluginmanifest.CapabilityKindTransform:
		if _, ok := s.transformHandlers[name]; ok {
			return kind, nil
		}
	case pluginmanifest.CapabilityKindMatcher:
		if _, ok := s.matcherHandlers[name]; ok {
			return kind, nil
		}
	}

	return "", fmt.Errorf("capability %q is not registered", name)
}

func (s *Server) stateBackend(name string) (StateBackendHandler, error) {
	kind, err := s.lookupCapability(name)
	if err != nil {
		return StateBackendHandler{}, err
	}
	if kind != pluginmanifest.CapabilityKindStateBackend {
		return StateBackendHandler{}, fmt.Errorf("capability %q is not a state backend", name)
	}

	handler, ok := s.stateBackendHandlers[name]
	if !ok {
		return StateBackendHandler{}, fmt.Errorf("state backend %q is not registered", name)
	}

	return handler, nil
}

func (s *Server) activeCapabilityNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.activeCapabilities))
	for name := range s.activeCapabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func newEmitter(writer *serverWriter) Emitter {
	return Emitter{
		write: func(request pluginprotocol.Request) error {
			return writer.WriteRequest(request)
		},
	}
}

func (e Emitter) Log(message string, fields map[string]string) error {
	if e.write == nil {
		return nil
	}

	return e.write(pluginprotocol.Request{
		JSONRPC: pluginprotocol.JSONRPCVersion,
		Method:  pluginprotocol.MethodLog,
		Params: pluginprotocol.LogParams{
			Message: message,
			Fields:  cloneStringMap(fields),
		},
	})
}

func (e Emitter) Progress(progress pluginprotocol.ProgressParams) error {
	if e.write == nil {
		return nil
	}

	return e.write(pluginprotocol.Request{
		JSONRPC: pluginprotocol.JSONRPCVersion,
		Method:  pluginprotocol.MethodProgress,
		Params:  progress,
	})
}

type serverWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *serverWriter) WriteResponse(response pluginprotocol.Response) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return pluginprotocol.WriteFrame(w.writer, response)
}

func (w *serverWriter) WriteRequest(request pluginprotocol.Request) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return pluginprotocol.WriteFrame(w.writer, request)
}

func invalidParamsError(err error) *pluginprotocol.ResponseError {
	return &pluginprotocol.ResponseError{
		Code:    pluginprotocol.ErrorCodeInvalidParams,
		Message: err.Error(),
	}
}

func failureError(err error, partial map[string]any) *pluginprotocol.ResponseError {
	theaterCode := "plugin_failed"
	var actionErr *ActionError
	if errors.As(err, &actionErr) && actionErr.Code != "" {
		theaterCode = actionErr.Code
	}

	return &pluginprotocol.ResponseError{
		Code:    pluginprotocol.ErrorCodePluginFailure,
		Message: err.Error(),
		Data: pluginprotocol.ErrorData{
			TheaterCode:    theaterCode,
			PartialOutputs: partial,
		},
	}
}

func decodeParams(raw, target any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	return json.Unmarshal(encoded, target)
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}

	return cloned
}
