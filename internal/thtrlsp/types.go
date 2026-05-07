package thtrlsp

import "encoding/json"

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type lspTextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type lspVersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type lspTextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type lspTextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type lspDidOpenTextDocumentParams struct {
	TextDocument lspTextDocumentItem `json:"textDocument"`
}

type lspDidChangeTextDocumentParams struct {
	TextDocument   lspVersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []lspTextDocumentContentChangeEvent `json:"contentChanges"`
}

type lspDidCloseTextDocumentParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
}

type lspDidSaveTextDocumentParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
}

type lspCompletionParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
	Position     lspPosition               `json:"position"`
}

type lspHoverParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
	Position     lspPosition               `json:"position"`
}

type lspSignatureHelpParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
	Position     lspPosition               `json:"position"`
}

type lspDocumentFormattingParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
}

type lspSemanticTokensParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
}

type lspCompletionItem struct {
	Label  string `json:"label"`
	Kind   int    `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type lspHover struct {
	Contents lspMarkupContent `json:"contents"`
	Range    *lspRange        `json:"range,omitempty"`
}

type lspMarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type lspSignatureHelp struct {
	Signatures      []lspSignatureInformation `json:"signatures"`
	ActiveSignature int                       `json:"activeSignature"`
	ActiveParameter int                       `json:"activeParameter,omitempty"`
}

type lspSignatureInformation struct {
	Label         string                    `json:"label"`
	Documentation string                    `json:"documentation,omitempty"`
	Parameters    []lspParameterInformation `json:"parameters,omitempty"`
}

type lspParameterInformation struct {
	Label string `json:"label"`
}

type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type lspSemanticTokens struct {
	Data []int `json:"data"`
}

type jsonrpcRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type lspDocument struct {
	URI           string
	Path          string
	Text          string
	Version       int
	PublishedURIs map[string]struct{}
}

type completionCandidate struct {
	Label  string
	Kind   int
	Detail string
}
