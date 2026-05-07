package thtrlsp

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alex-poliushkin/theater"
	authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"
)

const (
	blockKindAct      = "act"
	blockKindCall     = "call"
	blockKindHTTP     = "http"
	blockKindScenario = "scenario"
	blockKindState    = "state"
	blockKindTop      = "top"
	diagnosticsSource = "thtr-lsp"
)

func analyzeDocumentWithSupportAndOverlays(
	path string,
	text string,
	support languageSupport,
	sourceOverlays map[string][]byte,
) map[string][]lspDiagnostic {
	result, err := authoringthtr.AnalyzePathDetailedWithLibraryOverlay(path, []byte(text), support.sugar, sourceOverlays)
	if err != nil {
		return diagnosticsFromLoadError(path, text, err, sourceOverlays)
	}

	validator := theater.NewValidator(support.catalog, support.matchers)
	diagnostics := result.RewriteDiagnostics(validator.Validate(result.Spec))
	return diagnosticsByFile(path, text, diagnostics, sourceOverlays)
}

func diagnosticsFromLoadError(path, text string, err error, sourceOverlays map[string][]byte) map[string][]lspDiagnostic {
	var diagnosticError *authoringthtr.DiagnosticError
	if errors.As(err, &diagnosticError) {
		diagnostic := diagnosticError.Diagnostic()
		return diagnosticsByFile(path, text, []theater.Diagnostic{diagnostic}, sourceOverlays)
	}

	return map[string][]lspDiagnostic{
		path: {
			{
				Range:    lspRange{Start: lspPosition{}, End: lspPosition{}},
				Severity: 1,
				Source:   diagnosticsSource,
				Message:  err.Error(),
			},
		},
	}
}

func diagnosticsByFile(
	currentPath string,
	currentText string,
	diagnostics []theater.Diagnostic,
	sourceOverlays map[string][]byte,
) map[string][]lspDiagnostic {
	grouped := make(map[string][]lspDiagnostic)
	for i := range diagnostics {
		diagnostic := diagnostics[i]
		path := diagnostic.Span.File
		if path == "" {
			path = currentPath
		}

		grouped[path] = append(grouped[path], convertDiagnostic(path, currentPath, currentText, sourceOverlays, diagnostic))
	}

	if _, ok := grouped[currentPath]; !ok {
		grouped[currentPath] = nil
	}

	return grouped
}

func convertDiagnostic(
	path string,
	currentPath string,
	currentText string,
	sourceOverlays map[string][]byte,
	diagnostic theater.Diagnostic,
) lspDiagnostic {
	text := diagnosticText(path, currentPath, currentText, sourceOverlays)

	start := 0
	end := 0
	if diagnostic.Span.Line > 0 && diagnostic.Span.Column > 0 {
		start = offsetForLineColumn(text, diagnostic.Span.Line, diagnostic.Span.Column)
		end = offsetForLineColumn(text, diagnostic.Span.Line, diagnostic.Span.Column+1)
		start, end = diagnosticRangeOffsets(text, start, end)
	}

	return lspDiagnostic{
		Range:    rangeForOffsets(text, start, end),
		Severity: 1,
		Code:     diagnostic.Code,
		Source:   diagnosticsSource,
		Message:  diagnostic.Summary,
	}
}

func diagnosticText(path, currentPath, currentText string, sourceOverlays map[string][]byte) string {
	if path == currentPath {
		return currentText
	}
	if overlay, ok := sourceOverlays[path]; ok {
		return string(overlay)
	}
	if overlay, ok := sourceOverlays[canonicalSourceOverlayPath(path)]; ok {
		return string(overlay)
	}
	return fileText(path)
}

func canonicalSourceOverlayPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolvedPath, err := filepath.EvalSymlinks(absPath); err == nil {
		return resolvedPath
	}
	return absPath
}

func diagnosticRangeOffsets(text string, start, end int) (rangeStart, rangeEnd int) {
	start = clampOffset(text, start)
	end = clampOffset(text, end)
	if end > start {
		return start, end
	}
	if text == "" {
		return 0, 0
	}
	if start >= len(text) {
		return lastContentOffsetRange(text)
	}

	return start, nextDiagnosticOffset(text, start)
}

func lastContentOffsetRange(text string) (rangeStart, rangeEnd int) {
	for end := len(text); end > 0; {
		r, size := utf8.DecodeLastRuneInString(text[:end])
		if r == utf8.RuneError && size == 0 {
			break
		}

		start := end - size
		if r != '\n' && r != '\r' {
			return start, end
		}
		end = start
	}

	return 0, 1
}

func nextDiagnosticOffset(text string, start int) int {
	if start < 0 {
		start = 0
	}
	if start >= len(text) {
		if text == "" {
			return 0
		}
		return len(text)
	}

	_, size := utf8.DecodeRuneInString(text[start:])
	if size <= 0 {
		return start + 1
	}
	return start + size
}

func actionCapabilityCompletions(descriptors []theater.CapabilityDescriptor) []completionCandidate {
	candidates := capabilityCompletionsForFamilies(descriptors, theater.CapabilityFamilyAction)
	candidates = append(candidates, stateVerbCapabilityCompletions()...)
	return candidates
}

func matcherCapabilityCompletions(descriptors []theater.CapabilityDescriptor) []completionCandidate {
	return capabilityCompletionsForFamilies(descriptors, theater.CapabilityFamilyMatcher)
}

func inventoryCapabilityCompletions(descriptors []theater.CapabilityDescriptor) []completionCandidate {
	return capabilityCompletionsForFamilies(descriptors, theater.CapabilityFamilyInventory)
}

func stateBackendCapabilityCompletions(descriptors []theater.CapabilityDescriptor) []completionCandidate {
	return capabilityCompletionsForFamilies(descriptors, theater.CapabilityFamilyStateBackend)
}

func transformCapabilityCompletions(descriptors []theater.CapabilityDescriptor) []completionCandidate {
	return capabilityCompletionsForFamilies(descriptors, theater.CapabilityFamilyTransform)
}

func generatorCapabilityCompletions(descriptors []theater.CapabilityDescriptor) []completionCandidate {
	return capabilityCompletionsForFamilies(descriptors, theater.CapabilityFamilyGenerator)
}

func capabilityCompletionsForFamilies(
	descriptors []theater.CapabilityDescriptor,
	families ...theater.CapabilityFamily,
) []completionCandidate {
	allowed := make(map[theater.CapabilityFamily]struct{}, len(families))
	for _, family := range families {
		allowed[family] = struct{}{}
	}

	candidates := make([]completionCandidate, 0, len(descriptors))
	for i := range descriptors {
		descriptor := descriptors[i]
		if len(allowed) != 0 {
			if _, ok := allowed[descriptor.Family]; !ok {
				continue
			}
		}
		candidates = append(candidates, completionCandidate{
			Label:  descriptorCompletionLabel(descriptor),
			Kind:   3,
			Detail: descriptorCompletionDetail(descriptor),
		})
	}

	return candidates
}

func descriptorCompletionLabel(descriptor theater.CapabilityDescriptor) string {
	if descriptor.Family == theater.CapabilityFamilyGenerator {
		return "generate." + descriptor.Ref
	}

	return descriptor.Ref
}

func descriptorCompletionDetail(descriptor theater.CapabilityDescriptor) string {
	family := strings.ReplaceAll(string(descriptor.Family), "-", " ")
	var detail string
	switch descriptor.Provider.Kind {
	case theater.CapabilityProviderPlugin:
		detail = "plugin " + family + " from " + descriptor.Provider.PluginID + "@" + descriptor.Provider.PluginVersion
	default:
		detail = "built-in " + family
	}
	if summary := strings.TrimSpace(descriptor.Summary); summary != "" {
		detail += ": " + summary
	}

	return detail
}

func stateVerbCapabilityCompletions() []completionCandidate {
	return []completionCandidate{
		{Label: "state.update", Kind: 3, Detail: "thtr state verb"},
		{Label: "state.claim", Kind: 3, Detail: "thtr state verb"},
		{Label: "state.consume", Kind: 3, Detail: "thtr state verb"},
		{Label: "state.read", Kind: 3, Detail: "thtr state verb"},
		{Label: "state.release", Kind: 3, Detail: "thtr state verb"},
		{Label: "state.renew", Kind: 3, Detail: "thtr state verb"},
	}
}

func stateAliasCallCompletions(sectionKind string) []completionCandidate {
	switch sectionKind {
	case "record":
		return []completionCandidate{
			{Label: "state.record", Kind: 3, Detail: "thtr state alias constructor"},
		}
	case "pool":
		return []completionCandidate{
			{Label: "state.pool", Kind: 3, Detail: "thtr state alias constructor"},
		}
	default:
		return nil
	}
}

func keywordCompletions(kind string) []completionCandidate {
	switch kind {
	case blockKindTop:
		return []completionCandidate{
			{Label: "stage", Kind: 14, Detail: "top-level keyword"},
			{Label: "name", Kind: 14, Detail: "top-level keyword"},
			{Label: "http", Kind: 14, Detail: "top-level keyword"},
			{Label: "state", Kind: 14, Detail: "top-level keyword"},
			{Label: "scenario", Kind: 14, Detail: "top-level keyword"},
			{Label: "call", Kind: 14, Detail: "top-level keyword"},
		}
	case blockKindScenario:
		return []completionCandidate{
			{Label: "name", Kind: 14, Detail: "scenario keyword"},
			{Label: "act", Kind: 14, Detail: "scenario keyword"},
		}
	case blockKindAct:
		return []completionCandidate{
			{Label: "name", Kind: 14, Detail: "act keyword"},
			{Label: "eventually", Kind: 14, Detail: "act keyword"},
			{Label: "prop", Kind: 14, Detail: "act keyword"},
			{Label: "do", Kind: 14, Detail: "act keyword"},
			{Label: "capture_auth", Kind: 14, Detail: "act keyword"},
			{Label: "expect", Kind: 14, Detail: "act keyword"},
			{Label: "export", Kind: 14, Detail: "act keyword"},
			{Label: "on", Kind: 14, Detail: "act keyword"},
		}
	case blockKindCall:
		return []completionCandidate{
			{Label: "name", Kind: 14, Detail: "scenario call keyword"},
			{Label: "dependency", Kind: 14, Detail: "scenario call keyword"},
			{Label: "export", Kind: 14, Detail: "scenario call keyword"},
		}
	case blockKindHTTP:
		return []completionCandidate{
			{Label: "session", Kind: 14, Detail: "http section keyword"},
			{Label: "auth", Kind: 14, Detail: "http section keyword"},
			{Label: "identity", Kind: 14, Detail: "http section keyword"},
		}
	case blockKindState:
		return []completionCandidate{
			{Label: "backend", Kind: 14, Detail: "state section keyword"},
			{Label: "record", Kind: 14, Detail: "state section keyword"},
			{Label: "pool", Kind: 14, Detail: "state section keyword"},
		}
	default:
		return nil
	}
}

func selectorCompletions() []completionCandidate {
	return []completionCandidate{
		{Label: "field", Kind: 14, Detail: "selector root"},
		{Label: "decode", Kind: 14, Detail: "selector step"},
		{Label: "path", Kind: 14, Detail: "selector step"},
		{Label: "pick", Kind: 14, Detail: "selector step"},
		{Label: "regexp", Kind: 14, Detail: "selector step"},
	}
}

func keywordSet() []string {
	return []string{
		"stage", "name", "scenario", "act", "eventually", "prop", "do",
		"capture_auth", "repeatable", "expect", "assert", "matches", "contains",
		"not", "has", "item", "all", "items", "where", "key",
		"export", "call", "dependency", "when", "on", "http", "state", "backend",
		"record", "pool", "read", "update", "claim", "renew", "release", "consume", "object",
		"list", "string", "field", "decode", "path", "pick", "regexp", "generate",
	}
}

func filterCandidates(candidates []completionCandidate, prefix string) []lspCompletionItem {
	if len(candidates) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(candidates))
	items := make([]lspCompletionItem, 0, len(candidates))
	for i := range candidates {
		candidate := candidates[i]
		if prefix != "" && !strings.HasPrefix(candidate.Label, prefix) {
			continue
		}
		if _, ok := seen[candidate.Label]; ok {
			continue
		}

		seen[candidate.Label] = struct{}{}
		items = append(items, lspCompletionItem(candidate))
	}

	slices.SortFunc(items, func(left, right lspCompletionItem) int {
		return strings.Compare(left.Label, right.Label)
	})
	return items
}

func currentLinePrefix(text string, position lspPosition) string {
	offset := offsetForPosition(text, position)
	lineStart := offset
	for lineStart > 0 && text[lineStart-1] != '\n' {
		lineStart--
	}
	return text[lineStart:offset]
}

func currentFragment(prefix string) string {
	end := len(prefix)
	start := end
	for start > 0 {
		ch := prefix[start-1]
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '.' || ch == '/' || ch == '-' {
			start--
			continue
		}
		break
	}

	return prefix[start:end]
}

func completionItemsForDocumentWithCapabilities(
	text string,
	position lspPosition,
	capabilities []theater.CapabilityDescriptor,
) []lspCompletionItem {
	prefix := currentLinePrefix(text, position)
	trimmedPrefix := strings.TrimSpace(prefix)
	fragment := currentFragment(prefix)
	statementPrefix := strings.TrimSpace(strings.TrimSuffix(prefix, fragment))
	lines := strings.Split(text, "\n")
	lineIndex := position.Line
	if lineIndex < 0 {
		lineIndex = 0
	}
	if lineIndex >= len(lines) {
		lineIndex = len(lines) - 1
	}

	candidates := stateBackendIDCompletionsForPrefix(lines, prefix)
	if len(candidates) == 0 {
		candidates = descriptorArgumentCompletionsForPosition(text, position, lines, lineIndex, prefix, capabilities)
	}
	if len(candidates) == 0 {
		candidates = completionCandidatesForPrefix(lines, trimmedPrefix, capabilities)
	}

	if strings.HasPrefix(fragment, "generate.") {
		candidates = append(candidates, generatorCapabilityCompletions(capabilities)...)
	}

	if len(candidates) == 0 && strings.TrimSpace(statementPrefix) == "" {
		candidates = append(candidates, keywordCompletions(completionBlockKind(lines, lineIndex))...)
	}
	if len(candidates) == 0 {
		candidates = append(candidates, selectorCompletions()...)
		candidates = append(candidates, keywordCompletions(completionBlockKind(lines, lineIndex))...)
	}

	return filterCandidates(candidates, fragment)
}

func completionCandidatesForPrefix(
	lines []string,
	trimmedPrefix string,
	capabilities []theater.CapabilityDescriptor,
) []completionCandidate {
	switch {
	case strings.HasPrefix(trimmedPrefix, "do ") || strings.HasPrefix(trimmedPrefix, "do repeatable "):
		return actionCapabilityCompletions(capabilities)
	case isStateBackendAssignmentPrefix(trimmedPrefix):
		return stateBackendCapabilityCompletions(capabilities)
	case isStateAliasAssignmentPrefix(trimmedPrefix, "record"):
		return stateAliasCallCompletions("record")
	case isStateAliasAssignmentPrefix(trimmedPrefix, "pool"):
		return stateAliasCallCompletions("pool")
	case hasPropertyPipelineTransformPrefix(trimmedPrefix):
		return transformCapabilityCompletions(capabilities)
	case strings.HasPrefix(trimmedPrefix, "prop ") && strings.Contains(trimmedPrefix, "="):
		return inventoryCapabilityCompletions(capabilities)
	case containsAssertCallPrefix(trimmedPrefix):
		return matcherCapabilityCompletions(capabilities)
	case strings.HasPrefix(trimmedPrefix, blockKindCall+" ") && strings.Contains(trimmedPrefix, "="):
		return scenarioIDCompletions(lines)
	case strings.HasPrefix(trimmedPrefix, "on ") && strings.Contains(trimmedPrefix, "->"):
		return actIDCompletions(lines)
	case strings.Contains(trimmedPrefix, "$"):
		return selectorCompletions()
	default:
		return nil
	}
}

func descriptorArgumentCompletionsForPosition(
	text string,
	position lspPosition,
	lines []string,
	lineIndex int,
	prefix string,
	descriptors []theater.CapabilityDescriptor,
) []completionCandidate {
	if isArgumentValuePrefix(prefix) {
		return nil
	}

	ref, _, ok := callRefBeforePosition(text, position)
	if !ok {
		ref, ok = blockCallRefBeforeLine(lines, lineIndex)
	}
	if !ok {
		return nil
	}

	descriptor, ok := descriptorByLabel(descriptors)[ref]
	if !ok {
		return nil
	}

	params := descriptorParameters(descriptor)
	candidates := make([]completionCandidate, 0, len(params))
	for i := range params {
		parameter := params[i]
		detail := "optional argument: " + parameter.valueType
		if parameter.required {
			detail = "required argument: " + parameter.valueType
		}
		candidates = append(candidates, completionCandidate{
			Label:  parameter.name,
			Kind:   5,
			Detail: detail,
		})
	}
	return candidates
}

func stateBackendIDCompletionsForPrefix(lines []string, prefix string) []completionCandidate {
	if !isBackendArgumentValuePrefix(prefix) {
		return nil
	}

	ids := collectStateBackendIDs(lines)
	candidates := make([]completionCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, completionCandidate{
			Label:  id,
			Kind:   6,
			Detail: "state backend id",
		})
	}
	return candidates
}

func containsAssertCallPrefix(statementPrefix string) bool {
	return strings.Contains(statementPrefix, " assert ") || strings.HasSuffix(statementPrefix, " assert")
}

func isStateBackendAssignmentPrefix(statementPrefix string) bool {
	return strings.HasPrefix(statementPrefix, "backend ") && strings.Contains(statementPrefix, "=")
}

func isStateAliasAssignmentPrefix(statementPrefix, aliasKind string) bool {
	return strings.HasPrefix(statementPrefix, aliasKind+" ") && strings.Contains(statementPrefix, "=")
}

func hasPropertyPipelineTransformPrefix(statementPrefix string) bool {
	return strings.HasPrefix(statementPrefix, "prop ") && strings.Contains(statementPrefix, "|")
}

func scenarioIDCompletions(lines []string) []completionCandidate {
	candidates := make([]completionCandidate, 0)
	for _, id := range collectIDs(lines, blockKindScenario) {
		candidates = append(candidates, completionCandidate{Label: id, Kind: 6, Detail: "scenario id"})
	}

	return candidates
}

func actIDCompletions(lines []string) []completionCandidate {
	candidates := make([]completionCandidate, 0)
	for _, id := range collectIDs(lines, blockKindAct) {
		candidates = append(candidates, completionCandidate{Label: id, Kind: 6, Detail: "act id"})
	}

	return candidates
}

func collectStateBackendIDs(lines []string) []string {
	ids := make([]string, 0)
	seen := make(map[string]struct{})

	for i := range lines {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "backend ") {
			continue
		}

		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "backend "))
		if rest == "" {
			continue
		}
		end := strings.IndexAny(rest, " =(")
		if end >= 0 {
			rest = rest[:end]
		}
		if rest == "" {
			continue
		}
		if _, ok := seen[rest]; ok {
			continue
		}

		seen[rest] = struct{}{}
		ids = append(ids, rest)
	}

	slices.Sort(ids)
	return ids
}

func collectIDs(lines []string, keyword string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	prefix := keyword + " "

	for i := range lines {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}

		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if rest == "" {
			continue
		}

		end := strings.IndexAny(rest, " (")
		if end >= 0 {
			rest = rest[:end]
		}
		if rest == "" {
			continue
		}
		if _, ok := seen[rest]; ok {
			continue
		}

		seen[rest] = struct{}{}
		result = append(result, rest)
	}

	slices.Sort(result)
	return result
}

func completionBlockKind(lines []string, lineIndex int) string {
	indent := currentIndent(lines, lineIndex)
	for i := lineIndex - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if currentIndent(lines, i) >= indent {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "act "):
			return blockKindAct
		case strings.HasPrefix(trimmed, "scenario "):
			return blockKindScenario
		case strings.HasPrefix(trimmed, "call "):
			return blockKindCall
		case trimmed == blockKindHTTP:
			return blockKindHTTP
		case trimmed == blockKindState:
			return blockKindState
		}
	}

	return blockKindTop
}

func blockCallRefBeforeLine(lines []string, lineIndex int) (string, bool) {
	if lineIndex <= 0 || lineIndex >= len(lines) {
		return "", false
	}

	indent := currentIndent(lines, lineIndex)
	if indent == 0 {
		return "", false
	}

	for i := lineIndex - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if currentIndent(lines, i) >= indent {
			continue
		}

		return trailingReference(trimmed)
	}

	return "", false
}

func currentIndent(lines []string, index int) int {
	if index < 0 || index >= len(lines) {
		return 0
	}

	count := 0
	for count < len(lines[index]) && lines[index][count] == ' ' {
		count++
	}
	return count
}

func isArgumentValuePrefix(prefix string) bool {
	fragment := currentFragment(prefix)
	beforeFragment := strings.TrimSpace(strings.TrimSuffix(prefix, fragment))
	return strings.HasSuffix(beforeFragment, ":")
}

func isBackendArgumentValuePrefix(prefix string) bool {
	fragment := currentFragment(prefix)
	beforeFragment := strings.TrimSpace(strings.TrimSuffix(prefix, fragment))
	return strings.HasSuffix(beforeFragment, "backend:")
}

func trailingReference(line string) (string, bool) {
	end := len(line)
	for end > 0 && !isReferenceChar(line[end-1]) {
		end--
	}
	start := end
	for start > 0 && isReferenceChar(line[start-1]) {
		start--
	}
	if start == end {
		return "", false
	}

	ref := line[start:end]
	if !strings.Contains(ref, ".") {
		return "", false
	}
	return ref, true
}
