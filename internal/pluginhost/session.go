package pluginhost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alex-poliushkin/theater/internal/pluginredact"
	"github.com/alex-poliushkin/theater/internal/pluginregistry"
	"github.com/alex-poliushkin/theater/plugin/protocol"
)

const (
	defaultCancelGrace    = 500 * time.Millisecond
	defaultShutdownGrace  = time.Second
	defaultRequestTimeout = 15 * time.Second
	maxStderrBytes        = 4 << 10
)

type NotificationSink interface {
	Log(protocol.LogParams)
	Progress(protocol.ProgressParams)
}

type Session struct {
	plugin         pluginregistry.LoadedPlugin
	stdin          io.WriteCloser
	stdout         *bufio.Reader
	stderr         *bytes.Buffer
	shutdownGrace  time.Duration
	cancelGrace    time.Duration
	requestTimeout time.Duration
	redactor       pluginredact.Redactor
	kill           func() error
	closeTransport func(context.Context) error
	nextID         atomic.Uint64
	mu             sync.Mutex
}

type OpenConfig struct {
	Mode                protocol.SessionMode
	AllowedCapabilities []string
	Grants              protocol.HostGrants
	SessionConfig       map[string]any
}

type CallError struct {
	Response protocol.ResponseError
}

func Open(ctx context.Context, plugin pluginregistry.LoadedPlugin, config OpenConfig) (*Session, protocol.InitializeResult, error) {
	transport, err := openTransport(ctx, plugin)
	if err != nil {
		return nil, protocol.InitializeResult{}, err
	}

	session := &Session{
		plugin:         plugin,
		stdin:          transport.stdin,
		stdout:         bufio.NewReader(transport.stdout),
		stderr:         transport.stderr,
		shutdownGrace:  resolveDuration(plugin.Config.Timeouts.Shutdown, defaultShutdownGrace),
		cancelGrace:    resolveDuration(plugin.Config.Timeouts.CancelGrace, defaultCancelGrace),
		requestTimeout: resolveDuration(plugin.Config.Timeouts.RequestDefault, defaultRequestTimeout),
		redactor:       pluginredact.New(plugin.Config.Config, plugin.Config.Grants.Env),
		kill:           transport.kill,
		closeTransport: transport.close,
	}

	initializeCtx, cancel := context.WithTimeout(ctx, resolveDuration(plugin.Config.Timeouts.Launch, defaultRequestTimeout))
	defer cancel()
	initializeCtx, initializeCancel := session.withDefaultTimeout(initializeCtx)
	defer initializeCancel()

	var result protocol.InitializeResult
	err = session.Call(initializeCtx, protocol.MethodInitialize, protocol.InitializeParams{
		Protocol: protocol.Version{
			Name:  plugin.Manifest.Protocol.Name,
			Major: plugin.Manifest.Protocol.Major,
			Minor: plugin.Manifest.Protocol.Minor,
		},
		Mode:                config.Mode,
		SessionConfig:       cloneMap(config.SessionConfig),
		AllowedCapabilities: append([]string(nil), config.AllowedCapabilities...),
		Grants:              config.Grants,
	}, nil, &result)
	if err != nil {
		_ = session.Close(ctx)
		return nil, protocol.InitializeResult{}, err
	}

	verifyErr := session.verifyInitialize(result, config.AllowedCapabilities)
	return session, result, verifyErr
}

func (s *Session) Call(ctx context.Context, method string, params any, sink NotificationSink, result any) error {
	if s == nil {
		return errors.New("plugin session is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	callCtx, cancel := s.withDefaultTimeout(ctx)
	defer cancel()

	requestID := strconv.FormatUint(s.nextID.Add(1), 10)
	if err := protocol.WriteFrame(s.stdin, protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      requestID,
		Method:  method,
		Params:  params,
	}); err != nil {
		if callCtx.Err() != nil {
			return s.wrapProcessError(callCtx.Err())
		}
		return s.wrapProcessError(err)
	}

	cancelDone := make(chan struct{})
	defer close(cancelDone)
	go s.cancelOnContext(callCtx, requestID, cancelDone)

	for {
		raw, err := protocol.ReadFrame(s.stdout)
		if err != nil {
			if callCtx.Err() != nil {
				return s.wrapProcessError(callCtx.Err())
			}
			return s.wrapProcessError(err)
		}

		isNotification, response, err := decodeMessage(raw)
		if err != nil {
			if callCtx.Err() != nil {
				return s.wrapProcessError(callCtx.Err())
			}
			return s.wrapProcessError(err)
		}

		if isNotification {
			var request protocol.Request
			if err := json.Unmarshal(raw, &request); err != nil {
				return s.wrapProcessError(err)
			}
			s.handleNotification(request, sink)
			continue
		}

		if response.ID != requestID {
			return s.wrapProcessError(fmt.Errorf("unexpected response id %q", response.ID))
		}
		if response.Error != nil {
			return &CallError{Response: *response.Error}
		}
		if result == nil || response.Result == nil {
			return nil
		}

		resolved, err := json.Marshal(response.Result)
		if err != nil {
			return s.wrapProcessError(err)
		}
		if err := json.Unmarshal(resolved, result); err != nil {
			return s.wrapProcessError(err)
		}

		return nil
	}
}

func (s *Session) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, s.shutdownGrace)
	defer cancel()

	_ = s.Call(ctx, protocol.MethodShutdown, map[string]any{}, nil, nil)
	return s.close(ctx)
}

func (e *CallError) Error() string {
	return e.Response.Message
}

func (e *CallError) TheaterCode() string {
	return e.Response.Data.TheaterCode
}

func (e *CallError) PartialOutputs() map[string]any {
	return cloneMap(e.Response.Data.PartialOutputs)
}

func (e *CallError) Redact(redact func(string) string) *CallError {
	if e == nil || redact == nil {
		return e
	}

	cloned := *e
	cloned.Response.Message = redact(cloned.Response.Message)
	return &cloned
}

func (s *Session) cancelOnContext(ctx context.Context, id string, done <-chan struct{}) {
	if ctx == nil {
		return
	}

	select {
	case <-ctx.Done():
	case <-done:
		return
	}

	_ = protocol.WriteFrame(s.stdin, protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		Method:  protocol.MethodCancel,
		Params:  protocol.CancelParams{ID: id},
	})

	timer := time.NewTimer(s.cancelGrace)
	defer timer.Stop()

	select {
	case <-done:
		return
	case <-timer.C:
	}

	if s.kill != nil {
		_ = s.kill()
	}
}

func (s *Session) handleNotification(request protocol.Request, sink NotificationSink) {
	if sink == nil {
		return
	}

	switch request.Method {
	case protocol.MethodLog:
		var params protocol.LogParams
		if decodeParams(request.Params, &params) == nil {
			params.Message = s.redactor.RedactText(params.Message)
			params.Fields = s.redactor.RedactFields(params.Fields)
			sink.Log(params)
		}
	case protocol.MethodProgress:
		var params protocol.ProgressParams
		if decodeParams(request.Params, &params) == nil {
			params.Message = s.redactor.RedactText(params.Message)
			params.Phase = s.redactor.RedactText(params.Phase)
			params.Unit = s.redactor.RedactText(params.Unit)
			sink.Progress(params)
		}
	}
}

func (s *Session) verifyInitialize(result protocol.InitializeResult, allowedCapabilities []string) error {
	if result.Plugin.ID != s.plugin.Manifest.Plugin.ID {
		return s.wrapProcessError(
			fmt.Errorf("plugin id mismatch: got %q want %q", result.Plugin.ID, s.plugin.Manifest.Plugin.ID),
		)
	}
	if result.Plugin.Version != s.plugin.Manifest.Plugin.Version {
		return s.wrapProcessError(
			fmt.Errorf("plugin version mismatch: got %q want %q", result.Plugin.Version, s.plugin.Manifest.Plugin.Version),
		)
	}
	if result.Protocol.Name != s.plugin.Manifest.Protocol.Name {
		return s.wrapProcessError(
			fmt.Errorf("plugin protocol name mismatch: got %q want %q", result.Protocol.Name, s.plugin.Manifest.Protocol.Name),
		)
	}
	if result.Protocol.Major != s.plugin.Manifest.Protocol.Major {
		return s.wrapProcessError(
			fmt.Errorf("plugin protocol major mismatch: got %d want %d", result.Protocol.Major, s.plugin.Manifest.Protocol.Major),
		)
	}
	if result.DescriptorDigest != s.plugin.Manifest.DescriptorDigest {
		return s.wrapProcessError(
			fmt.Errorf(
				"plugin descriptor digest mismatch: got %q want %q",
				result.DescriptorDigest,
				s.plugin.Manifest.DescriptorDigest,
			),
		)
	}

	allowed := make(map[string]struct{}, len(allowedCapabilities))
	for _, capability := range allowedCapabilities {
		allowed[capability] = struct{}{}
	}
	for _, capability := range result.ActiveCapabilities {
		if _, ok := allowed[capability]; !ok {
			return s.wrapProcessError(fmt.Errorf("plugin advertised non-allowed capability %q", capability))
		}
	}

	return nil
}

func (s *Session) withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, s.requestTimeout)
}

func (s *Session) close(ctx context.Context) error {
	if s.closeTransport == nil {
		return nil
	}
	return s.closeTransport(ctx)
}

func (s *Session) wrapProcessError(err error) error {
	if err == nil {
		return nil
	}
	if s == nil {
		return err
	}

	stderr := ""
	if s.stderr != nil {
		stderr = strings.TrimSpace(s.stderr.String())
	}
	if stderr == "" {
		return err
	}
	if len(stderr) > maxStderrBytes {
		stderr = stderr[:maxStderrBytes] + "…"
	}
	stderr = s.redactor.RedactText(stderr)

	return fmt.Errorf("%w: %s", err, stderr)
}

func decodeMessage(raw []byte) (bool, protocol.Response, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false, protocol.Response{}, err
	}

	if _, ok := envelope["method"]; ok {
		return true, protocol.Response{}, nil
	}

	var response protocol.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		return false, protocol.Response{}, err
	}

	return false, response, nil
}

func decodeParams(raw, target any) error {
	if raw == nil {
		return nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	return json.Unmarshal(encoded, target)
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}

	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}

	return cloned
}

func resolveDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}
