package thtrlsp

import "testing"

func testLanguageSupport(t *testing.T) languageSupport {
	t.Helper()

	support, err := newLanguageSupport("", "")
	if err != nil {
		t.Fatalf("build language support: %v", err)
	}

	return support
}

func testCapabilityCompletions(t *testing.T) []completionCandidate {
	t.Helper()

	support := testLanguageSupport(t)
	candidates := capabilityCompletionsForFamilies(support.capabilities)
	candidates = append(candidates, stateVerbCapabilityCompletions()...)
	return candidates
}

func testCompletionItemsForDocument(t *testing.T, text string, position lspPosition) []lspCompletionItem {
	t.Helper()

	return completionItemsForDocumentWithCapabilities(text, position, testLanguageSupport(t).capabilities)
}

func testAnalyzeDocument(t *testing.T, path, text string) map[string][]lspDiagnostic {
	t.Helper()

	return analyzeDocumentWithSupportAndOverlays(path, text, testLanguageSupport(t), nil)
}
