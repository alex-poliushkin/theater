package thtr

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alex-poliushkin/theater"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
)

const (
	migrateHTTPAuthCall         = "http.auth"
	migrateHTTPIdentityCall     = "http.identity"
	migrateHTTPSessionCall      = "http.session.browser"
	migrateStateReadCall        = "state.read"
	migrateStateUpdateCall      = "state.update"
	migrateStateClaimCall       = "state.claim"
	migrateStateRecordCall      = "state.record"
	migrateStatePoolCall        = "state.pool"
	migrateStateReadAction      = "action.state.read"
	migrateStateUpdateAction    = "action.state.update"
	migrateStateClaimAction     = "action.state.claim"
	migrateStateRecordInventory = "inventory.state.record"
	migrateStatePoolInventory   = "inventory.state.pool"
	migrateCaptureSourceForm    = "form_field"
	migrateCaptureSourceHeader  = "response_header"
	migrateCaptureSourceCookie  = "response_cookie"
	migrateCaptureSourceJSON    = "json_pointer"
	migrateGeneratorPrefix      = "generate."
	migrateStringCall           = "string"
	migrateSelectorField        = "field"
	migrateSelectorDecode       = "decode"
	migrateSelectorPath         = "path"
	migrateSelectorPick         = "pick"
	migrateSelectorRegexp       = "regexp"
	migrateStateMinGuaranteeArg = "min_guarantee"
)

const (
	migrateStateRecordArg  = sectionStateRecord
	migrateStatePoolArg    = sectionStatePool
	migrateStateBackendArg = sectionStateBackend
)

// MarshalStage renders one canonical theater.StageSpec into formatter-clean
// `.thtr` source for the encodable subset of StageSpec. It returns an error
// when the input relies on names or structures that `.thtr` cannot represent
// without changing semantics.
func MarshalStage(spec theater.StageSpec) ([]byte, error) {
	document, err := buildSyntaxDocument(spec)
	if err != nil {
		return nil, err
	}

	formatter := thtrFormatter{}
	formatter.writeDocument(document)
	return []byte(formatter.builder.String()), nil
}

func buildSyntaxDocument(spec theater.StageSpec) (*syntaxDocument, error) {
	stageID, err := requireLocalIdentifier(spec.ID, "stage id")
	if err != nil {
		return nil, err
	}

	document := &syntaxDocument{
		Stage: stageSyntax{
			ID: stageID,
		},
		Scenarios: make([]scenarioSyntax, 0, len(spec.Scenarios)),
		Calls:     make([]scenarioCallSyntax, 0, len(spec.ScenarioCalls)),
	}
	if spec.Name != "" {
		document.Stage.Name = buildOptionalName(spec.Name)
	}

	if spec.HTTP != nil {
		httpSection, ok, err := buildHTTPSection(*spec.HTTP)
		if err != nil {
			return nil, err
		}
		if ok {
			document.HTTP = &httpSection
		}
	}

	if spec.State != nil {
		stateSection, ok, err := buildStateSection(*spec.State)
		if err != nil {
			return nil, err
		}
		if ok {
			document.State = &stateSection
		}
	}

	for i := range spec.Scenarios {
		scenario, err := buildScenario(spec.Scenarios[i])
		if err != nil {
			return nil, err
		}
		document.Scenarios = append(document.Scenarios, scenario)
	}

	for i := range spec.ScenarioCalls {
		call, err := buildScenarioCall(spec.ScenarioCalls[i])
		if err != nil {
			return nil, err
		}
		document.Calls = append(document.Calls, call)
	}

	rewriteRepeatedStateHandleAliases(document)

	return document, nil
}

type stateHandleRewriteCandidate struct {
	kind          string
	scenarioIndex int
	actIndex      int
	propertyIndex int
	propertyName  string
	inventoryCall callExpressionSyntax
}

type stateHandleRewriteGroup struct {
	kind       string
	aliasID    string
	candidates []stateHandleRewriteCandidate
}

func rewriteRepeatedStateHandleAliases(document *syntaxDocument) {
	if document == nil || document.State == nil {
		return
	}

	groups := collectStateHandleRewriteGroups(document)
	if len(groups) == 0 {
		return
	}

	usedIDs := make(map[string]struct{}, len(document.State.Entries))
	backendIDs := make(map[string]struct{}, len(document.State.Entries))
	for i := range document.State.Entries {
		usedIDs[document.State.Entries[i].ID] = struct{}{}
		if document.State.Entries[i].Kind == sectionStateBackend {
			backendIDs[document.State.Entries[i].ID] = struct{}{}
		}
	}

	for i := range groups {
		groups[i].aliasID = allocateGeneratedStateAliasID(groups[i].candidates[0].propertyName, usedIDs)
		entry := buildStateAliasRewriteEntry(groups[i], backendIDs)
		document.State.Entries = append(document.State.Entries, entry)
		rewriteStateHandleGroup(document, groups[i])
	}
}

func collectStateHandleRewriteGroups(document *syntaxDocument) []stateHandleRewriteGroup {
	var groups []stateHandleRewriteGroup
	for scenarioIndex := range document.Scenarios {
		for actIndex := range document.Scenarios[scenarioIndex].Acts {
			candidate, ok := stateHandleRewriteCandidateForAct(document.Scenarios[scenarioIndex].Acts[actIndex])
			if !ok {
				continue
			}
			candidate.scenarioIndex = scenarioIndex
			candidate.actIndex = actIndex

			matched := false
			for groupIndex := range groups {
				if groups[groupIndex].kind != candidate.kind {
					continue
				}
				if !inventoryCallsEquivalent(groups[groupIndex].candidates[0].inventoryCall, candidate.inventoryCall) {
					continue
				}
				groups[groupIndex].candidates = append(groups[groupIndex].candidates, candidate)
				matched = true
				break
			}
			if matched {
				continue
			}
			groups = append(groups, stateHandleRewriteGroup{
				kind:       candidate.kind,
				candidates: []stateHandleRewriteCandidate{candidate},
			})
		}
	}

	filtered := groups[:0]
	for i := range groups {
		if len(groups[i].candidates) < 2 {
			continue
		}
		filtered = append(filtered, groups[i])
	}
	return filtered
}

func stateHandleRewriteCandidateForAct(act actSyntax) (stateHandleRewriteCandidate, bool) {
	if act.Action == nil {
		return stateHandleRewriteCandidate{}, false
	}

	for propertyIndex := range act.Properties {
		call, ok := directStateHandleInventoryCall(act.Properties[propertyIndex])
		if !ok {
			continue
		}

		kind, targetArg, actionKind, allowedActionArgs, ok := stateHandleRewritePattern(call.Name, act.Action.Call.Name)
		if !ok {
			continue
		}
		if !callUsesOnlyNamedArgs(call, stateHandleInventoryAllowedArgs(kind)) {
			continue
		}
		if !actionMatchesStateHandleRewrite(act.Action.Call, targetArg, act.Properties[propertyIndex].Name, allowedActionArgs) {
			continue
		}
		if actReferencesRefOutsideAction(act, propertyIndex, act.Properties[propertyIndex].Name) {
			continue
		}
		if !stateHandleRewriteActionAllowed(actionKind, act.Action.Call.Args) {
			continue
		}

		return stateHandleRewriteCandidate{
			kind:          kind,
			propertyIndex: propertyIndex,
			propertyName:  act.Properties[propertyIndex].Name,
			inventoryCall: call,
		}, true
	}

	return stateHandleRewriteCandidate{}, false
}

func directStateHandleInventoryCall(property propertySyntax) (callExpressionSyntax, bool) {
	call, ok := ungroupExpression(property.Value).(callExpressionSyntax)
	if !ok {
		return callExpressionSyntax{}, false
	}
	switch call.Name {
	case migrateStateRecordInventory, migrateStatePoolInventory:
		return call, true
	default:
		return callExpressionSyntax{}, false
	}
}

func stateHandleRewritePattern(inventoryCall, actionCall string) (kind, targetArg, actionKind string, allowedActionArgs []string, ok bool) {
	switch {
	case inventoryCall == migrateStateRecordInventory && actionCall == migrateStateReadAction:
		return sectionStateRecord, migrateStateRecordArg, migrateStateReadAction, []string{migrateStateRecordArg}, true
	case inventoryCall == migrateStateRecordInventory && actionCall == migrateStateUpdateAction:
		return sectionStateRecord, migrateStateRecordArg, migrateStateUpdateAction, []string{
			migrateStateRecordArg, "expected_version", "value",
		}, true
	case inventoryCall == migrateStatePoolInventory && actionCall == migrateStateClaimAction:
		return sectionStatePool, migrateStatePoolArg, migrateStateClaimAction, []string{
			migrateStatePoolArg, "selector", "lease",
		}, true
	default:
		return "", "", "", nil, false
	}
}

func stateHandleInventoryAllowedArgs(kind string) []string {
	switch kind {
	case sectionStateRecord:
		return []string{migrateStateBackendArg, migrateStateRecordArg, migrateStateMinGuaranteeArg}
	case sectionStatePool:
		return []string{migrateStateBackendArg, migrateStatePoolArg, migrateStateMinGuaranteeArg}
	default:
		return nil
	}
}

func callUsesOnlyNamedArgs(call callExpressionSyntax, allowed []string) bool {
	if len(call.Args) == 0 {
		return false
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}

	for i := range call.Args {
		if call.Args[i].Name == "" || len(call.Args[i].Mapping) != 0 {
			return false
		}
		if _, ok := allowedSet[call.Args[i].Name]; !ok {
			return false
		}
	}

	return true
}

func actionMatchesStateHandleRewrite(
	call callExpressionSyntax,
	targetArg string,
	propertyName string,
	allowed []string,
) bool {
	if !callUsesOnlyNamedArgs(call, allowed) {
		return false
	}

	targetSeen := false
	for i := range call.Args {
		if call.Args[i].Name != targetArg {
			if expressionUsesRef(call.Args[i].Value, propertyName) {
				return false
			}
			continue
		}

		ref, ok := ungroupExpression(call.Args[i].Value).(refExpressionSyntax)
		if !ok || ref.Name != propertyName {
			return false
		}
		targetSeen = true
	}

	return targetSeen
}

func stateHandleRewriteActionAllowed(actionKind string, args []callArgumentSyntax) bool {
	switch actionKind {
	case migrateStateReadAction:
		return lookupCallArg(args, "record") != nil
	case migrateStateUpdateAction:
		return lookupCallArg(args, "record") != nil &&
			lookupCallArg(args, "expected_version") != nil &&
			lookupCallArg(args, "value") != nil
	case migrateStateClaimAction:
		return lookupCallArg(args, "pool") != nil && lookupCallArg(args, "lease") != nil
	default:
		return false
	}
}

func actReferencesRefOutsideAction(act actSyntax, propertyIndex int, name string) bool {
	for i := range act.Properties {
		if i == propertyIndex {
			continue
		}
		if expressionUsesRef(act.Properties[i].Value, name) {
			return true
		}
	}
	if act.CaptureAuth != nil {
		for i := range act.CaptureAuth.Slots {
			if expressionUsesRef(act.CaptureAuth.Slots[i].Value, name) {
				return true
			}
		}
	}
	for i := range act.Expectations {
		if expressionUsesRef(act.Expectations[i].Subject, name) || assertionUsesRef(act.Expectations[i].Assert, name) {
			return true
		}
	}
	for i := range act.Exports {
		if expressionUsesRef(act.Exports[i].Value, name) {
			return true
		}
	}
	return false
}

func expressionUsesRef(expr expressionSyntax, name string) bool {
	switch value := ungroupExpression(expr).(type) {
	case refExpressionSyntax:
		return value.Name == name
	case callExpressionSyntax:
		for i := range value.Args {
			if expressionUsesRef(value.Args[i].Value, name) || mappingUsesRef(value.Args[i].Mapping, name) {
				return true
			}
		}
		return false
	case pipelineExpressionSyntax:
		if expressionUsesRef(value.Base, name) {
			return true
		}
		for i := range value.Steps {
			if expressionUsesRef(value.Steps[i], name) {
				return true
			}
		}
		return false
	case objectExpressionSyntax:
		return mappingUsesRef(value.Fields, name)
	case listExpressionSyntax:
		for i := range value.Items {
			if expressionUsesRef(value.Items[i], name) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func mappingUsesRef(entries []mappingEntrySyntax, name string) bool {
	for i := range entries {
		if expressionUsesRef(entries[i].Value, name) || mappingUsesRef(entries[i].Mapping, name) {
			return true
		}
	}
	return false
}

func assertionUsesRef(assertion assertionSyntax, name string) bool {
	if expressionUsesRef(assertion.Value, name) || expressionUsesRef(assertion.SecondValue, name) {
		return true
	}
	if assertion.Nested != nil && assertionUsesRef(*assertion.Nested, name) {
		return true
	}
	for i := range assertion.Clauses {
		if expressionUsesRef(assertion.Clauses[i].Subject, name) || assertionUsesRef(assertion.Clauses[i].Assert, name) {
			return true
		}
	}
	return false
}

func allocateGeneratedStateAliasID(base string, used map[string]struct{}) string {
	candidate := base
	if candidate == "" {
		candidate = "state_alias"
	}
	if _, exists := used[candidate]; !exists {
		used[candidate] = struct{}{}
		return candidate
	}

	for suffix := 2; ; suffix++ {
		candidate = fmt.Sprintf("%s_%d", base, suffix)
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func buildStateAliasRewriteEntry(group stateHandleRewriteGroup, backendIDs map[string]struct{}) stageSectionEntrySyntax {
	callName := migrateStateRecordCall
	resourceArgName := migrateStateRecordArg
	if group.kind == sectionStatePool {
		callName = migrateStatePoolCall
		resourceArgName = migrateStatePoolArg
	}

	args := make([]callArgumentSyntax, 0, 3)
	backend := rewriteStateAliasStaticArg(
		lookupCallArg(group.candidates[0].inventoryCall.Args, migrateStateBackendArg),
		migrateStateBackendArg,
		backendIDs,
	)
	if backend != nil {
		args = append(args, callArgumentSyntax{Name: migrateStateBackendArg, Value: backend})
	}
	if resource := lookupCallArg(group.candidates[0].inventoryCall.Args, resourceArgName); resource != nil {
		args = append(args, callArgumentSyntax{Name: resourceArgName, Value: resource.Value})
	}
	guarantee := rewriteStateAliasStaticArg(
		lookupCallArg(group.candidates[0].inventoryCall.Args, migrateStateMinGuaranteeArg),
		migrateStateMinGuaranteeArg,
		backendIDs,
	)
	if guarantee != nil {
		args = append(args, callArgumentSyntax{Name: migrateStateMinGuaranteeArg, Value: guarantee})
	}

	return stageSectionEntrySyntax{
		Kind: group.kind,
		ID:   group.aliasID,
		Call: callExpressionSyntax{
			Name: callName,
			Args: args,
		},
	}
}

func rewriteStateAliasStaticArg(arg *callArgumentSyntax, name string, backendIDs map[string]struct{}) expressionSyntax {
	if arg == nil {
		return nil
	}

	switch name {
	case migrateStateBackendArg:
		if value, ok := stringLiteralValue(arg.Value); ok {
			if _, exists := backendIDs[value]; exists && validLocalIdentifier(value) {
				return symbolExpressionSyntax{Name: value}
			}
		}
	case migrateStateMinGuaranteeArg:
		if value, ok := stringLiteralValue(arg.Value); ok && validLocalIdentifier(value) {
			return symbolExpressionSyntax{Name: value}
		}
	}

	return arg.Value
}

func rewriteStateHandleGroup(document *syntaxDocument, group stateHandleRewriteGroup) {
	for i := range group.candidates {
		candidate := group.candidates[i]
		act := &document.Scenarios[candidate.scenarioIndex].Acts[candidate.actIndex]
		rewriteStateHandleAction(&act.Action.Call, group.kind, group.aliasID)
		act.Properties = append(act.Properties[:candidate.propertyIndex], act.Properties[candidate.propertyIndex+1:]...)
	}
}

func rewriteStateHandleAction(call *callExpressionSyntax, kind, aliasID string) {
	switch {
	case kind == sectionStateRecord && call.Name == migrateStateReadAction:
		call.Name = migrateStateReadCall
		call.Args = []callArgumentSyntax{{
			Name:  migrateStateRecordArg,
			Value: symbolExpressionSyntax{Name: aliasID},
		}}
	case kind == sectionStateRecord && call.Name == migrateStateUpdateAction:
		call.Name = migrateStateUpdateCall
		call.Args = []callArgumentSyntax{
			{Name: migrateStateRecordArg, Value: symbolExpressionSyntax{Name: aliasID}},
			{Name: "if_version", Value: lookupCallArg(call.Args, "expected_version").Value},
			{Name: "value", Value: lookupCallArg(call.Args, "value").Value},
		}
	case kind == sectionStatePool && call.Name == migrateStateClaimAction:
		args := []callArgumentSyntax{{
			Name:  migrateStatePoolArg,
			Value: symbolExpressionSyntax{Name: aliasID},
		}}
		if selector := lookupCallArg(call.Args, "selector"); selector != nil {
			if selectorArgs := rewriteStateClaimSelectorArgs(selector); len(selectorArgs) != 0 {
				args = append(args, selectorArgs...)
			} else {
				args = append(args, callArgumentSyntax{Name: "selector", Value: selector.Value})
			}
		}
		args = append(args, callArgumentSyntax{Name: "lease", Value: lookupCallArg(call.Args, "lease").Value})
		call.Name = migrateStateClaimCall
		call.Args = args
	}
}

func rewriteStateClaimSelectorArgs(selector *callArgumentSyntax) []callArgumentSyntax {
	object, ok := ungroupExpression(selector.Value).(objectExpressionSyntax)
	if !ok || len(object.Fields) == 0 {
		return nil
	}

	var idArg *callArgumentSyntax
	var fieldsArg *callArgumentSyntax
	for i := range object.Fields {
		entry := object.Fields[i]
		switch entry.Name {
		case "id":
			if len(entry.Mapping) != 0 {
				return nil
			}
			arg := callArgumentSyntax{Name: "id", Value: entry.Value, Span: entry.Span}
			idArg = &arg
		case "fields":
			if len(entry.Mapping) != 0 {
				return nil
			}
			fieldsObject, ok := ungroupExpression(entry.Value).(objectExpressionSyntax)
			if !ok {
				return nil
			}
			arg := callArgumentSyntax{Name: "fields", Value: fieldsObject, Span: entry.Span}
			fieldsArg = &arg
		default:
			return nil
		}
	}

	args := make([]callArgumentSyntax, 0, 2)
	if idArg != nil {
		args = append(args, *idArg)
	}
	if fieldsArg != nil {
		args = append(args, *fieldsArg)
	}
	return args
}

func lookupCallArg(args []callArgumentSyntax, name string) *callArgumentSyntax {
	for i := range args {
		if args[i].Name == name {
			return &args[i]
		}
	}
	return nil
}

func stringLiteralValue(expr expressionSyntax) (string, bool) {
	literal, ok := ungroupExpression(expr).(literalExpressionSyntax)
	if !ok || literal.Kind != literalKindString {
		return "", false
	}
	value, err := strconv.Unquote(literal.Text)
	if err != nil {
		return "", false
	}
	return value, true
}

func inventoryCallsEquivalent(left, right callExpressionSyntax) bool {
	if left.Name != right.Name || left.BlockForm != right.BlockForm || len(left.Args) != len(right.Args) {
		return false
	}
	for i := range left.Args {
		if !callArgumentsEquivalent(left.Args[i], right.Args[i]) {
			return false
		}
	}
	return true
}

func callArgumentsEquivalent(left, right callArgumentSyntax) bool {
	if left.Name != right.Name || len(left.Mapping) != len(right.Mapping) {
		return false
	}
	if len(left.Mapping) != 0 {
		return mappingsEquivalent(left.Mapping, right.Mapping)
	}
	return expressionsEquivalent(left.Value, right.Value)
}

func mappingsEquivalent(left, right []mappingEntrySyntax) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Name != right[i].Name || len(left[i].Mapping) != len(right[i].Mapping) {
			return false
		}
		if len(left[i].Mapping) != 0 {
			if !mappingsEquivalent(left[i].Mapping, right[i].Mapping) {
				return false
			}
			continue
		}
		if !expressionsEquivalent(left[i].Value, right[i].Value) {
			return false
		}
	}
	return true
}

func expressionsEquivalent(left, right expressionSyntax) bool {
	left = ungroupExpression(left)
	right = ungroupExpression(right)

	switch leftValue := left.(type) {
	case literalExpressionSyntax:
		return literalExpressionsEquivalent(leftValue, right)
	case symbolExpressionSyntax:
		return symbolExpressionsEquivalent(leftValue, right)
	case refExpressionSyntax:
		return refExpressionsEquivalent(leftValue, right)
	case callExpressionSyntax:
		return callExpressionsEquivalent(leftValue, right)
	case pipelineExpressionSyntax:
		return pipelineExpressionsEquivalent(leftValue, right)
	case objectExpressionSyntax:
		return objectExpressionsEquivalent(leftValue, right)
	case listExpressionSyntax:
		return listExpressionsEquivalent(leftValue, right)
	default:
		return false
	}
}

func literalExpressionsEquivalent(left literalExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(literalExpressionSyntax)
	return ok && left.Kind == rightValue.Kind && left.Text == rightValue.Text
}

func symbolExpressionsEquivalent(left symbolExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(symbolExpressionSyntax)
	return ok && left.Name == rightValue.Name
}

func refExpressionsEquivalent(left refExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(refExpressionSyntax)
	return ok && left.Name == rightValue.Name
}

func callExpressionsEquivalent(left callExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(callExpressionSyntax)
	return ok && inventoryCallsEquivalent(left, rightValue)
}

func pipelineExpressionsEquivalent(left pipelineExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(pipelineExpressionSyntax)
	if !ok || len(left.Steps) != len(rightValue.Steps) || !expressionsEquivalent(left.Base, rightValue.Base) {
		return false
	}
	for i := range left.Steps {
		if !inventoryCallsEquivalent(left.Steps[i], rightValue.Steps[i]) {
			return false
		}
	}
	return true
}

func objectExpressionsEquivalent(left objectExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(objectExpressionSyntax)
	return ok && left.Dynamic == rightValue.Dynamic && mappingsEquivalent(left.Fields, rightValue.Fields)
}

func listExpressionsEquivalent(left listExpressionSyntax, right expressionSyntax) bool {
	rightValue, ok := right.(listExpressionSyntax)
	if !ok || left.Dynamic != rightValue.Dynamic || len(left.Items) != len(rightValue.Items) {
		return false
	}
	for i := range left.Items {
		if !expressionsEquivalent(left.Items[i], rightValue.Items[i]) {
			return false
		}
	}
	return true
}

func buildOptionalName(name string) *nameSyntax {
	if name == "" {
		return nil
	}

	return &nameSyntax{
		Value: stringLiteralExpression(name),
	}
}

func buildHTTPSection(spec theater.HTTPSpec) (stageSectionSyntax, bool, error) {
	entries := make([]stageSectionEntrySyntax, 0, len(spec.Sessions)+len(spec.Auth)+len(spec.Identities))

	for _, name := range sortedStringKeys(spec.Sessions) {
		sessionID, err := requireLocalIdentifier(name, "http session id")
		if err != nil {
			return stageSectionSyntax{}, false, err
		}
		entries = append(entries, stageSectionEntrySyntax{
			Kind: sectionHTTPSession,
			ID:   sessionID,
			Call: callExpressionSyntax{Name: migrateHTTPSessionCall},
		})
	}

	for _, name := range sortedStringKeys(spec.Auth) {
		authID, err := requireLocalIdentifier(name, "http auth id")
		if err != nil {
			return stageSectionSyntax{}, false, err
		}
		call, err := buildHTTPAuthCall(spec.Auth[name])
		if err != nil {
			return stageSectionSyntax{}, false, err
		}
		entries = append(entries, stageSectionEntrySyntax{
			Kind: sectionHTTPAuth,
			ID:   authID,
			Call: call,
		})
	}

	for _, name := range sortedStringKeys(spec.Identities) {
		identityID, err := requireLocalIdentifier(name, "http identity id")
		if err != nil {
			return stageSectionSyntax{}, false, err
		}
		call := buildHTTPIdentityCall(spec.Identities[name])
		entries = append(entries, stageSectionEntrySyntax{
			Kind: sectionHTTPIdentity,
			ID:   identityID,
			Call: call,
		})
	}

	if len(entries) == 0 {
		return stageSectionSyntax{}, false, nil
	}

	return stageSectionSyntax{
		Name:    "http",
		Entries: entries,
	}, true, nil
}

func buildHTTPAuthCall(spec theater.HTTPAuthSpec) (callExpressionSyntax, error) {
	args := make([]callArgumentSyntax, 0, 1)
	if len(spec.Attach) != 0 {
		items := make([]expressionSyntax, 0, len(spec.Attach))
		for i := range spec.Attach {
			item, err := buildHTTPAttachmentExpression(spec.Attach[i])
			if err != nil {
				return callExpressionSyntax{}, err
			}
			items = append(items, item)
		}
		args = append(args, callArgumentSyntax{
			Name: "attach",
			Value: listExpressionSyntax{
				Dynamic: true,
				Items:   items,
			},
		})
	}

	return callExpressionSyntax{
		Name: migrateHTTPAuthCall,
		Args: args,
	}, nil
}

func buildHTTPAttachmentExpression(spec theater.HTTPAuthAttachmentSpec) (expressionSyntax, error) {
	switch {
	case spec.Bearer != nil:
		return objectExpressionSyntax{
			Dynamic: true,
			Fields: []mappingEntrySyntax{{
				Name: "bearer",
				Value: objectExpressionSyntax{
					Dynamic: true,
					Fields: []mappingEntrySyntax{{
						Name:  "token",
						Value: stringLiteralExpression(spec.Bearer.Token),
					}},
				},
			}},
		}, nil
	case spec.Basic != nil:
		return objectExpressionSyntax{
			Dynamic: true,
			Fields: []mappingEntrySyntax{{
				Name: "basic",
				Value: objectExpressionSyntax{
					Dynamic: true,
					Fields: []mappingEntrySyntax{
						{Name: "username", Value: stringLiteralExpression(spec.Basic.Username)},
						{Name: "password", Value: stringLiteralExpression(spec.Basic.Password)},
					},
				},
			}},
		}, nil
	case spec.APIKey != nil:
		return objectExpressionSyntax{
			Dynamic: true,
			Fields: []mappingEntrySyntax{{
				Name: "api_key",
				Value: objectExpressionSyntax{
					Dynamic: true,
					Fields: []mappingEntrySyntax{
						{Name: "in", Value: stringLiteralExpression(string(spec.APIKey.In))},
						{Name: "name", Value: stringLiteralExpression(spec.APIKey.Name)},
						{Name: "value", Value: stringLiteralExpression(spec.APIKey.Value)},
					},
				},
			}},
		}, nil
	case spec.HeaderSlot != nil:
		return buildHTTPNamedSlotAttachmentExpression("header_slot", spec.HeaderSlot.Name, spec.HeaderSlot.Slot), nil
	case spec.QuerySlot != nil:
		return buildHTTPNamedSlotAttachmentExpression("query_slot", spec.QuerySlot.Name, spec.QuerySlot.Slot), nil
	case spec.FormSlot != nil:
		return buildHTTPNamedSlotAttachmentExpression("form_slot", spec.FormSlot.Name, spec.FormSlot.Slot), nil
	default:
		return nil, errors.New("http auth attachment is empty")
	}
}

func buildHTTPNamedSlotAttachmentExpression(kind, name, slot string) expressionSyntax {
	return objectExpressionSyntax{
		Dynamic: true,
		Fields: []mappingEntrySyntax{{
			Name: kind,
			Value: objectExpressionSyntax{
				Dynamic: true,
				Fields: []mappingEntrySyntax{
					{Name: "name", Value: stringLiteralExpression(name)},
					{Name: "slot", Value: stringLiteralExpression(slot)},
				},
			},
		}},
	}
}

func buildHTTPIdentityCall(spec theater.HTTPIdentitySpec) callExpressionSyntax {
	args := make([]callArgumentSyntax, 0, 2)
	if spec.Session != "" {
		args = append(args, callArgumentSyntax{
			Name:  "session",
			Value: stringLiteralExpression(spec.Session),
		})
	}
	if spec.Auth != "" {
		args = append(args, callArgumentSyntax{
			Name:  "auth",
			Value: stringLiteralExpression(spec.Auth),
		})
	}

	return callExpressionSyntax{
		Name: migrateHTTPIdentityCall,
		Args: args,
	}
}

func buildStateSection(spec theater.StateSpec) (stageSectionSyntax, bool, error) {
	entries := make([]stageSectionEntrySyntax, 0, len(spec.Backends))
	for _, name := range sortedStringKeys(spec.Backends) {
		backendID, err := requireLocalIdentifier(name, "state backend id")
		if err != nil {
			return stageSectionSyntax{}, false, err
		}
		call, err := buildStaticCall(spec.Backends[name].Use, spec.Backends[name].With, nil)
		if err != nil {
			return stageSectionSyntax{}, false, err
		}
		entries = append(entries, stageSectionEntrySyntax{
			Kind: sectionStateBackend,
			ID:   backendID,
			Call: call,
		})
	}

	if len(entries) == 0 {
		return stageSectionSyntax{}, false, nil
	}

	return stageSectionSyntax{
		Name:    "state",
		Entries: entries,
	}, true, nil
}

func buildScenario(spec theater.ScenarioSpec) (scenarioSyntax, error) {
	scenarioID, err := requireSlashName(spec.ID, "scenario id")
	if err != nil {
		return scenarioSyntax{}, err
	}

	scenario := scenarioSyntax{
		ID:     scenarioID,
		Inputs: make([]inputSyntax, 0, len(spec.Inputs)),
		Acts:   make([]actSyntax, 0, len(spec.Acts)),
	}
	if spec.Name != "" {
		scenario.Name = buildOptionalName(spec.Name)
	}

	for _, name := range sortedValueContractKeys(spec.Inputs) {
		input, err := buildInputSyntax(name, spec.Inputs[name])
		if err != nil {
			return scenarioSyntax{}, err
		}
		scenario.Inputs = append(scenario.Inputs, input)
	}

	for i := range spec.Acts {
		act, err := buildAct(spec.Acts[i])
		if err != nil {
			return scenarioSyntax{}, err
		}
		scenario.Acts = append(scenario.Acts, act)
	}

	return scenario, nil
}

func buildInputSyntax(name string, contract theater.ValueContract) (inputSyntax, error) {
	inputName, err := requireLocalIdentifier(name, "scenario input")
	if err != nil {
		return inputSyntax{}, err
	}
	if contract.Kind == "" || !contract.Kind.Valid() {
		return inputSyntax{}, fmt.Errorf("input %q must declare one supported kind", name)
	}
	if len(contract.Kinds) != 0 || contract.Description != "" || contract.Sensitivity != "" ||
		contract.Capture != "" || len(contract.Fields) != 0 || contract.Elem != nil {
		return inputSyntax{}, fmt.Errorf("input %q uses contract fields that .thtr inputs do not encode", name)
	}

	typeName, err := requireLocalIdentifier(string(contract.Kind), "input kind")
	if err != nil {
		return inputSyntax{}, err
	}

	return inputSyntax{
		Name:     inputName,
		Type:     typeName,
		Required: contract.Required,
	}, nil
}

func buildAct(spec theater.ActSpec) (actSyntax, error) {
	action, err := buildAction(spec.Action)
	if err != nil {
		return actSyntax{}, err
	}

	actID, err := requireLocalIdentifier(spec.ID, "act id")
	if err != nil {
		return actSyntax{}, err
	}

	act := actSyntax{
		ID:         actID,
		Action:     &action,
		Properties: make([]propertySyntax, 0, len(spec.Properties)),
	}
	if spec.Name != "" {
		act.Name = buildOptionalName(spec.Name)
	}
	if spec.Eventually != nil {
		act.Eventually = &eventuallySyntax{
			Timeout:  spec.Eventually.Timeout,
			Interval: spec.Eventually.Interval,
		}
	}

	for _, name := range sortedPropertyKeys(spec.Properties) {
		property, err := buildProperty(name, spec.Properties[name])
		if err != nil {
			return actSyntax{}, err
		}
		act.Properties = append(act.Properties, property)
	}

	if spec.CaptureAuth != nil {
		capture, err := buildCaptureAuth(*spec.CaptureAuth)
		if err != nil {
			return actSyntax{}, err
		}
		act.CaptureAuth = &capture
	}

	for i := range spec.Expectations {
		expectation, err := buildExpectation(spec.Expectations[i])
		if err != nil {
			return actSyntax{}, err
		}
		act.Expectations = append(act.Expectations, expectation)
	}

	for i := range spec.Exports {
		export, err := buildActExport(spec.Exports[i])
		if err != nil {
			return actSyntax{}, err
		}
		act.Exports = append(act.Exports, export)
	}

	for i := range spec.Transitions {
		event, err := requireLocalIdentifier(transitionEventLabel(spec.Transitions[i].On), "transition event")
		if err != nil {
			return actSyntax{}, err
		}
		target, err := requireLocalIdentifier(spec.Transitions[i].To, "transition target")
		if err != nil {
			return actSyntax{}, err
		}
		act.Transitions = append(act.Transitions, transitionSyntax{
			Event: event,
			To:    target,
		})
	}

	return act, nil
}

func buildAction(spec theater.ActionSpec) (actionSyntax, error) {
	call, err := buildBindingCall(spec.Use, spec.With, nil)
	if err != nil {
		return actionSyntax{}, err
	}
	normalizeStateActionLiterals(&call)

	return actionSyntax{
		Repeatable: spec.Repeatable,
		Call:       call,
	}, nil
}

func normalizeStateActionLiterals(call *callExpressionSyntax) {
	switch call.Name {
	case migrateStateClaimAction, migrateStateClaimCall:
		normalizeStateClaimLeaseArg(call.Args)
	case stateRenewActionCall, stateRenewSugarCall:
		normalizeDurationArg(call.Args, "ttl")
	}
}

func normalizeStateClaimLeaseArg(args []callArgumentSyntax) {
	arg := lookupCallArg(args, "lease")
	if arg == nil {
		return
	}

	object, ok := ungroupExpression(arg.Value).(objectExpressionSyntax)
	if !ok {
		return
	}
	for i := range object.Fields {
		switch object.Fields[i].Name {
		case "ttl":
			object.Fields[i].Value = normalizeDurationExpression(object.Fields[i].Value)
		case "on_expiry":
			object.Fields[i].Value = normalizeSymbolExpression(object.Fields[i].Value)
		}
	}
	arg.Value = object
}

func normalizeDurationArg(args []callArgumentSyntax, name string) {
	arg := lookupCallArg(args, name)
	if arg == nil {
		return
	}
	arg.Value = normalizeDurationExpression(arg.Value)
}

func normalizeDurationExpression(expr expressionSyntax) expressionSyntax {
	value, ok := stringLiteralValue(expr)
	if !ok {
		return expr
	}
	if _, err := time.ParseDuration(value); err != nil {
		return expr
	}
	return literalExpressionSyntax{
		Kind: literalKindDuration,
		Text: value,
	}
}

func normalizeSymbolExpression(expr expressionSyntax) expressionSyntax {
	value, ok := stringLiteralValue(expr)
	if !ok || !validLocalIdentifier(value) {
		return expr
	}
	return symbolExpressionSyntax{Name: value}
}

func buildProperty(name string, spec theater.PropertySpec) (propertySyntax, error) {
	propertyName, err := requireLocalIdentifier(name, "property")
	if err != nil {
		return propertySyntax{}, err
	}
	if spec.Inventory == nil {
		return propertySyntax{}, fmt.Errorf("property %q must declare inventory", name)
	}

	base, err := buildBindingCall(spec.Inventory.Use, spec.Inventory.With, nil)
	if err != nil {
		return propertySyntax{}, err
	}

	value := expressionSyntax(base)
	if len(spec.Decorators) != 0 {
		steps := make([]callExpressionSyntax, 0, len(spec.Decorators))
		for i := range spec.Decorators {
			call, err := buildStaticCall(spec.Decorators[i].Use, spec.Decorators[i].With, nil)
			if err != nil {
				return propertySyntax{}, err
			}
			steps = append(steps, call)
		}
		value = pipelineExpressionSyntax{
			Base:  base,
			Steps: steps,
		}
	}

	return propertySyntax{
		Name:  propertyName,
		Value: value,
	}, nil
}

func buildCaptureAuth(spec theater.HTTPAuthCaptureSpec) (captureAuthSyntax, error) {
	authName, err := requireLocalIdentifier(spec.Auth, "capture_auth target")
	if err != nil {
		return captureAuthSyntax{}, err
	}

	capture := captureAuthSyntax{
		Auth:  authName,
		Slots: make([]mappingEntrySyntax, 0, len(spec.Slots)),
	}

	for _, name := range sortedCaptureSourceKeys(spec.Slots) {
		slotName, err := requireLocalIdentifier(name, "capture_auth slot")
		if err != nil {
			return captureAuthSyntax{}, err
		}
		value, err := buildCaptureSourceExpression(spec.Slots[name])
		if err != nil {
			return captureAuthSyntax{}, err
		}
		capture.Slots = append(capture.Slots, mappingEntrySyntax{
			Name:  slotName,
			Value: value,
		})
	}

	return capture, nil
}

func buildCaptureSourceExpression(spec theater.HTTPCaptureSourceSpec) (expressionSyntax, error) {
	switch {
	case spec.ResponseHeader != "":
		return positionalCall(migrateCaptureSourceHeader, stringLiteralExpression(spec.ResponseHeader)), nil
	case spec.ResponseCookie != "":
		return positionalCall(migrateCaptureSourceCookie, stringLiteralExpression(spec.ResponseCookie)), nil
	case spec.JSONPointer != "":
		return positionalCall(migrateCaptureSourceJSON, stringLiteralExpression(string(spec.JSONPointer))), nil
	case spec.FormField != "":
		return positionalCall(migrateCaptureSourceForm, stringLiteralExpression(spec.FormField)), nil
	default:
		return nil, errors.New("capture_auth source is empty")
	}
}

func buildExpectation(spec theater.ExpectationSpec) (expectationSyntax, error) {
	expectationID, err := requireLocalIdentifier(spec.ID, "expectation id")
	if err != nil {
		return expectationSyntax{}, err
	}

	subject, err := buildSubjectExpression(spec.Subject)
	if err != nil {
		return expectationSyntax{}, err
	}
	assertion, err := buildAssertion(spec.Assert)
	if err != nil {
		return expectationSyntax{}, err
	}

	return expectationSyntax{
		ID:      expectationID,
		Subject: subject,
		Assert:  assertion,
	}, nil
}

func buildSubjectExpression(spec theater.SubjectSpec) (expressionSyntax, error) {
	if spec.Field == "" {
		return nil, errors.New("expectation subject must declare field")
	}
	fieldName, err := requireLocalIdentifier(spec.Field, "subject field")
	if err != nil {
		return nil, err
	}

	steps, err := buildSelectorSteps(spec.Decode, spec.Path, spec.Through)
	if err != nil {
		return nil, err
	}

	base := positionalCall(migrateSelectorField, symbolExpressionSyntax{Name: fieldName})
	if len(steps) == 0 {
		return base, nil
	}

	return pipelineExpressionSyntax{
		Base:  base,
		Steps: steps,
	}, nil
}

func buildAssertion(spec theater.AssertSpec) (assertionSyntax, error) {
	switch spec.Ref {
	case builtinexpectation.EqualRef:
		return buildMaybeUnaryAssertion(assertionKindEqual, spec.Ref, "expected", spec.Args)
	case builtinexpectation.PresentRef:
		return buildNoArgAssertion(assertionKindPresent, spec.Ref, spec.Args)
	case builtinexpectation.NullRef:
		return buildNoArgAssertion(assertionKindNull, spec.Ref, spec.Args)
	case builtinexpectation.NotNullRef:
		return buildNoArgAssertion(assertionKindNotNull, spec.Ref, spec.Args)
	case builtinexpectation.ContainsRef:
		return buildMaybeUnaryAssertion(assertionKindContains, spec.Ref, "expected", spec.Args)
	case builtinexpectation.MatchesRef:
		return buildMaybeUnaryAssertion(assertionKindMatches, spec.Ref, "pattern", spec.Args)
	case builtinexpectation.GTRef:
		return buildMaybeUnaryAssertion(assertionKindGT, spec.Ref, "expected", spec.Args)
	case builtinexpectation.GTERef:
		return buildMaybeUnaryAssertion(assertionKindGTE, spec.Ref, "expected", spec.Args)
	case builtinexpectation.LTRef:
		return buildMaybeUnaryAssertion(assertionKindLT, spec.Ref, "expected", spec.Args)
	case builtinexpectation.LTERef:
		return buildMaybeUnaryAssertion(assertionKindLTE, spec.Ref, "expected", spec.Args)
	case builtinexpectation.BetweenRef:
		return buildMaybeBinaryAssertion(assertionKindBetween, spec.Ref, "min", "max", spec.Args)
	case builtinexpectation.HasKeyRef:
		return buildMaybeUnaryAssertion(assertionKindHasKey, spec.Ref, "key", spec.Args)
	case builtinexpectation.LacksKeyRef:
		return buildMaybeUnaryAssertion(assertionKindLacksKey, spec.Ref, "key", spec.Args)
	case builtinexpectation.HasEntryRef:
		return buildMaybeHasEntryAssertion(spec.Ref, spec.Args)
	default:
		return buildCanonicalAssertion(spec.Ref, spec.Args)
	}
}

func buildNoArgAssertion(kind assertionKind, ref string, args map[string]theater.BindingSpec) (assertionSyntax, error) {
	if len(args) != 0 {
		return buildCanonicalAssertion(ref, args)
	}

	return assertionSyntax{Kind: kind}, nil
}

func buildMaybeUnaryAssertion(
	kind assertionKind,
	ref string,
	arg string,
	args map[string]theater.BindingSpec,
) (assertionSyntax, error) {
	if len(args) != 1 {
		return buildCanonicalAssertion(ref, args)
	}

	value, ok := args[arg]
	if !ok {
		return buildCanonicalAssertion(ref, args)
	}

	expr, err := buildBindingExpression(value)
	if err != nil {
		return assertionSyntax{}, err
	}

	return assertionSyntax{
		Kind:  kind,
		Value: expr,
	}, nil
}

func buildMaybeBinaryAssertion(
	kind assertionKind,
	ref string,
	firstArg string,
	secondArg string,
	args map[string]theater.BindingSpec,
) (assertionSyntax, error) {
	if len(args) != 2 {
		return buildCanonicalAssertion(ref, args)
	}

	firstValue, ok := args[firstArg]
	if !ok {
		return buildCanonicalAssertion(ref, args)
	}
	secondValue, ok := args[secondArg]
	if !ok {
		return buildCanonicalAssertion(ref, args)
	}

	firstExpr, err := buildBindingExpression(firstValue)
	if err != nil {
		return assertionSyntax{}, err
	}
	secondExpr, err := buildBindingExpression(secondValue)
	if err != nil {
		return assertionSyntax{}, err
	}

	return assertionSyntax{
		Kind:        kind,
		Value:       firstExpr,
		SecondValue: secondExpr,
	}, nil
}

func buildMaybeHasEntryAssertion(ref string, args map[string]theater.BindingSpec) (assertionSyntax, error) {
	if len(args) != 2 {
		return buildCanonicalAssertion(ref, args)
	}

	key, ok := args["key"]
	if !ok {
		return buildCanonicalAssertion(ref, args)
	}
	assert, ok := args["assert"]
	if !ok {
		return buildCanonicalAssertion(ref, args)
	}

	keyExpr, err := buildBindingExpression(key)
	if err != nil {
		return assertionSyntax{}, err
	}
	nestedSpec, ok := bindingObjectAssertSpec(assert)
	if !ok {
		return buildCanonicalAssertion(ref, args)
	}
	nested, err := buildAssertion(nestedSpec)
	if err != nil {
		return assertionSyntax{}, err
	}

	return assertionSyntax{
		Kind:   assertionKindHasEntry,
		Value:  keyExpr,
		Nested: &nested,
	}, nil
}

func bindingObjectAssertSpec(binding theater.BindingSpec) (theater.AssertSpec, bool) {
	if binding.Kind != theater.BindingKindObject {
		return theater.AssertSpec{}, false
	}
	refBinding, ok := binding.Object["ref"]
	if !ok || refBinding.Kind != theater.BindingKindLiteral {
		return theater.AssertSpec{}, false
	}
	ref, ok := refBinding.Value.(string)
	if !ok || ref == "" {
		return theater.AssertSpec{}, false
	}
	argsBinding, ok := binding.Object["args"]
	if !ok {
		return theater.AssertSpec{Ref: ref}, true
	}
	if argsBinding.Kind != theater.BindingKindObject {
		return theater.AssertSpec{}, false
	}

	return theater.AssertSpec{
		Ref:  ref,
		Args: argsBinding.Object,
	}, true
}

func buildCanonicalAssertion(ref string, args map[string]theater.BindingSpec) (assertionSyntax, error) {
	callRef, err := requireDotName(ref, "matcher ref")
	if err != nil {
		return assertionSyntax{}, err
	}

	call, err := buildBindingCall(ref, args, matcherArgumentOrder(ref))
	if err != nil {
		return assertionSyntax{}, err
	}
	call.Name = callRef

	return assertionSyntax{
		Kind:  assertionKindCall,
		Value: call,
	}, nil
}

func buildActExport(spec theater.ExportSpec) (exportSyntax, error) {
	value, err := buildExportExpression(spec)
	if err != nil {
		return exportSyntax{}, err
	}
	exportName, err := requireLocalIdentifier(spec.As, "export")
	if err != nil {
		return exportSyntax{}, err
	}

	return exportSyntax{
		Name:  exportName,
		Value: value,
	}, nil
}

func buildExportExpression(spec theater.ExportSpec) (expressionSyntax, error) {
	if spec.Ref != nil {
		return buildExportRefExpression(spec)
	}
	if spec.Field == "" {
		return nil, fmt.Errorf("export %q must declare field or ref", spec.As)
	}

	return buildSubjectExpression(theater.SubjectSpec{
		Field:   spec.Field,
		Decode:  spec.Decode,
		Path:    spec.Path,
		Through: spec.Through,
	})
}

func buildExportRefExpression(spec theater.ExportSpec) (expressionSyntax, error) {
	if spec.Ref.Name == "" {
		return nil, errors.New("ref selection requires ref name")
	}
	if spec.Decode != "" && (spec.Ref.Decode != "" || !spec.Ref.Path.IsZero() || len(spec.Ref.Through) != 0) {
		return nil, fmt.Errorf("export %q cannot migrate split ref and export selectors with export-level decode", spec.As)
	}
	refName, err := requireLocalIdentifier(spec.Ref.Name, "ref name")
	if err != nil {
		return nil, err
	}

	refSteps, err := buildSelectorSteps(spec.Ref.Decode, spec.Ref.Path, spec.Ref.Through)
	if err != nil {
		return nil, err
	}
	exportSteps, err := buildSelectorSteps(spec.Decode, spec.Path, spec.Through)
	if err != nil {
		return nil, err
	}
	steps := refSteps
	steps = append(steps, exportSteps...)

	base := refExpressionSyntax{Name: refName}
	if len(steps) == 0 {
		return base, nil
	}

	return pipelineExpressionSyntax{
		Base:  base,
		Steps: steps,
	}, nil
}

func buildScenarioCall(spec theater.ScenarioCallSpec) (scenarioCallSyntax, error) {
	callID, err := requireLocalIdentifier(spec.ID, "scenario call id")
	if err != nil {
		return scenarioCallSyntax{}, err
	}
	scenarioID, err := requireSlashName(spec.ScenarioID, "scenario call target")
	if err != nil {
		return scenarioCallSyntax{}, err
	}

	bindings, err := buildNamedBindingArguments(spec.Bindings, nil)
	if err != nil {
		return scenarioCallSyntax{}, err
	}

	call := scenarioCallSyntax{
		ID:         callID,
		ScenarioID: scenarioID,
		Bindings:   bindings,
	}
	if spec.Name != "" {
		call.Name = buildOptionalName(spec.Name)
	}

	for i := range spec.Dependencies {
		dependencyID, err := requireLocalIdentifier(spec.Dependencies[i].CallID, "dependency call id")
		if err != nil {
			return scenarioCallSyntax{}, err
		}
		when := dependencyPredicateLabel(spec.Dependencies[i].When)
		if when != "" {
			when, err = requireLocalIdentifier(when, "dependency predicate")
			if err != nil {
				return scenarioCallSyntax{}, err
			}
		}
		call.Dependencies = append(call.Dependencies, dependencySyntax{
			CallID: dependencyID,
			When:   when,
		})
	}

	for i := range spec.Exports {
		export, err := buildScenarioCallExport(spec.Exports[i])
		if err != nil {
			return scenarioCallSyntax{}, err
		}
		call.Exports = append(call.Exports, export)
	}

	return call, nil
}

func buildScenarioCallExport(spec theater.ExportSpec) (exportSyntax, error) {
	if spec.Ref == nil || spec.Ref.Name == "" {
		return exportSyntax{}, fmt.Errorf("scenario call export %q must select direct ref", spec.As)
	}
	if spec.Ref.Decode != "" || spec.Ref.Path != "" || len(spec.Ref.Through) != 0 || spec.Field != "" ||
		spec.Decode != "" || spec.Path != "" || len(spec.Through) != 0 {
		return exportSyntax{}, fmt.Errorf("scenario call export %q must stay direct ref", spec.As)
	}
	exportName, err := requireLocalIdentifier(spec.As, "scenario call export")
	if err != nil {
		return exportSyntax{}, err
	}
	refName, err := requireLocalIdentifier(spec.Ref.Name, "scenario call export ref")
	if err != nil {
		return exportSyntax{}, err
	}

	return exportSyntax{
		Name: exportName,
		Value: refExpressionSyntax{
			Name: refName,
		},
	}, nil
}

func buildBindingCall(name string, args map[string]theater.BindingSpec, preferred []string) (callExpressionSyntax, error) {
	callName, err := requireDotName(name, "call name")
	if err != nil {
		return callExpressionSyntax{}, err
	}
	callArgs, err := buildNamedBindingArguments(args, preferred)
	if err != nil {
		return callExpressionSyntax{}, err
	}

	return callExpressionSyntax{
		Name: callName,
		Args: callArgs,
	}, nil
}

func buildStaticCall(name string, args map[string]any, preferred []string) (callExpressionSyntax, error) {
	callName, err := requireDotName(name, "call name")
	if err != nil {
		return callExpressionSyntax{}, err
	}
	callArgs, err := buildNamedStaticArguments(args, preferred)
	if err != nil {
		return callExpressionSyntax{}, err
	}

	return callExpressionSyntax{
		Name: callName,
		Args: callArgs,
	}, nil
}

func buildNamedBindingArguments(args map[string]theater.BindingSpec, preferred []string) ([]callArgumentSyntax, error) {
	order := orderedArgumentNames(args, preferred)
	callArgs := make([]callArgumentSyntax, 0, len(order))
	for _, name := range order {
		argumentName, err := requireLocalIdentifier(name, "call argument")
		if err != nil {
			return nil, err
		}
		value, err := buildBindingExpression(args[name])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, callArgumentSyntax{
			Name:  argumentName,
			Value: value,
		})
	}

	return callArgs, nil
}

func buildNamedStaticArguments(args map[string]any, preferred []string) ([]callArgumentSyntax, error) {
	order := orderedStaticArgumentNames(args, preferred)
	callArgs := make([]callArgumentSyntax, 0, len(order))
	for _, name := range order {
		argumentName, err := requireLocalIdentifier(name, "call argument")
		if err != nil {
			return nil, err
		}
		value, err := buildStaticExpression(args[name])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, callArgumentSyntax{
			Name:  argumentName,
			Value: value,
		})
	}

	return callArgs, nil
}

func buildBindingExpression(spec theater.BindingSpec) (expressionSyntax, error) {
	switch spec.Kind {
	case theater.BindingKindLiteral:
		return buildLiteralExpression(spec.Value)
	case theater.BindingKindRef:
		if spec.Ref == nil {
			return nil, errors.New("ref binding is missing ref")
		}
		return buildRefSelectionExpression(*spec.Ref)
	case theater.BindingKindObject:
		fields := make([]mappingEntrySyntax, 0, len(spec.Object))
		for _, name := range sortedStringKeys(spec.Object) {
			value, err := buildBindingExpression(spec.Object[name])
			if err != nil {
				return nil, err
			}
			fields = append(fields, mappingEntrySyntax{
				Name:  name,
				Value: value,
			})
		}
		return objectExpressionSyntax{
			Dynamic: true,
			Fields:  fields,
		}, nil
	case theater.BindingKindList:
		items := make([]expressionSyntax, 0, len(spec.List))
		for i := range spec.List {
			value, err := buildBindingExpression(spec.List[i])
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return listExpressionSyntax{
			Dynamic: true,
			Items:   items,
		}, nil
	case theater.BindingKindString:
		args := make([]callArgumentSyntax, 0, len(spec.Parts))
		for i := range spec.Parts {
			value, err := buildBindingExpression(spec.Parts[i])
			if err != nil {
				return nil, err
			}
			args = append(args, callArgumentSyntax{Value: value})
		}
		return callExpressionSyntax{
			Name: migrateStringCall,
			Args: args,
		}, nil
	case theater.BindingKindGenerate:
		return buildBindingCall(migrateGeneratorPrefix+spec.Generator, spec.Args, nil)
	default:
		return nil, fmt.Errorf("binding kind %q is not supported by .thtr migrator", spec.Kind)
	}
}

func buildStaticExpression(value any) (expressionSyntax, error) {
	switch typed := value.(type) {
	case string:
		return stringLiteralExpression(typed), nil
	case bool:
		if typed {
			return literalExpressionSyntax{Kind: literalKindBool, Text: "true"}, nil
		}
		return literalExpressionSyntax{Kind: literalKindBool, Text: "false"}, nil
	case nil:
		return literalExpressionSyntax{Kind: literalKindNull, Text: "null"}, nil
	case map[string]any:
		fields := make([]mappingEntrySyntax, 0, len(typed))
		for _, name := range sortedStaticStringKeys(typed) {
			child, err := buildStaticExpression(typed[name])
			if err != nil {
				return nil, err
			}
			fields = append(fields, mappingEntrySyntax{
				Name:  name,
				Value: child,
			})
		}
		return objectExpressionSyntax{
			Dynamic: true,
			Fields:  fields,
		}, nil
	case []any:
		items := make([]expressionSyntax, 0, len(typed))
		for i := range typed {
			child, err := buildStaticExpression(typed[i])
			if err != nil {
				return nil, err
			}
			items = append(items, child)
		}
		return listExpressionSyntax{
			Dynamic: true,
			Items:   items,
		}, nil
	default:
		if isNumericLiteralValue(typed) {
			return buildLiteralExpression(typed)
		}
		return nil, fmt.Errorf("static value type %T is not supported by .thtr migrator", value)
	}
}

func buildLiteralExpression(value any) (expressionSyntax, error) {
	switch typed := value.(type) {
	case string:
		return stringLiteralExpression(typed), nil
	case bool:
		if typed {
			return literalExpressionSyntax{Kind: literalKindBool, Text: "true"}, nil
		}
		return literalExpressionSyntax{Kind: literalKindBool, Text: "false"}, nil
	case nil:
		return literalExpressionSyntax{Kind: literalKindNull, Text: "null"}, nil
	default:
		if numeric, ok, err := buildNumericLiteralExpression(value); ok || err != nil {
			return numeric, err
		}
		return nil, fmt.Errorf("literal type %T is not supported by .thtr migrator", value)
	}
}

func buildNumericLiteralExpression(value any) (expressionSyntax, bool, error) {
	switch typed := value.(type) {
	case int:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatInt(int64(typed), 10)}, true, nil
	case int8:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatInt(int64(typed), 10)}, true, nil
	case int16:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatInt(int64(typed), 10)}, true, nil
	case int32:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatInt(int64(typed), 10)}, true, nil
	case int64:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatInt(typed, 10)}, true, nil
	case uint:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatUint(uint64(typed), 10)}, true, nil
	case uint8:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatUint(uint64(typed), 10)}, true, nil
	case uint16:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatUint(uint64(typed), 10)}, true, nil
	case uint32:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatUint(uint64(typed), 10)}, true, nil
	case uint64:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatUint(typed, 10)}, true, nil
	case float32:
		if err := validateFiniteFloat(float64(typed)); err != nil {
			return nil, true, err
		}
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatFloat(float64(typed), 'g', -1, 32)}, true, nil
	case float64:
		if err := validateFiniteFloat(typed); err != nil {
			return nil, true, err
		}
		return literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.FormatFloat(typed, 'g', -1, 64)}, true, nil
	case json.Number:
		return literalExpressionSyntax{Kind: literalKindNumber, Text: string(typed)}, true, nil
	default:
		return nil, false, nil
	}
}

func buildRefSelectionExpression(spec theater.RefSpec) (expressionSyntax, error) {
	if spec.Name == "" {
		return nil, errors.New("ref selection requires ref name")
	}
	refName, err := requireLocalIdentifier(spec.Name, "ref name")
	if err != nil {
		return nil, err
	}

	steps, err := buildSelectorSteps(spec.Decode, spec.Path, spec.Through)
	if err != nil {
		return nil, err
	}

	base := refExpressionSyntax{Name: refName}
	if len(steps) == 0 {
		return base, nil
	}

	return pipelineExpressionSyntax{
		Base:  base,
		Steps: steps,
	}, nil
}

func buildSelectorSteps(
	decode theater.DecodeKind,
	path theater.JSONPointer,
	through []theater.ThroughStepSpec,
) ([]callExpressionSyntax, error) {
	steps := make([]callExpressionSyntax, 0, 1+len(through))
	if decode != "" {
		decodeName, err := requireLocalIdentifier(string(decode), "decode selector")
		if err != nil {
			return nil, err
		}
		steps = append(steps, positionalCall(migrateSelectorDecode, symbolExpressionSyntax{Name: decodeName}))
	}
	if path != "" {
		steps = append(steps, positionalCall(migrateSelectorPath, stringLiteralExpression(string(path))))
	}
	for i := range through {
		switch {
		case through[i].Path != "":
			steps = append(steps, positionalCall(migrateSelectorPath, stringLiteralExpression(string(through[i].Path))))
		case through[i].Pick != nil:
			if len(through[i].Pick.Where) != 0 {
				clauses, err := buildPickWhereClauses(through[i].Pick.Where)
				if err != nil {
					return nil, err
				}
				steps = append(steps, callExpressionSyntax{
					Name:    migrateSelectorPick,
					Clauses: clauses,
				})
				continue
			}

			equals, err := buildBindingExpression(through[i].Pick.Equals)
			if err != nil {
				return nil, err
			}
			steps = append(steps, callExpressionSyntax{
				Name: migrateSelectorPick,
				Args: []callArgumentSyntax{
					{Name: "at", Value: stringLiteralExpression(string(through[i].Pick.At))},
					{Name: "equals", Value: equals},
				},
			})
		case through[i].Regexp != nil:
			args := []callArgumentSyntax{
				{Name: "pattern", Value: stringLiteralExpression(through[i].Regexp.Pattern)},
			}
			if through[i].Regexp.Group != 0 {
				args = append(args, callArgumentSyntax{
					Name:  "group",
					Value: literalExpressionSyntax{Kind: literalKindNumber, Text: strconv.Itoa(through[i].Regexp.Group)},
				})
			}
			steps = append(steps, callExpressionSyntax{
				Name: migrateSelectorRegexp,
				Args: args,
			})
		case through[i].Transform != nil:
			call, err := buildStaticCall(through[i].Transform.Use, through[i].Transform.With, nil)
			if err != nil {
				return nil, err
			}
			steps = append(steps, call)
		default:
			return nil, errors.New("through step is empty")
		}
	}

	return steps, nil
}

func buildPickWhereClauses(specs []theater.PickWhereClauseSpec) ([]relativeClauseSyntax, error) {
	clauses := make([]relativeClauseSyntax, 0, len(specs))
	for i := range specs {
		subject, err := buildRelativeSubjectExpression(specs[i].Subject)
		if err != nil {
			return nil, err
		}
		assertion, err := buildAssertion(specs[i].Assert)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, relativeClauseSyntax{
			Subject: subject,
			Assert:  assertion,
		})
	}

	return clauses, nil
}

func buildRelativeSubjectExpression(spec theater.RelativeSubjectSpec) (expressionSyntax, error) {
	steps, err := buildSelectorSteps(spec.Decode, spec.Path, nil)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, errors.New("relative clause subject must declare decode or path")
	}
	if len(steps) == 1 {
		return steps[0], nil
	}

	return pipelineExpressionSyntax{
		Base:  steps[0],
		Steps: steps[1:],
	}, nil
}

func matcherArgumentOrder(ref string) []string {
	switch ref {
	case builtinexpectation.BetweenRef:
		return []string{"min", "max"}
	case builtinexpectation.HasEntryRef:
		return []string{"key", "assert"}
	case builtinexpectation.NotRef:
		return []string{"assert"}
	case builtinexpectation.HasItemRef, builtinexpectation.AllItemsRef:
		return []string{"where"}
	default:
		return nil
	}
}

func orderedArgumentNames(args map[string]theater.BindingSpec, preferred []string) []string {
	if len(args) == 0 {
		return nil
	}

	names := make([]string, 0, len(args))
	used := make(map[string]struct{}, len(preferred))
	for _, name := range preferred {
		if _, ok := args[name]; !ok {
			continue
		}
		names = append(names, name)
		used[name] = struct{}{}
	}

	rest := make([]string, 0, len(args)-len(names))
	for name := range args {
		if _, ok := used[name]; ok {
			continue
		}
		rest = append(rest, name)
	}
	sort.Strings(rest)
	return append(names, rest...)
}

func orderedStaticArgumentNames(args map[string]any, preferred []string) []string {
	if len(args) == 0 {
		return nil
	}

	names := make([]string, 0, len(args))
	used := make(map[string]struct{}, len(preferred))
	for _, name := range preferred {
		if _, ok := args[name]; !ok {
			continue
		}
		names = append(names, name)
		used[name] = struct{}{}
	}

	rest := make([]string, 0, len(args)-len(names))
	for name := range args {
		if _, ok := used[name]; ok {
			continue
		}
		rest = append(rest, name)
	}
	sort.Strings(rest)
	return append(names, rest...)
}

func sortedStringKeys[V any](items map[string]V) []string {
	if len(items) == 0 {
		return nil
	}

	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedStaticStringKeys(items map[string]any) []string {
	return sortedStringKeys(items)
}

func sortedValueContractKeys(items map[string]theater.ValueContract) []string {
	return sortedStringKeys(items)
}

func sortedPropertyKeys(items map[string]theater.PropertySpec) []string {
	return sortedStringKeys(items)
}

func sortedCaptureSourceKeys(items map[string]theater.HTTPCaptureSourceSpec) []string {
	return sortedStringKeys(items)
}

func requireDotName(name, context string) (string, error) {
	return requireSegmentedIdentifier(name, ".", context)
}

func requireLocalIdentifier(name, context string) (string, error) {
	if !validLocalIdentifier(name) {
		return "", fmt.Errorf("%s %q is not encodable as bare .thtr identifier", context, name)
	}

	return name, nil
}

func requireSegmentedIdentifier(name, separator, context string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s must not be empty", context)
	}

	parts := strings.Split(name, separator)
	for _, part := range parts {
		if !validLocalIdentifier(part) {
			return "", fmt.Errorf("%s %q is not encodable as .thtr name", context, name)
		}
	}

	return name, nil
}

func requireSlashName(name, context string) (string, error) {
	return requireSegmentedIdentifier(name, "/", context)
}

func validLocalIdentifier(name string) bool {
	if name == "" {
		return false
	}

	for i, r := range name {
		if i == 0 {
			if !isIdentifierStart(r) {
				return false
			}
			continue
		}
		if !isIdentifierPart(r) {
			return false
		}
	}

	return true
}

func positionalCall(name string, value expressionSyntax) callExpressionSyntax {
	return callExpressionSyntax{
		Name: name,
		Args: []callArgumentSyntax{{Value: value}},
	}
}

func stringLiteralExpression(value string) literalExpressionSyntax {
	return literalExpressionSyntax{
		Kind: literalKindString,
		Text: strconv.Quote(value),
	}
}

func dependencyPredicateLabel(predicate theater.TriggerPredicate) string {
	switch predicate {
	case "", theater.TriggerPredicateSuccess:
		return ""
	default:
		return string(predicate)
	}
}

func transitionEventLabel(outcome theater.TransitionOutcome) string {
	return strings.TrimPrefix(string(outcome), "on_")
}

func isNumericLiteralValue(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		json.Number:
		return true
	default:
		return false
	}
}

func validateFiniteFloat(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("numeric literal must be finite, got %v", value)
	}

	return nil
}
