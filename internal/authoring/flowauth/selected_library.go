package flowauth

import (
	"fmt"
	"sort"

	"github.com/alex-poliushkin/theater"
)

const (
	httpActionRef       = "action.http"
	httpInventoryGetRef = "inventory.http.get"
)

// SelectedLibraryAuthError reports a repo-aware auth composition failure.
type SelectedLibraryAuthError struct {
	Code            string
	AuthName        string
	AttachmentIndex int
	Summary         string
}

func (e *SelectedLibraryAuthError) Error() string {
	return e.Summary
}

// DeclaredHTTPAuthNames returns the auth names already owned by a flow.
func DeclaredHTTPAuthNames(httpSpec *theater.HTTPSpec) map[string]struct{} {
	names := make(map[string]struct{})
	if httpSpec == nil {
		return names
	}
	for name := range httpSpec.Auth {
		names[name] = struct{}{}
	}
	return names
}

// SelectedScenarioHTTPAuthNames returns auth names used by selected scenarios.
func SelectedScenarioHTTPAuthNames(
	scenarios []theater.ScenarioSpec,
	selectedScenarioIDs map[string]struct{},
) map[string]struct{} {
	authNames := make(map[string]struct{})
	for i := range scenarios {
		scenario := scenarios[i]
		if _, selected := selectedScenarioIDs[scenario.ID]; !selected {
			continue
		}

		collectScenarioHTTPAuthNames(authNames, scenario)
	}

	return authNames
}

// ValidateSelectedLibraryHTTPAuth checks selected-library auth declarations.
func ValidateSelectedLibraryHTTPAuth(
	librarySpec theater.StageSpec,
	selectedScenarioIDs map[string]struct{},
	flowAuthNames map[string]struct{},
	composedAuthOwners map[string]string,
	libraryFile string,
) error {
	issues := SelectedLibraryHTTPAuthIssues(
		librarySpec,
		selectedScenarioIDs,
		flowAuthNames,
		composedAuthOwners,
		libraryFile,
	)
	if len(issues) > 0 {
		return &issues[0]
	}
	return nil
}

// SelectedLibraryHTTPAuthIssues returns selected-library auth diagnostics while
// preserving the same owner rules used by ValidateSelectedLibraryHTTPAuth.
func SelectedLibraryHTTPAuthIssues(
	librarySpec theater.StageSpec,
	selectedScenarioIDs map[string]struct{},
	flowAuthNames map[string]struct{},
	composedAuthOwners map[string]string,
	libraryFile string,
) []SelectedLibraryAuthError {
	if librarySpec.HTTP == nil || len(librarySpec.HTTP.Auth) == 0 || len(selectedScenarioIDs) == 0 {
		return nil
	}

	var issues []SelectedLibraryAuthError
	for _, authName := range sortedHTTPAuthNames(librarySpec.HTTP.Auth) {
		auth := librarySpec.HTTP.Auth[authName]
		if attachmentIndex, ok := firstNonSlotBackedHTTPAuthAttachment(auth); ok {
			issues = append(issues, SelectedLibraryAuthError{
				Code:            "invalid_selected_library_http_auth",
				AuthName:        authName,
				AttachmentIndex: attachmentIndex,
				Summary: fmt.Sprintf(
					"selected library http auth %q must use slot-backed attachments in %s",
					authName,
					libraryFile,
				),
			})
			continue
		}
		if _, exists := flowAuthNames[authName]; exists {
			issues = append(issues, SelectedLibraryAuthError{
				Code:            "duplicate_selected_library_http_auth",
				AuthName:        authName,
				AttachmentIndex: -1,
				Summary: fmt.Sprintf(
					"http auth %q is declared by both flow and selected library file %s",
					authName,
					libraryFile,
				),
			})
			continue
		}
		if owner, exists := composedAuthOwners[authName]; exists {
			issues = append(issues, SelectedLibraryAuthError{
				Code:            "duplicate_selected_library_http_auth",
				AuthName:        authName,
				AttachmentIndex: -1,
				Summary: fmt.Sprintf(
					"http auth %q is declared by multiple selected library files: %s, %s",
					authName,
					owner,
					libraryFile,
				),
			})
			continue
		}

		composedAuthOwners[authName] = libraryFile
	}

	return issues
}

// ComposeSelectedLibraryHTTPAuth copies selected auth entries into a flow.
func ComposeSelectedLibraryHTTPAuth(
	assembled *theater.StageSpec,
	librarySpec theater.StageSpec,
	selectedAuthNames map[string]struct{},
) {
	if librarySpec.HTTP == nil || len(librarySpec.HTTP.Auth) == 0 || len(selectedAuthNames) == 0 {
		return
	}

	for _, authName := range sortedStringSet(selectedAuthNames) {
		auth, ok := librarySpec.HTTP.Auth[authName]
		if !ok {
			continue
		}

		ensureHTTPAuthMap(assembled)
		assembled.HTTP.Auth[authName] = auth.Clone()
	}
}

func collectScenarioHTTPAuthNames(authNames map[string]struct{}, scenario theater.ScenarioSpec) {
	for authName := range scenario.AuthBindings {
		authNames[authName] = struct{}{}
	}
	for i := range scenario.Acts {
		act := scenario.Acts[i]
		if act.CaptureAuth != nil && act.CaptureAuth.Auth != "" {
			authNames[act.CaptureAuth.Auth] = struct{}{}
		}
		if act.Action.Use == httpActionRef {
			collectLiteralHTTPAuthName(authNames, act.Action.With)
		}
		collectHTTPInventoryAuthNames(authNames, act.Properties)
	}
}

func collectHTTPInventoryAuthNames(authNames map[string]struct{}, properties map[string]theater.PropertySpec) {
	for propertyName := range properties {
		property := properties[propertyName]
		if property.Inventory == nil || property.Inventory.Use != httpInventoryGetRef {
			continue
		}
		collectLiteralHTTPAuthName(authNames, property.Inventory.With)
	}
}

func collectLiteralHTTPAuthName(authNames map[string]struct{}, bindings map[string]theater.BindingSpec) {
	authName, ok := literalHTTPAuthName(bindings)
	if ok && authName != "" && authName != theater.HTTPAuthNone {
		authNames[authName] = struct{}{}
	}
}

func literalHTTPAuthName(bindings map[string]theater.BindingSpec) (string, bool) {
	if bindings == nil {
		return "", false
	}
	binding, ok := bindings["auth"]
	if !ok || binding.Kind != theater.BindingKindLiteral {
		return "", false
	}

	value, ok := binding.Value.(string)
	return value, ok
}

func ensureHTTPAuthMap(stage *theater.StageSpec) {
	if stage.HTTP == nil {
		stage.HTTP = &theater.HTTPSpec{}
	}
	if stage.HTTP.Auth == nil {
		stage.HTTP.Auth = make(map[string]theater.HTTPAuthSpec)
	}
}

func firstNonSlotBackedHTTPAuthAttachment(auth theater.HTTPAuthSpec) (int, bool) {
	for i := range auth.Attach {
		if !isSlotBackedHTTPAuthAttachment(auth.Attach[i]) {
			return i, true
		}
	}
	return -1, false
}

func isSlotBackedHTTPAuthAttachment(attachment theater.HTTPAuthAttachmentSpec) bool {
	if httpAuthAttachmentKindCount(attachment) != 1 {
		return false
	}

	switch {
	case attachment.Bearer != nil:
		return attachment.Bearer.Token == "" && attachment.Bearer.TokenSlot != ""
	case attachment.HeaderSlot != nil:
		return attachment.HeaderSlot.Slot != ""
	case attachment.QuerySlot != nil:
		return attachment.QuerySlot.Slot != ""
	case attachment.FormSlot != nil:
		return attachment.FormSlot.Slot != ""
	default:
		return false
	}
}

func httpAuthAttachmentKindCount(attachment theater.HTTPAuthAttachmentSpec) int {
	count := 0
	if attachment.Bearer != nil {
		count++
	}
	if attachment.Basic != nil {
		count++
	}
	if attachment.APIKey != nil {
		count++
	}
	if attachment.HeaderSlot != nil {
		count++
	}
	if attachment.QuerySlot != nil {
		count++
	}
	if attachment.FormSlot != nil {
		count++
	}
	return count
}

func sortedHTTPAuthNames(auth map[string]theater.HTTPAuthSpec) []string {
	names := make([]string, 0, len(auth))
	for name := range auth {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedStringSet(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}
