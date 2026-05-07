package thtrlsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
)

const (
	jsonrpcVersion       = "2.0"
	textDocumentSyncFull = 1
)

var errServerExit = errors.New("server exit")

// Run serves one stdio LSP session.
//
//nolint:contextcheck // theater.Validator currently exposes only a context-free Validate API used by diagnostics analysis.
func Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	_ = ctx
	support, err := newLanguageSupportFromEnvironment()
	if err != nil {
		return err
	}
	server := &server{
		reader:  bufio.NewReader(stdin),
		writer:  stdout,
		docs:    make(map[string]lspDocument),
		support: support,
	}
	return server.run()
}

type server struct {
	reader            *bufio.Reader
	writer            io.Writer
	docs              map[string]lspDocument
	support           languageSupport
	shutdownRequested bool
}

func (s *server) run() error {
	for {
		payload, err := s.readMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var request jsonrpcRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			continue
		}
		if request.Method == "" {
			continue
		}

		if err := s.handle(request); err != nil {
			if errors.Is(err, errServerExit) {
				return nil
			}
			return err
		}
	}
}

func (s *server) handle(request jsonrpcRequest) error {
	switch request.Method {
	case "initialize":
		return s.handleInitialize(request)
	case "initialized":
		return nil
	case "shutdown":
		return s.handleShutdown(request)
	case "exit":
		return errServerExit
	case "textDocument/didOpen":
		return s.handleDidOpen(request)
	case "textDocument/didChange":
		return s.handleDidChange(request)
	case "textDocument/didClose":
		return s.handleDidClose(request)
	case "textDocument/didSave":
		return s.handleDidSave(request)
	case "textDocument/completion":
		return s.handleCompletion(request)
	case "textDocument/hover":
		return s.handleHover(request)
	case "textDocument/signatureHelp":
		return s.handleSignatureHelp(request)
	case "textDocument/formatting":
		return s.handleFormatting(request)
	case "textDocument/semanticTokens/full":
		return s.handleSemanticTokens(request)
	default:
		return s.handleUnknownMethod(request)
	}
}

func (s *server) didOpen(params lspDidOpenTextDocumentParams) error {
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return err
	}

	s.docs[params.TextDocument.URI] = lspDocument{
		URI:           params.TextDocument.URI,
		Path:          path,
		Text:          params.TextDocument.Text,
		Version:       params.TextDocument.Version,
		PublishedURIs: make(map[string]struct{}),
	}
	if err := s.publishDocumentDiagnostics(params.TextDocument.URI); err != nil {
		return err
	}
	return s.publishDependentDiagnostics(params.TextDocument.URI)
}

func (s *server) didChange(params lspDidChangeTextDocumentParams) error {
	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return nil
	}
	if len(params.ContentChanges) == 0 {
		return nil
	}

	document.Text = params.ContentChanges[len(params.ContentChanges)-1].Text
	document.Version = params.TextDocument.Version
	s.docs[params.TextDocument.URI] = document
	if err := s.publishDocumentDiagnostics(params.TextDocument.URI); err != nil {
		return err
	}
	return s.publishDependentDiagnostics(params.TextDocument.URI)
}

func (s *server) didClose(params lspDidCloseTextDocumentParams) error {
	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return nil
	}

	for uri := range document.PublishedURIs {
		if err := s.publishDiagnostics(uri, nil); err != nil {
			return err
		}
	}
	delete(s.docs, params.TextDocument.URI)
	return s.publishDependentDiagnostics(params.TextDocument.URI)
}

func (s *server) didSave(params lspDidSaveTextDocumentParams) error {
	if _, ok := s.docs[params.TextDocument.URI]; ok {
		if err := s.publishDocumentDiagnostics(params.TextDocument.URI); err != nil {
			return err
		}
	}

	return s.publishDependentDiagnostics(params.TextDocument.URI)
}

func (s *server) publishDocumentDiagnostics(uri string) error {
	document, ok := s.docs[uri]
	if !ok {
		return nil
	}

	grouped := analyzeDocumentWithSupportAndOverlays(document.Path, document.Text, s.support, s.sourceOverlays())
	nextPublished := make(map[string]struct{}, len(grouped))
	for path, diagnostics := range grouped {
		targetURI := uriFromPath(path)
		if path == document.Path {
			targetURI = document.URI
		}
		nextPublished[targetURI] = struct{}{}
		if err := s.publishDiagnostics(targetURI, diagnostics); err != nil {
			return err
		}
	}

	for targetURI := range document.PublishedURIs {
		if _, ok := nextPublished[targetURI]; ok {
			continue
		}
		if err := s.publishDiagnostics(targetURI, nil); err != nil {
			return err
		}
	}

	document.PublishedURIs = nextPublished
	s.docs[uri] = document
	return nil
}

func (s *server) sourceOverlays() map[string][]byte {
	if len(s.docs) == 0 {
		return nil
	}

	overlays := make(map[string][]byte, len(s.docs))
	for _, document := range s.docs {
		data := []byte(document.Text)
		overlays[document.Path] = data
		overlays[canonicalSourceOverlayPath(document.Path)] = data
	}
	return overlays
}

func (s *server) publishDependentDiagnostics(changedURI string) error {
	dependentSet := make(map[string]struct{})
	for uri, document := range s.docs {
		if uri == changedURI {
			continue
		}
		if _, ok := document.PublishedURIs[changedURI]; ok {
			dependentSet[uri] = struct{}{}
			continue
		}
		if documentShouldRevalidateForLibraryChange(document, changedURI) {
			dependentSet[uri] = struct{}{}
		}
	}

	dependents := make([]string, 0, len(dependentSet))
	for uri := range dependentSet {
		dependents = append(dependents, uri)
	}
	sort.Strings(dependents)

	for _, uri := range dependents {
		if err := s.publishDocumentDiagnostics(uri); err != nil {
			return err
		}
	}
	return nil
}

func documentShouldRevalidateForLibraryChange(document lspDocument, changedURI string) bool {
	changedPath, err := pathFromURI(changedURI)
	if err != nil {
		return false
	}

	return authoringthtr.LibraryChangeAffectsFlowDocument(document.Path, changedPath)
}

func (s *server) handleInitialize(request jsonrpcRequest) error {
	result := map[string]any{
		"capabilities": map[string]any{
			"textDocumentSync": map[string]any{
				"openClose": true,
				"change":    textDocumentSyncFull,
				"save":      true,
			},
			"documentFormattingProvider": true,
			"completionProvider": map[string]any{
				"resolveProvider": false,
			},
			"hoverProvider": true,
			"signatureHelpProvider": map[string]any{
				"triggerCharacters": []string{"(", ","},
			},
			"semanticTokensProvider": map[string]any{
				"legend": map[string]any{
					"tokenTypes":     semanticTokenTypes,
					"tokenModifiers": []string{},
				},
				"full": true,
			},
		},
		"serverInfo": map[string]any{
			"name":    "thtr-lsp",
			"version": "0.1.0",
		},
	}
	return s.writeResponse(request.ID, result, nil)
}

func (s *server) handleShutdown(request jsonrpcRequest) error {
	s.shutdownRequested = true
	return s.writeResponse(request.ID, nil, nil)
}

func (s *server) handleDidOpen(request jsonrpcRequest) error {
	var params lspDidOpenTextDocumentParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	return s.didOpen(params)
}

func (s *server) handleDidChange(request jsonrpcRequest) error {
	var params lspDidChangeTextDocumentParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	return s.didChange(params)
}

func (s *server) handleDidClose(request jsonrpcRequest) error {
	var params lspDidCloseTextDocumentParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	return s.didClose(params)
}

func (s *server) handleDidSave(request jsonrpcRequest) error {
	var params lspDidSaveTextDocumentParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	return s.didSave(params)
}

func (s *server) handleCompletion(request jsonrpcRequest) error {
	var params lspCompletionParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return s.writeResponse(request.ID, []lspCompletionItem{}, nil)
	}

	items := completionItemsForDocumentWithCapabilities(document.Text, params.Position, s.support.capabilities)
	return s.writeResponse(request.ID, items, nil)
}

func (s *server) handleHover(request jsonrpcRequest) error {
	var params lspHoverParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return s.writeResponse(request.ID, nil, nil)
	}

	hover := hoverForDocument(document.Text, params.Position, s.support.capabilities)
	return s.writeResponse(request.ID, hover, nil)
}

func (s *server) handleSignatureHelp(request jsonrpcRequest) error {
	var params lspSignatureHelpParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return s.writeResponse(request.ID, lspSignatureHelp{}, nil)
	}

	help := signatureHelpForDocument(document.Text, params.Position, s.support.capabilities)
	return s.writeResponse(request.ID, help, nil)
}

func (s *server) handleFormatting(request jsonrpcRequest) error {
	var params lspDocumentFormattingParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return s.writeResponse(request.ID, []lspTextEdit{}, nil)
	}

	edits, err := formatDocument(document)
	if err != nil {
		return s.writeResponse(request.ID, nil, &jsonrpcError{Code: -32603, Message: err.Error()})
	}
	return s.writeResponse(request.ID, edits, nil)
}

func (s *server) handleSemanticTokens(request jsonrpcRequest) error {
	var params lspSemanticTokensParams
	if err := s.unmarshalParams(request, &params); err != nil {
		return err
	}

	document, ok := s.docs[params.TextDocument.URI]
	if !ok {
		return s.writeResponse(request.ID, lspSemanticTokens{}, nil)
	}

	return s.writeResponse(request.ID, semanticTokensForDocument(document.Text), nil)
}

func (s *server) handleUnknownMethod(request jsonrpcRequest) error {
	if len(request.ID) == 0 {
		return nil
	}

	return s.writeResponse(request.ID, nil, &jsonrpcError{Code: -32601, Message: "method not found"})
}

func (s *server) unmarshalParams(request jsonrpcRequest, target any) error {
	if err := json.Unmarshal(request.Params, target); err != nil {
		return s.writeInvalidParamsIfRequest(request.ID, err)
	}

	return nil
}

func (s *server) publishDiagnostics(uri string, diagnostics []lspDiagnostic) error {
	if diagnostics == nil {
		diagnostics = []lspDiagnostic{}
	}

	return s.writeNotification("textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": diagnostics,
	})
}

func formatDocument(document lspDocument) ([]lspTextEdit, error) {
	formatted, err := authoringFormat(document.Text)
	if err != nil {
		return nil, err
	}
	if formatted == document.Text {
		return []lspTextEdit{}, nil
	}

	return []lspTextEdit{
		{
			Range:   rangeForOffsets(document.Text, 0, len(document.Text)),
			NewText: formatted,
		},
	}, nil
}

func authoringFormat(text string) (string, error) {
	formatted, err := authoringFormatBytes([]byte(text))
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

func (s *server) writeInvalidParamsIfRequest(id json.RawMessage, err error) error {
	if len(id) == 0 {
		return nil
	}

	return s.writeResponse(id, nil, &jsonrpcError{Code: -32602, Message: err.Error()})
}

func (s *server) writeNotification(method string, params any) error {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": jsonrpcVersion,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}

	return s.writePayload(payload)
}

func (s *server) writeResponse(id json.RawMessage, result any, responseErr *jsonrpcError) error {
	response := jsonrpcResponse{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Result:  result,
		Error:   responseErr,
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}

	return s.writePayload(payload)
}

func (s *server) writePayload(payload []byte) error {
	if _, err := fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err := s.writer.Write(payload)
	return err
}

func (s *server) readMessage() ([]byte, error) {
	contentLength := 0
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" {
			break
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}

		length, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		contentLength = length
	}

	if contentLength <= 0 {
		return nil, io.EOF
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
