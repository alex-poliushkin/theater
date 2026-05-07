package thtrlsp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

func TestAnalyzeDocumentRewritesFlowLibraryDiagnostics(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "flows", "auth", "smoke.thtr"), "stage smoke\n")
	libraryPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "lib", "auth", "login.thtr"), `stage auth-lib

scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`)

	grouped := testAnalyzeDocument(t, flowPath, `stage smoke

call login-user = auth/login()
`)
	if got, want := len(grouped[flowPath]), 0; got != want {
		t.Fatalf("current file diagnostics mismatch: got %d want %d", got, want)
	}
	diagnostics := grouped[libraryPath]
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("library diagnostics count mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Code, "invalid_eventually_interval"; got != want {
		t.Fatalf("library diagnostic code mismatch: got %q want %q", got, want)
	}
}

func TestCompletionItemsForDocumentSuggestScenarioIDsInCall(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act submit
    do action.http()

call run = `

	items := testCompletionItemsForDocument(t, text, lspPosition{
		Line:      5,
		Character: len("call run = "),
	})
	if !containsCompletionLabel(items, "login") {
		t.Fatalf("expected scenario completion, got %#v", items)
	}
}

func TestCompletionItemsForDocumentSuggestActIDsInTransition(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act prepare
    do action.http()
  act submit
    do action.http()
    on pass -> `

	items := testCompletionItemsForDocument(t, text, lspPosition{
		Line:      6,
		Character: len("    on pass -> "),
	})
	if !containsCompletionLabel(items, "prepare") {
		t.Fatalf("expected act completion, got %#v", items)
	}
	if !containsCompletionLabel(items, "submit") {
		t.Fatalf("expected current act completion, got %#v", items)
	}
}

func TestCompletionItemsForDocumentSuggestExpectationNotMatcherCall(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act submit
    expect response: field(status_code) not assert expectation.n`

	items := testCompletionItemsForDocument(t, text, lspPosition{
		Line:      3,
		Character: len(`    expect response: field(status_code) not assert expectation.n`),
	})
	if !containsCompletionLabel(items, "expectation.not") {
		t.Fatalf("expected expectation.not completion, got %#v", items)
	}
}

func TestCompletionItemsForDocumentSuggestStateAliasConstructors(t *testing.T) {
	t.Parallel()

	recordText := `stage smoke
state
  record shared_meta = state.r`

	items := testCompletionItemsForDocument(t, recordText, lspPosition{
		Line:      2,
		Character: len(`  record shared_meta = state.r`),
	})
	if !containsCompletionLabel(items, "state.record") {
		t.Fatalf("expected state.record completion, got %#v", items)
	}

	poolText := `stage smoke
state
  pool otp_identities = state.p`

	items = testCompletionItemsForDocument(t, poolText, lspPosition{
		Line:      2,
		Character: len(`  pool otp_identities = state.p`),
	})
	if !containsCompletionLabel(items, "state.pool") {
		t.Fatalf("expected state.pool completion, got %#v", items)
	}
}

func TestCompletionItemsForDocumentSuggestStateBackendsTransformsAndArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		line      int
		character int
		want      string
	}{
		{
			name: "state backend ref",
			text: `stage smoke
state
  backend local = state.backend.
`,
			line:      2,
			character: len("  backend local = state.backend."),
			want:      "state.backend.file",
		},
		{
			name: "decorator transform ref",
			text: `stage smoke
scenario pipeline
  act load
    prop profile = inventory.http.get(url: "/profile") | json.
    do action.http(method: "GET", url: "/health")
`,
			line:      3,
			character: len(`    prop profile = inventory.http.get(url: "/profile") | json.`),
			want:      "json.decode",
		},
		{
			name: "required action argument",
			text: `stage smoke
scenario login
  act submit
    do action.http(m
`,
			line:      3,
			character: len("    do action.http(m"),
			want:      "method",
		},
		{
			name: "required action argument in multiline call",
			text: `stage smoke
scenario login
  act submit
    do action.http(
      m
    )
`,
			line:      4,
			character: len("      m"),
			want:      "method",
		},
		{
			name: "state alias backend id",
			text: `stage smoke
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(backend: l
`,
			line:      3,
			character: len("  record shared_meta = state.record(backend: l"),
			want:      "local",
		},
		{
			name: "selector step after expectation pipeline",
			text: `stage smoke
scenario pipeline
  act load
    expect payload: field(body) | p
`,
			line:      3,
			character: len(`    expect payload: field(body) | p`),
			want:      "path",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			items := testCompletionItemsForDocument(t, test.text, lspPosition{
				Line:      test.line,
				Character: test.character,
			})
			if !containsCompletionLabel(items, test.want) {
				t.Fatalf("expected %q completion, got %#v", test.want, items)
			}
		})
	}
}

func TestSignatureHelpForDocumentSupportsMultilineCalls(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act submit
    do action.http(
      method: "GET",
      url: "/health"
    )
`

	help := signatureHelpForDocument(text, lspPosition{
		Line:      4,
		Character: len(`      method: "GET",`),
	}, testLanguageSupport(t).capabilities)
	if got, want := len(help.Signatures), 1; got != want {
		t.Fatalf("signature count mismatch: got %d want %d", got, want)
	}
	if !strings.Contains(help.Signatures[0].Label, "action.http(") {
		t.Fatalf("signature label must describe action.http, got %#v", help.Signatures[0].Label)
	}
}

func TestCompletionItemsForDocumentSuggestStateVerbCalls(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario verify-state
  act claim-item
    do state.`

	items := testCompletionItemsForDocument(t, text, lspPosition{
		Line:      3,
		Character: len(`    do state.`),
	})
	for _, label := range []string{"state.read", "state.update", "state.claim", "state.renew", "state.release", "state.consume"} {
		if !containsCompletionLabel(items, label) {
			t.Fatalf("expected %s completion, got %#v", label, items)
		}
	}
}

func TestCompletionItemsForDocumentSuggestMatcherCallAtTrailingAssertPrefix(t *testing.T) {
	t.Parallel()

	text := `stage smoke
scenario login
  act submit
    expect response: field(status_code) not assert `

	items := testCompletionItemsForDocument(t, text, lspPosition{
		Line:      3,
		Character: len(`    expect response: field(status_code) not assert `),
	})
	if !containsCompletionLabel(items, "expectation.not") {
		t.Fatalf("expected expectation.not completion at trailing assert prefix, got %#v", items)
	}
}

func TestAnalyzeDocumentReportsCollectionWhereLowerDiagnostic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.thtr")
	grouped := testAnalyzeDocument(t, path, `stage main
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect bad: field(body) | decode(json) | path("/notifications") has item where field(status_code) == 200
`)

	diagnostics := grouped[path]
	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Code, "thtr_lower_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Message, `relative clause subject may start only with decode(...) or path(...)`; got != want {
		t.Fatalf("diagnostic message mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Range.Start.Line, 4; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
}

func TestAnalyzeDocumentReportsRemovedStateSurfaceDiagnostics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		text        string
		wantMessage string
		wantLine    int
	}{
		{
			name: "state cas",
			text: `stage main
scenario login
  act update
    do state.cas(expected_version: "1")
`,
			wantMessage: "state.cas has been removed; use state.update(... if_version: ...)",
			wantLine:    3,
		},
		{
			name: "state update expected version",
			text: `stage main
scenario login
  act update
    do state.update(expected_version: "1")
`,
			wantMessage: "state.update uses if_version; expected_version is the canonical action field",
			wantLine:    3,
		},
		{
			name: "state claim where",
			text: `stage main
scenario login
  act claim
    do state.claim
      where:
        purpose: "registration"
`,
			wantMessage: "state.claim where has been removed; use fields:",
			wantLine:    4,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "invalid.thtr")
			grouped := testAnalyzeDocument(t, path, tc.text)
			diagnostics := grouped[path]
			if got, want := len(diagnostics), 1; got != want {
				t.Fatalf("diagnostic count mismatch: got %d want %d", got, want)
			}
			if got, want := diagnostics[0].Code, "thtr_lower_error"; got != want {
				t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostics[0].Message, tc.wantMessage; got != want {
				t.Fatalf("diagnostic message mismatch: got %q want %q", got, want)
			}
			if got, want := diagnostics[0].Range.Start.Line, tc.wantLine; got != want {
				t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
			}
		})
	}
}

func TestSemanticTokensForDocumentReturnsTokenTypes(t *testing.T) {
	t.Parallel()

	text := `stage smoke
# note
scenario login
  act submit
    do action.http(method: "GET", timeout: 1s)
`

	tokens := semanticTokensForDocument(text)
	if len(tokens.Data) == 0 {
		t.Fatal("expected semantic tokens, got none")
	}
	if len(tokens.Data)%5 != 0 {
		t.Fatalf("semantic tokens data length must be divisible by 5, got %d", len(tokens.Data))
	}

	tokenTypes := make([]int, 0, len(tokens.Data)/5)
	for i := 3; i < len(tokens.Data); i += 5 {
		tokenTypes = append(tokenTypes, tokens.Data[i])
	}
	if !slices.Contains(tokenTypes, 0) {
		t.Fatalf("expected keyword token type, got %v", tokenTypes)
	}
	if !slices.Contains(tokenTypes, 1) {
		t.Fatalf("expected comment token type, got %v", tokenTypes)
	}
	if !slices.Contains(tokenTypes, 2) {
		t.Fatalf("expected string token type, got %v", tokenTypes)
	}
	if !slices.Contains(tokenTypes, 3) {
		t.Fatalf("expected number token type, got %v", tokenTypes)
	}
}

func TestFormatDocumentReturnsWholeDocumentEdit(t *testing.T) {
	t.Parallel()

	document := lspDocument{
		Text: `stage smoke
scenario login
  act submit
    do action.http(method:"GET",url:"/health")
`,
	}

	edits, err := formatDocument(document)
	if err != nil {
		t.Fatalf("format document failed: %v", err)
	}
	if got, want := len(edits), 1; got != want {
		t.Fatalf("format edit count mismatch: got %d want %d", got, want)
	}
	if edits[0].NewText == document.Text {
		t.Fatal("expected formatter to rewrite document")
	}
}

func TestFormatDocumentFormatsStateErgonomicsSurface(t *testing.T) {
	t.Parallel()

	document := lspDocument{
		Text: `stage smoke
state
  backend local = state.backend.file(root: "/tmp/theater-state")
  record shared_meta = state.record(backend: local,record: "env/shared-meta", min_guarantee: local-atomic)
  pool otp_identities = state.pool(backend: local,pool: "otp-identities", min_guarantee: local-atomic)
scenario verify-state
  act claim-item
    do state.claim(pool: otp_identities,lease: object { ttl: 5m })
`,
	}

	edits, err := formatDocument(document)
	if err != nil {
		t.Fatalf("format document failed: %v", err)
	}
	if got, want := len(edits), 1; got != want {
		t.Fatalf("format edit count mismatch: got %d want %d", got, want)
	}
	if !strings.Contains(edits[0].NewText, "record shared_meta = state.record\n    backend: local") {
		t.Fatalf("formatted document must normalize record alias block:\n%s", edits[0].NewText)
	}
	if !strings.Contains(edits[0].NewText, `do state.claim(pool: otp_identities, lease: object { ttl: 5m })`) {
		t.Fatalf("formatted document must normalize state.claim spacing:\n%s", edits[0].NewText)
	}
}

func TestRunServesInitializeAndDiagnostics(t *testing.T) {
	t.Parallel()

	docPath := filepath.Join(t.TempDir(), "invalid.thtr")
	docURI := uriFromPath(docPath)
	docText := `stage smoke
scenario login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`

	stdin := bytes.NewBuffer(nil)
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        docURI,
				"languageId": "thtr",
				"version":    1,
				"text":       docText,
			},
		},
	})
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "shutdown",
	})
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "exit",
	})

	stdout := bytes.NewBuffer(nil)
	if err := Run(context.Background(), stdin, stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	messages := readFramedMessages(t, stdout.Bytes())
	if got, want := len(messages), 3; got != want {
		t.Fatalf("message count mismatch: got %d want %d", got, want)
	}

	initialize := messages[0]
	if got, want := jsonNumberValue(t, initialize["id"]), int64(1); got != want {
		t.Fatalf("initialize response id mismatch: got %d want %d", got, want)
	}
	result, ok := initialize["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize response missing result: %#v", initialize)
	}
	capabilities, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("initialize response missing capabilities: %#v", initialize)
	}
	syncOptions, ok := capabilities["textDocumentSync"].(map[string]any)
	if !ok || syncOptions["save"] != true {
		t.Fatalf("initialize response missing save sync options: %#v", capabilities["textDocumentSync"])
	}
	if capabilities["hoverProvider"] != true {
		t.Fatalf("initialize response missing hover provider: %#v", capabilities)
	}
	signatureProvider, ok := capabilities["signatureHelpProvider"].(map[string]any)
	if !ok {
		t.Fatalf("initialize response missing signature help provider: %#v", capabilities)
	}
	triggerCharacters, ok := signatureProvider["triggerCharacters"].([]any)
	if !ok || !slices.Contains(triggerCharacters, "(") || !slices.Contains(triggerCharacters, ",") {
		t.Fatalf("initialize response signature triggers mismatch: %#v", signatureProvider)
	}

	publish := messages[1]
	if got, want := publish["method"], "textDocument/publishDiagnostics"; got != want {
		t.Fatalf("notification method mismatch: got %v want %v", got, want)
	}
	params, ok := publish["params"].(map[string]any)
	if !ok {
		t.Fatalf("publishDiagnostics params missing: %#v", publish)
	}
	if got, want := params["uri"], docURI; got != want {
		t.Fatalf("publishDiagnostics uri mismatch: got %v want %v", got, want)
	}
	diagnostics, ok := params["diagnostics"].([]any)
	if !ok || len(diagnostics) != 1 {
		t.Fatalf("publishDiagnostics diagnostics mismatch: %#v", params["diagnostics"])
	}
	diagnostic, ok := diagnostics[0].(map[string]any)
	if !ok {
		t.Fatalf("publishDiagnostics entry mismatch: %#v", diagnostics[0])
	}
	if got, want := diagnostic["code"], "invalid_eventually_interval"; got != want {
		t.Fatalf("publishDiagnostics code mismatch: got %v want %v", got, want)
	}

	shutdown := messages[2]
	if got, want := jsonNumberValue(t, shutdown["id"]), int64(2); got != want {
		t.Fatalf("shutdown response id mismatch: got %d want %d", got, want)
	}
}

func TestRunServesPluginDescriptorCompletionHoverAndSignature(t *testing.T) {
	configPath, lockPath := writeDescriptorOnlySmokeRegistry(t)
	t.Setenv(envPluginsConfig, configPath)
	t.Setenv(envPluginsLock, lockPath)

	docPath := filepath.Join(t.TempDir(), "plugin.thtr")
	docURI := uriFromPath(docPath)
	docText := `stage smoke
state
  backend smoke = state_backend.smoke.file(path: "/tmp/theater-state")
scenario plugin
  act echo
    do action.smoke.echo(value: "hello")
    expect echoed: field(echo) assert matcher.smoke.equal(expected: "hello")
  act partial
    do action.sm
  act transform
    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(prefix: "demo", suffix: "!")
    do action.smoke.echo(value: "hello")
`

	stdin := bytes.NewBuffer(nil)
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        docURI,
				"languageId": "thtr",
				"version":    1,
				"text":       docText,
			},
		},
	})
	writeFramedMessage(t, stdin, lspRequest(2, "textDocument/completion", docURI, lspPosition{
		Line:      8,
		Character: len("    do action.sm"),
	}))
	writeFramedMessage(t, stdin, lspRequest(3, "textDocument/hover", docURI, lspPosition{
		Line:      5,
		Character: len("    do action.smoke"),
	}))
	writeFramedMessage(t, stdin, lspRequest(4, "textDocument/signatureHelp", docURI, lspPosition{
		Line:      5,
		Character: len(`    do action.smoke.echo(`),
	}))
	writeFramedMessage(t, stdin, lspRequest(5, "textDocument/completion", docURI, lspPosition{
		Line:      6,
		Character: len(`    expect echoed: field(echo) assert matcher.sm`),
	}))
	writeFramedMessage(t, stdin, lspRequest(6, "textDocument/signatureHelp", docURI, lspPosition{
		Line:      6,
		Character: len(`    expect echoed: field(echo) assert matcher.smoke.equal(`),
	}))
	writeFramedMessage(t, stdin, lspRequest(7, "textDocument/completion", docURI, lspPosition{
		Line:      2,
		Character: len(`  backend smoke = state_backend.sm`),
	}))
	writeFramedMessage(t, stdin, lspRequest(8, "textDocument/signatureHelp", docURI, lspPosition{
		Line:      2,
		Character: len(`  backend smoke = state_backend.smoke.file(`),
	}))
	writeFramedMessage(t, stdin, lspRequest(9, "textDocument/completion", docURI, lspPosition{
		Line:      10,
		Character: len(`    prop wrapped = inventory.http.get(url: "/payload") | transform.sm`),
	}))
	writeFramedMessage(t, stdin, lspRequest(10, "textDocument/signatureHelp", docURI, lspPosition{
		Line:      10,
		Character: len(`    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(`),
	}))
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      11,
		"method":  "shutdown",
	})
	writeFramedMessage(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "exit",
	})

	stdout := bytes.NewBuffer(nil)
	if err := Run(context.Background(), stdin, stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	messages := readFramedMessages(t, stdout.Bytes())
	actionCompletion := responseResult[[]any](t, messages, 2)
	if !completionResponseContains(actionCompletion, "action.smoke.echo", "plugin action from smoke-plugin@0.2.0") {
		t.Fatalf("plugin action completion missing descriptor detail: %#v", actionCompletion)
	}

	hover := responseResult[map[string]any](t, messages, 3)
	hoverContents := hover["contents"].(map[string]any)
	if !strings.Contains(hoverContents["value"].(string), "Signature: `action.smoke.echo(value: string)`") {
		t.Fatalf("plugin action hover missing descriptor signature: %#v", hover)
	}

	actionSignature := responseResult[map[string]any](t, messages, 4)
	if !signatureResponseContains(actionSignature, "action.smoke.echo(value: string)") {
		t.Fatalf("plugin action signature help missing descriptor signature: %#v", actionSignature)
	}
	if got := jsonNumberValue(t, actionSignature["activeSignature"]); got != 0 {
		t.Fatalf("plugin action signature activeSignature mismatch: got %d want 0", got)
	}

	matcherCompletion := responseResult[[]any](t, messages, 5)
	if !completionResponseContains(matcherCompletion, "matcher.smoke.equal", "plugin matcher from smoke-plugin@0.2.0") {
		t.Fatalf("plugin matcher completion missing descriptor detail: %#v", matcherCompletion)
	}

	matcherSignature := responseResult[map[string]any](t, messages, 6)
	if !signatureResponseContains(matcherSignature, "matcher.smoke.equal(expected: string)") {
		t.Fatalf("plugin matcher signature help missing descriptor signature: %#v", matcherSignature)
	}

	stateBackendCompletion := responseResult[[]any](t, messages, 7)
	if !completionResponseContains(stateBackendCompletion, "state_backend.smoke.file", "plugin state backend from smoke-plugin@0.2.0") {
		t.Fatalf("plugin state backend completion missing descriptor detail: %#v", stateBackendCompletion)
	}

	stateBackendSignature := responseResult[map[string]any](t, messages, 8)
	if !signatureResponseContains(stateBackendSignature, "state_backend.smoke.file(path: string)") {
		t.Fatalf("plugin state backend signature help missing descriptor signature: %#v", stateBackendSignature)
	}

	transformCompletion := responseResult[[]any](t, messages, 9)
	if !completionResponseContains(transformCompletion, "transform.smoke.wrap", "plugin transform from smoke-plugin@0.2.0") {
		t.Fatalf("plugin transform completion missing descriptor detail: %#v", transformCompletion)
	}

	transformSignature := responseResult[map[string]any](t, messages, 10)
	if !signatureResponseContains(transformSignature, "transform.smoke.wrap(prefix: string, suffix: string)") {
		t.Fatalf("plugin transform signature help missing descriptor signature: %#v", transformSignature)
	}
}

func TestServerRevalidatesDependentFlowWhenLibraryDocumentChanges(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	libraryPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "lib", "auth", "login.thtr"), invalidEventuallyLibrarySource())
	flowURI := uriFromPath(flowPath)
	libraryURI := uriFromPath(libraryPath)

	stdout := bytes.NewBuffer(nil)
	s := &server{
		writer:  stdout,
		docs:    make(map[string]lspDocument),
		support: testLanguageSupport(t),
	}
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        flowURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       readTestFile(t, flowPath),
		},
	}); err != nil {
		t.Fatalf("open flow failed: %v", err)
	}

	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; !ok {
		t.Fatalf("flow diagnostics must track library publication target: %#v", s.docs[flowURI].PublishedURIs)
	}
	messages := readFramedMessages(t, stdout.Bytes())
	if !publishedDiagnosticsContain(messages, libraryURI, "invalid_eventually_interval") {
		t.Fatalf("expected initial library diagnostic publication, got %#v", messages)
	}

	stdout.Reset()
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        libraryURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       readTestFile(t, libraryPath),
		},
	}); err != nil {
		t.Fatalf("open library failed: %v", err)
	}

	validLibrary := validEventuallyLibrarySource()
	stdout.Reset()
	if err := s.didChange(lspDidChangeTextDocumentParams{
		TextDocument: lspVersionedTextDocumentIdentifier{
			URI:     libraryURI,
			Version: 2,
		},
		ContentChanges: []lspTextDocumentContentChangeEvent{{Text: validLibrary}},
	}); err != nil {
		t.Fatalf("change library failed: %v", err)
	}

	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; ok {
		t.Fatalf("flow diagnostics must clear stale library publication target: %#v", s.docs[flowURI].PublishedURIs)
	}
	messages = readFramedMessages(t, stdout.Bytes())
	if !publishedDiagnosticsEmpty(messages, libraryURI) {
		t.Fatalf("expected library diagnostics to be cleared after dependent revalidation, got %#v", messages)
	}
}

func TestServerRevalidatesDependentFlowWhenValidLibraryStartsFailing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	libraryPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "lib", "auth", "login.thtr"), validEventuallyLibrarySource())
	flowURI := uriFromPath(flowPath)
	libraryURI := uriFromPath(libraryPath)

	stdout := bytes.NewBuffer(nil)
	s := &server{
		writer:  stdout,
		docs:    make(map[string]lspDocument),
		support: testLanguageSupport(t),
	}
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        flowURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       readTestFile(t, flowPath),
		},
	}); err != nil {
		t.Fatalf("open flow failed: %v", err)
	}
	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; ok {
		t.Fatalf("valid flow should not publish library diagnostics yet: %#v", s.docs[flowURI].PublishedURIs)
	}

	stdout.Reset()
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        libraryURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       readTestFile(t, libraryPath),
		},
	}); err != nil {
		t.Fatalf("open library failed: %v", err)
	}

	invalidLibrary := invalidEventuallyLibrarySourceWithLeadingComment()
	stdout.Reset()
	if err := s.didChange(lspDidChangeTextDocumentParams{
		TextDocument: lspVersionedTextDocumentIdentifier{
			URI:     libraryURI,
			Version: 2,
		},
		ContentChanges: []lspTextDocumentContentChangeEvent{{Text: invalidLibrary}},
	}); err != nil {
		t.Fatalf("change library failed: %v", err)
	}

	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; !ok {
		t.Fatalf("flow diagnostics must start tracking newly invalid library: %#v", s.docs[flowURI].PublishedURIs)
	}
	messages := readFramedMessages(t, stdout.Bytes())
	if !publishedDiagnosticsContain(messages, libraryURI, "invalid_eventually_interval") {
		t.Fatalf("expected dependent flow to publish new library diagnostic, got %#v", messages)
	}
	if got, want := publishedDiagnosticLinesByCode(t, messages, libraryURI, "invalid_eventually_interval"), []int64{5, 5}; !slices.Equal(got, want) {
		t.Fatalf("overlay diagnostic lines mismatch: got %v want %v messages=%#v", got, want, messages)
	}
}

func TestServerRevalidatesDependentFlowWhenLibraryOverlayOpens(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	libraryPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "lib", "auth", "login.thtr"), validEventuallyLibrarySource())
	flowURI := uriFromPath(flowPath)
	libraryURI := uriFromPath(libraryPath)

	stdout := bytes.NewBuffer(nil)
	s := &server{
		writer:  stdout,
		docs:    make(map[string]lspDocument),
		support: testLanguageSupport(t),
	}
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        flowURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       readTestFile(t, flowPath),
		},
	}); err != nil {
		t.Fatalf("open flow failed: %v", err)
	}
	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; ok {
		t.Fatalf("valid flow should not publish library diagnostics yet: %#v", s.docs[flowURI].PublishedURIs)
	}

	invalidOverlay := invalidEventuallyLibrarySourceWithLeadingComment()
	stdout.Reset()
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        libraryURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       invalidOverlay,
		},
	}); err != nil {
		t.Fatalf("open dirty library overlay failed: %v", err)
	}

	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; !ok {
		t.Fatalf("flow diagnostics must start tracking dirty library overlay: %#v", s.docs[flowURI].PublishedURIs)
	}
	messages := readFramedMessages(t, stdout.Bytes())
	if !publishedDiagnosticsContain(messages, libraryURI, "invalid_eventually_interval") {
		t.Fatalf("expected dependent flow to publish dirty library diagnostic, got %#v", messages)
	}
	if got, want := publishedDiagnosticLinesByCode(t, messages, libraryURI, "invalid_eventually_interval"), []int64{5, 5}; !slices.Equal(got, want) {
		t.Fatalf("overlay diagnostic lines mismatch: got %v want %v messages=%#v", got, want, messages)
	}
}

func TestServerRevalidatesDependentFlowWhenLibraryOverlayCloses(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	flowPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "flows", "auth", "smoke.thtr"), `stage smoke

call login-user = auth/login()
`)
	libraryPath := writeTestFile(t, filepath.Join(repoRoot, "theater", "lib", "auth", "login.thtr"), validEventuallyLibrarySource())
	flowURI := uriFromPath(flowPath)
	libraryURI := uriFromPath(libraryPath)

	stdout := bytes.NewBuffer(nil)
	s := &server{
		writer:  stdout,
		docs:    make(map[string]lspDocument),
		support: testLanguageSupport(t),
	}
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        flowURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       readTestFile(t, flowPath),
		},
	}); err != nil {
		t.Fatalf("open flow failed: %v", err)
	}
	if err := s.didOpen(lspDidOpenTextDocumentParams{
		TextDocument: lspTextDocumentItem{
			URI:        libraryURI,
			LanguageID: "thtr",
			Version:    1,
			Text:       invalidEventuallyLibrarySourceWithLeadingComment(),
		},
	}); err != nil {
		t.Fatalf("open dirty library overlay failed: %v", err)
	}
	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; !ok {
		t.Fatalf("flow diagnostics must track dirty library overlay before close: %#v", s.docs[flowURI].PublishedURIs)
	}

	stdout.Reset()
	if err := s.didClose(lspDidCloseTextDocumentParams{
		TextDocument: lspTextDocumentIdentifier{URI: libraryURI},
	}); err != nil {
		t.Fatalf("close dirty library overlay failed: %v", err)
	}

	if _, ok := s.docs[flowURI].PublishedURIs[libraryURI]; ok {
		t.Fatalf("flow diagnostics must clear library diagnostics after overlay closes: %#v", s.docs[flowURI].PublishedURIs)
	}
	messages := readFramedMessages(t, stdout.Bytes())
	if !publishedDiagnosticsEmpty(messages, libraryURI) {
		t.Fatalf("expected library diagnostics to clear after dirty overlay closes, got %#v", messages)
	}
}

func TestAnalyzeDocumentUsesConfiguredPluginDescriptorsForDiagnostics(t *testing.T) {
	t.Parallel()

	configPath, lockPath := writeDescriptorOnlySmokeRegistry(t)
	support, err := newLanguageSupport(configPath, lockPath)
	if err != nil {
		t.Fatalf("new language support failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "plugin.thtr")
	diagnostics := analyzeDocumentWithSupportAndOverlays(path, `stage smoke
scenario plugin
  act echo
    do action.smoke.echo()
`, support, nil)[path]

	if got, want := len(diagnostics), 1; got != want {
		t.Fatalf("diagnostic count mismatch: got %d want %d diagnostics=%#v", got, want, diagnostics)
	}
	if got, want := diagnostics[0].Code, "missing_action_arg"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Message, `action input "value" is required`; got != want {
		t.Fatalf("diagnostic message mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostics[0].Range.Start.Line, 3; got != want {
		t.Fatalf("diagnostic line mismatch: got %d want %d", got, want)
	}
	if got, want := diagnostics[0].Range.Start.Character, len("    "); got != want {
		t.Fatalf("diagnostic start character mismatch: got %d want %d", got, want)
	}
	if !lspPositionAfter(diagnostics[0].Range.End, diagnostics[0].Range.Start) {
		t.Fatalf("diagnostic range must be non-empty: %#v", diagnostics[0].Range)
	}
}

func TestNewLanguageSupportRequiresPluginConfigAndLockPair(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pluginsConfig string
		pluginsLock   string
		want          string
	}{
		{
			name:        "lock without config",
			pluginsLock: "plugins.lock.json",
			want:        "thtr-lsp requires THEATER_PLUGINS_CONFIG when THEATER_PLUGINS_LOCK is set",
		},
		{
			name:          "config without lock",
			pluginsConfig: "plugins.json",
			want:          "thtr-lsp requires THEATER_PLUGINS_LOCK when THEATER_PLUGINS_CONFIG is set",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := newLanguageSupport(test.pluginsConfig, test.pluginsLock)
			if err == nil {
				t.Fatal("expected incomplete plugin environment error, got nil")
			}
			if got := err.Error(); got != test.want {
				t.Fatalf("error mismatch: got %q want %q", got, test.want)
			}
		})
	}
}

func containsCompletionLabel(items []lspCompletionItem, label string) bool {
	for i := range items {
		if items[i].Label == label {
			return true
		}
	}

	return false
}

func writeTestFile(t *testing.T, path, contents string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(data)
}

func invalidEventuallyLibrarySource() string {
	return `stage auth-lib

scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`
}

func validEventuallyLibrarySource() string {
	return `stage auth-lib

scenario auth/login
  act submit
    eventually 2s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`
}

func invalidEventuallyLibrarySourceWithLeadingComment() string {
	return `stage auth-lib

# unsaved editor overlay
scenario auth/login
  act submit
    eventually 1s every 1s
    do repeatable action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`
}

func writeFramedMessage(t *testing.T, buffer *bytes.Buffer, payload any) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if _, err := fmt.Fprintf(buffer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		t.Fatalf("write header failed: %v", err)
	}
	if _, err := buffer.Write(body); err != nil {
		t.Fatalf("write body failed: %v", err)
	}
}

func readFramedMessages(t *testing.T, payload []byte) []map[string]any {
	t.Helper()

	reader := bufio.NewReader(bytes.NewReader(payload))
	messages := make([]map[string]any, 0)
	for {
		contentLength := 0
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return messages
				}
				t.Fatalf("read header failed: %v", err)
			}
			if line == "\r\n" {
				break
			}

			name, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
				if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &contentLength); err != nil {
					t.Fatalf("parse content length failed: %v", err)
				}
			}
		}
		if contentLength == 0 {
			t.Fatal("missing content length in framed output")
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			t.Fatalf("read body failed: %v", err)
		}

		message := make(map[string]any)
		if err := json.Unmarshal(body, &message); err != nil {
			t.Fatalf("unmarshal output payload failed: %v", err)
		}
		messages = append(messages, message)
	}
}

func lspRequest(id int, method, uri string, position lspPosition) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position": map[string]any{
				"line":      position.Line,
				"character": position.Character,
			},
		},
	}
}

func responseResult[T any](t *testing.T, messages []map[string]any, id int64) T {
	t.Helper()

	for i := range messages {
		if messages[i]["id"] == nil {
			continue
		}
		if jsonNumberValue(t, messages[i]["id"]) != id {
			continue
		}
		result, ok := messages[i]["result"].(T)
		if !ok {
			t.Fatalf("response %d result type mismatch: %#v", id, messages[i]["result"])
		}
		return result
	}

	t.Fatalf("missing response id %d in %#v", id, messages)
	var zero T
	return zero
}

func completionResponseContains(items []any, label, detailSubstring string) bool {
	for i := range items {
		item, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		if item["label"] != label {
			continue
		}
		detail, _ := item["detail"].(string)
		return strings.Contains(detail, detailSubstring)
	}

	return false
}

func publishedDiagnosticsContain(messages []map[string]any, uri, code string) bool {
	for i := range messages {
		if messages[i]["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params, ok := messages[i]["params"].(map[string]any)
		if !ok || params["uri"] != uri {
			continue
		}
		diagnostics, ok := params["diagnostics"].([]any)
		if !ok {
			continue
		}
		for j := range diagnostics {
			diagnostic, ok := diagnostics[j].(map[string]any)
			if ok && diagnostic["code"] == code {
				return true
			}
		}
	}

	return false
}

func publishedDiagnosticsEmpty(messages []map[string]any, uri string) bool {
	for i := range messages {
		if messages[i]["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params, ok := messages[i]["params"].(map[string]any)
		if !ok || params["uri"] != uri {
			continue
		}
		diagnostics, ok := params["diagnostics"].([]any)
		if !ok {
			continue
		}
		if len(diagnostics) == 0 {
			return true
		}
	}

	return false
}

func publishedDiagnosticLinesByCode(t *testing.T, messages []map[string]any, uri, code string) []int64 {
	t.Helper()

	lines := make([]int64, 0)
	for i := range messages {
		if messages[i]["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params, ok := messages[i]["params"].(map[string]any)
		if !ok || params["uri"] != uri {
			continue
		}
		diagnostics, ok := params["diagnostics"].([]any)
		if !ok {
			continue
		}
		for j := range diagnostics {
			diagnostic, ok := diagnostics[j].(map[string]any)
			if !ok || diagnostic["code"] != code {
				continue
			}
			rng, ok := diagnostic["range"].(map[string]any)
			if !ok {
				t.Fatalf("diagnostic range missing: %#v", diagnostic)
			}
			start, ok := rng["start"].(map[string]any)
			if !ok {
				t.Fatalf("diagnostic range start missing: %#v", rng)
			}
			lines = append(lines, jsonNumberValue(t, start["line"]))
		}
	}
	return lines
}

func signatureResponseContains(response map[string]any, label string) bool {
	signatures, ok := response["signatures"].([]any)
	if !ok {
		return false
	}
	for i := range signatures {
		signature, ok := signatures[i].(map[string]any)
		if !ok {
			continue
		}
		if signature["label"] == label {
			return true
		}
	}

	return false
}

func writeDescriptorOnlySmokeRegistry(t *testing.T) (string, string) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	manifestPath := filepath.Join(root, "testdata", "plugins", "smoke", "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read smoke manifest: %v", err)
	}

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "plugins.json")
	lockPath := filepath.Join(tempDir, "plugins.lock.json")
	writeTestJSONFile(t, configPath, pluginregistry.ConfigFile{
		Schema: pluginregistry.ConfigSchemaVersion,
		Plugins: map[string]pluginregistry.PluginEntry{
			"smoke-plugin": {
				Manifest: manifestPath,
				Exec: pluginregistry.ExecSpec{
					Command: []string{filepath.Join(tempDir, "missing-plugin-executable")},
				},
				AllowCapabilities: []string{
					"action.smoke.echo",
					"state_backend.smoke.file",
					"transform.smoke.wrap",
					"matcher.smoke.equal",
				},
			},
		},
	})
	writeTestJSONFile(t, lockPath, pluginregistry.LockFile{
		Schema: pluginregistry.LockSchemaVersion,
		Plugins: map[string]pluginregistry.LockEntry{
			"smoke-plugin": {
				ManifestSHA256:   sha256Digest(manifestRaw),
				ExecutableSHA256: "sha256:descriptor-lsp-must-not-read-this",
			},
		},
	})

	return configPath, lockPath
}

func writeTestJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode JSON %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write JSON %s: %v", path, err)
	}
}

func sha256Digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func jsonNumberValue(t *testing.T, value any) int64 {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("expected JSON number, got %#v", value)
	}
	return int64(number)
}
