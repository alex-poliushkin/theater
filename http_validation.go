package theater

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

const (
	httpActionRef       = "action.http"
	httpInventoryGetRef = "inventory.http.get"
)

func validateHTTPAuthoring(stage *stagePlan) []Diagnostic {
	diagnostics := validateHTTPRegistry(stage)

	for i := range stage.Scenarios {
		scenario := &stage.Scenarios[i]
		for j := range scenario.Acts {
			act := &scenario.Acts[j]
			if act.Action.Use == httpActionRef {
				diagnostics = append(diagnostics, validateHTTPRequestBindings(stage.HTTP, act.Path+"/action", act.Action.With)...)
			}

			diagnostics = append(diagnostics, validateHTTPCapture(stage.HTTP, act)...)

			for k := range act.Properties {
				property := &act.Properties[k]
				if property.Inventory.Use != httpInventoryGetRef {
					continue
				}

				diagnostics = append(
					diagnostics,
					validateHTTPRequestBindings(stage.HTTP, property.Path+"/inventory/with", property.Inventory.With)...,
				)
			}
		}
	}

	return diagnostics
}

func validateHTTPRegistry(stage *stagePlan) []Diagnostic {
	if stage == nil || stage.HTTP == nil {
		return nil
	}

	diagnostics := make([]Diagnostic, 0)
	for name := range stage.HTTP.Sessions {
		path := httpSessionPath(stage.Path, name)
		if err := validateRefName(name); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_http_session_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http session %q is invalid: %v", name, err),
			})
		}
		if name == HTTPSessionNone {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "reserved_http_session_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http session name %q is reserved", HTTPSessionNone),
			})
		}
	}

	for name, auth := range stage.HTTP.Auth {
		path := httpAuthPath(stage.Path, name)
		if err := validateRefName(name); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_http_auth_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http auth %q is invalid: %v", name, err),
			})
		}
		if name == HTTPAuthNone {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "reserved_http_auth_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http auth name %q is reserved", HTTPAuthNone),
			})
		}
		if len(auth.Attach) == 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_http_auth_attach",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http auth %q must declare at least one attachment", name),
			})
			continue
		}

		diagnostics = append(diagnostics, validateHTTPAuthAttachments(path, auth)...)
	}

	for name, identity := range stage.HTTP.Identities {
		path := httpIdentityPath(stage.Path, name)
		if err := validateRefName(name); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_http_identity_name",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http identity %q is invalid: %v", name, err),
			})
		}
		if identity.Session == "" && identity.Auth == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "missing_http_identity_targets",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http identity %q must declare session or auth", name),
			})
		}
		if identity.Session != "" && !hasHTTPSession(stage.HTTP, identity.Session) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_http_identity_session_ref",
				Path:     path + "/session",
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http identity session %q is not declared in stage http.sessions", identity.Session),
			})
		}
		if identity.Auth != "" && !hasHTTPAuth(stage.HTTP, identity.Auth) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_http_identity_auth_ref",
				Path:     path + "/auth",
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http identity auth %q is not declared in stage http.auth", identity.Auth),
			})
		}
	}

	return diagnostics
}

func validateHTTPAuthAttachments(path string, auth HTTPAuthSpec) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	headerTargets := make(map[string]struct{}, len(auth.Attach))
	queryTargets := make(map[string]struct{}, len(auth.Attach))
	formTargets := make(map[string]struct{}, len(auth.Attach))

	for i := range auth.Attach {
		attachmentPath := httpAttachPath(path, i)
		attachment := auth.Attach[i]
		if httpAttachmentKindCount(attachment) != 1 {
			diagnostics = append(diagnostics, invalidHTTPAttachmentDiagnostic(attachmentPath))
			continue
		}

		switch {
		case attachment.Bearer != nil:
			if attachment.Bearer.Token == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_bearer_token",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "bearer attachment token is required",
				})
			}
			diagnostics = append(diagnostics, validateHTTPHeaderTarget(attachmentPath, "Authorization", headerTargets)...)
		case attachment.Basic != nil:
			if attachment.Basic.Username == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_basic_username",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "basic attachment username is required",
				})
			}
			if attachment.Basic.Password == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_basic_password",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "basic attachment password is required",
				})
			}
			diagnostics = append(diagnostics, validateHTTPHeaderTarget(attachmentPath, "Authorization", headerTargets)...)
		case attachment.APIKey != nil:
			if !attachment.APIKey.In.Valid() {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "invalid_http_auth_api_key_in",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  fmt.Sprintf("api_key attachment in %q is invalid", attachment.APIKey.In),
				})
			}
			if attachment.APIKey.Name == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_api_key_name",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "api_key attachment name is required",
				})
			}
			if attachment.APIKey.Value == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_api_key_value",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "api_key attachment value is required",
				})
			}
			switch attachment.APIKey.In {
			case HTTPAPIKeyInHeader:
				diagnostics = append(diagnostics, validateHTTPHeaderTarget(attachmentPath, attachment.APIKey.Name, headerTargets)...)
			case HTTPAPIKeyInQuery:
				diagnostics = append(diagnostics, validateHTTPQueryTarget(attachmentPath, attachment.APIKey.Name, queryTargets)...)
			}
		case attachment.HeaderSlot != nil:
			if attachment.HeaderSlot.Slot == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_slot_name",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "slot attachment slot is required",
				})
			}
			diagnostics = append(diagnostics, validateHTTPHeaderTarget(attachmentPath, attachment.HeaderSlot.Name, headerTargets)...)
		case attachment.QuerySlot != nil:
			if attachment.QuerySlot.Slot == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_slot_name",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "slot attachment slot is required",
				})
			}
			diagnostics = append(diagnostics, validateHTTPQueryTarget(attachmentPath, attachment.QuerySlot.Name, queryTargets)...)
		case attachment.FormSlot != nil:
			if attachment.FormSlot.Slot == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "missing_http_auth_slot_name",
					Path:     attachmentPath,
					Severity: SeverityError,
					Summary:  "slot attachment slot is required",
				})
			}
			diagnostics = append(diagnostics, validateHTTPFormTarget(attachmentPath, attachment.FormSlot.Name, formTargets)...)
		}
	}

	return diagnostics
}

func invalidHTTPAttachmentDiagnostic(path string) Diagnostic {
	return Diagnostic{
		Code:     "invalid_http_auth_attachment",
		Path:     path,
		Severity: SeverityError,
		Summary:  "http auth attachment must declare exactly one of bearer, basic, api_key, header_slot, query_slot, or form_slot",
	}
}

func validateHTTPHeaderTarget(path, name string, seen map[string]struct{}) []Diagnostic {
	canonical := http.CanonicalHeaderKey(name)
	if canonical == "" {
		return []Diagnostic{{
			Code:     "missing_http_auth_header_name",
			Path:     path,
			Severity: SeverityError,
			Summary:  "http auth header target is required",
		}}
	}
	if _, ok := seen[canonical]; ok {
		return []Diagnostic{{
			Code:     "duplicate_http_auth_header",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("http auth header %q is duplicated", canonical),
		}}
	}

	seen[canonical] = struct{}{}
	return nil
}

func validateHTTPQueryTarget(path, name string, seen map[string]struct{}) []Diagnostic {
	if name == "" {
		return []Diagnostic{{
			Code:     "missing_http_auth_query_name",
			Path:     path,
			Severity: SeverityError,
			Summary:  "http auth query target is required",
		}}
	}
	if _, ok := seen[name]; ok {
		return []Diagnostic{{
			Code:     "duplicate_http_auth_query",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("http auth query parameter %q is duplicated", name),
		}}
	}

	seen[name] = struct{}{}
	return nil
}

func validateHTTPFormTarget(path, name string, seen map[string]struct{}) []Diagnostic {
	if name == "" {
		return []Diagnostic{{
			Code:     "missing_http_auth_form_name",
			Path:     path,
			Severity: SeverityError,
			Summary:  "http auth form field target is required",
		}}
	}
	if _, ok := seen[name]; ok {
		return []Diagnostic{{
			Code:     "duplicate_http_auth_form",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("http auth form field %q is duplicated", name),
		}}
	}

	seen[name] = struct{}{}
	return nil
}

func validateHTTPCapture(httpSpec *HTTPSpec, act *actPlan) []Diagnostic {
	if act == nil || act.CaptureAuth == nil {
		return nil
	}

	capturePath := httpCapturePath(act.Path)
	if act.Action.Use != httpActionRef {
		return []Diagnostic{{
			Code:     "invalid_http_capture_auth_usage",
			Path:     capturePath,
			Severity: SeverityError,
			Summary:  "capture_auth is only supported for action.http",
		}}
	}

	diagnostics := make([]Diagnostic, 0)
	if act.CaptureAuth.Auth == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_http_capture_auth_ref",
			Path:     capturePath + "/auth",
			Severity: SeverityError,
			Summary:  "capture_auth auth ref is required",
		})
	}

	var auth HTTPAuthSpec
	authKnown := false
	if act.CaptureAuth.Auth != "" {
		if httpSpec == nil || !hasHTTPAuth(httpSpec, act.CaptureAuth.Auth) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_http_capture_auth_ref",
				Path:     capturePath + "/auth",
				Severity: SeverityError,
				Summary:  fmt.Sprintf("http auth %q is not declared in stage http.auth", act.CaptureAuth.Auth),
			})
		} else {
			auth = httpSpec.Auth[act.CaptureAuth.Auth]
			authKnown = true
		}
	}

	if len(act.CaptureAuth.Slots) == 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "missing_http_capture_slots",
			Path:     capturePath,
			Severity: SeverityError,
			Summary:  "capture_auth must declare at least one slot",
		})
		return diagnostics
	}

	for slot, source := range act.CaptureAuth.Slots {
		slotPath := httpCaptureSlotPath(capturePath, slot)
		if slot == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "invalid_http_capture_slot_name",
				Path:     slotPath,
				Severity: SeverityError,
				Summary:  "capture_auth slot name is required",
			})
			continue
		}

		if authKnown && !authUsesSlot(auth, slot) {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unknown_http_capture_slot_ref",
				Path:     slotPath,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("auth slot %q is not declared by target http auth", slot),
			})
		}

		diagnostics = append(diagnostics, validateHTTPCaptureSource(slotPath, source)...)
	}

	return diagnostics
}

func validateHTTPCaptureSource(path string, source HTTPCaptureSourceSpec) []Diagnostic {
	configured := 0
	if source.ResponseHeader != "" {
		configured++
	}
	if source.ResponseCookie != "" {
		configured++
	}
	if !source.JSONPointer.IsZero() {
		configured++
	}
	if source.FormField != "" {
		configured++
	}
	if configured != 1 {
		return []Diagnostic{{
			Code:     "invalid_http_capture_source",
			Path:     path,
			Severity: SeverityError,
			Summary:  "capture_auth slot source must declare exactly one of response_header, response_cookie, json_pointer, or form_field",
		}}
	}
	if !source.JSONPointer.IsZero() {
		if err := source.JSONPointer.Validate(); err != nil {
			return []Diagnostic{{
				Code:     "invalid_http_capture_json_pointer",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("capture_auth json_pointer is invalid: %v", err),
			}}
		}
	}

	return nil
}

func validateHTTPRequestBindings(httpSpec *HTTPSpec, parentPath string, bindings map[string]bindingPlan) []Diagnostic {
	sessionBinding, hasSession := bindings["session"]
	authBinding, hasAuth := bindings["auth"]
	identityBinding, hasIdentity := bindings["identity"]
	formBinding, hasForm := bindings["form"]
	_, hasJSON := bindings["json"]
	_, hasBody := bindings["body"]

	diagnostics := make([]Diagnostic, 0)

	sessionMode, sessionName, sessionKnown := literalHTTPSessionSelection(sessionBinding, hasSession)
	diagnostics = append(
		diagnostics,
		validateHTTPSessionSelection(parentPath, hasSession, sessionMode, sessionName, sessionKnown)...,
	)

	authMode, authName, authKnown := literalHTTPAuthSelection(authBinding, hasAuth)
	diagnostics = append(
		diagnostics,
		validateHTTPAuthSelection(parentPath, hasAuth, authMode, authName, authKnown)...,
	)

	identityName, identityKnown := literalBindingString(identityBinding, hasIdentity)
	diagnostics = append(
		diagnostics,
		validateHTTPIdentitySelection(parentPath, hasIdentity, identityName, identityKnown)...,
	)

	identity, identityKnownAndResolved, identityDiagnostics := resolveHTTPIdentitySelection(
		httpSpec,
		parentPath,
		hasIdentity,
		identityName,
		identityKnown,
	)
	diagnostics = append(diagnostics, identityDiagnostics...)

	effectiveSessionMode, effectiveSessionName, effectiveSessionKnown := resolveEffectiveHTTPSessionSelection(
		hasSession,
		sessionMode,
		sessionName,
		sessionKnown,
		identity,
		identityKnownAndResolved,
	)
	diagnostics = append(
		diagnostics,
		validateEffectiveHTTPSessionSelection(httpSpec, parentPath, effectiveSessionMode, effectiveSessionName, effectiveSessionKnown)...,
	)

	effectiveAuthMode, effectiveAuthName, effectiveAuthKnown := resolveEffectiveHTTPAuthSelection(
		hasAuth,
		authMode,
		authName,
		authKnown,
		identity,
		identityKnownAndResolved,
	)
	diagnostics = append(
		diagnostics,
		validateEffectiveHTTPAuthSelection(httpSpec, parentPath, effectiveAuthMode, effectiveAuthName, effectiveAuthKnown)...,
	)

	headers := bindingObjectKeys(headersBinding(bindings))
	formFields := bindingObjectKeys(formBinding)
	diagnostics = append(
		diagnostics,
		validateHTTPManagedSessionAndFormConflicts(
			parentPath,
			headers,
			hasForm,
			hasJSON,
			hasBody,
			effectiveSessionMode,
			effectiveSessionKnown,
		)...,
	)

	effectiveAuth, authResolved := HTTPAuthSpec{}, false
	if effectiveAuthKnown && effectiveAuthMode == literalHTTPAuthNamed && httpSpec != nil {
		effectiveAuth, authResolved = httpSpec.Auth[effectiveAuthName]
	}
	diagnostics = append(
		diagnostics,
		validateHTTPFormContentType(parentPath, bindings, hasForm, authResolved, effectiveAuth)...,
	)
	diagnostics = append(
		diagnostics,
		validateHTTPJSONContentType(parentPath, bindings, hasJSON)...,
	)

	if authResolved {
		diagnostics = append(
			diagnostics,
			validateHTTPManualAuthConflicts(parentPath, headers, formFields, literalURLBinding(bindings), hasBody, effectiveAuth)...,
		)
	}

	return diagnostics
}

func validateHTTPSessionSelection(
	parentPath string,
	hasSession bool,
	sessionMode literalHTTPSessionMode,
	sessionName string,
	sessionKnown bool,
) []Diagnostic {
	if !hasSession || !sessionKnown || sessionName != "" || sessionMode == httpclientSessionNone {
		return nil
	}

	return []Diagnostic{{
		Code:     "invalid_http_session_ref",
		Path:     bindingPath(parentPath, "session"),
		Severity: SeverityError,
		Summary:  "http session must not be empty",
	}}
}

func validateHTTPAuthSelection(
	parentPath string,
	hasAuth bool,
	authMode literalHTTPAuthMode,
	authName string,
	authKnown bool,
) []Diagnostic {
	if !hasAuth || !authKnown || authMode != literalHTTPAuthNamed || authName != "" {
		return nil
	}

	return []Diagnostic{{
		Code:     "invalid_http_auth_ref",
		Path:     bindingPath(parentPath, "auth"),
		Severity: SeverityError,
		Summary:  "http auth ref must not be empty",
	}}
}

func validateHTTPIdentitySelection(
	parentPath string,
	hasIdentity bool,
	identityName string,
	identityKnown bool,
) []Diagnostic {
	if !hasIdentity || !identityKnown || identityName != "" {
		return nil
	}

	return []Diagnostic{{
		Code:     "invalid_http_identity_ref",
		Path:     bindingPath(parentPath, "identity"),
		Severity: SeverityError,
		Summary:  "http identity ref must not be empty",
	}}
}

func resolveHTTPIdentitySelection(
	httpSpec *HTTPSpec,
	parentPath string,
	hasIdentity bool,
	identityName string,
	identityKnown bool,
) (identity HTTPIdentitySpec, resolved bool, diagnostics []Diagnostic) {
	if !hasIdentity || !identityKnown || identityName == "" {
		return HTTPIdentitySpec{}, false, nil
	}
	if httpSpec == nil || !hasHTTPIdentity(httpSpec, identityName) {
		return HTTPIdentitySpec{}, false, []Diagnostic{{
			Code:     "unknown_http_identity_ref",
			Path:     bindingPath(parentPath, "identity"),
			Severity: SeverityError,
			Summary:  fmt.Sprintf("http identity %q is not declared in stage http.identities", identityName),
		}}
	}

	return httpSpec.Identities[identityName], true, nil
}

func resolveEffectiveHTTPSessionSelection(
	hasSession bool,
	sessionMode literalHTTPSessionMode,
	sessionName string,
	sessionKnown bool,
	identity HTTPIdentitySpec,
	identityResolved bool,
) (literalHTTPSessionMode, string, bool) {
	mode := httpclientSessionDefault
	name := ""
	known := true
	if identityResolved && identity.Session != "" {
		mode = httpclientSessionNamed
		name = identity.Session
	}
	if hasSession {
		mode = sessionMode
		name = sessionName
		known = sessionKnown
	}

	return mode, name, known
}

func validateEffectiveHTTPSessionSelection(
	httpSpec *HTTPSpec,
	parentPath string,
	mode literalHTTPSessionMode,
	name string,
	known bool,
) []Diagnostic {
	if !known || mode != httpclientSessionNamed {
		return nil
	}
	if hasHTTPSession(httpSpec, name) {
		return nil
	}

	return []Diagnostic{{
		Code:     "unknown_http_session_ref",
		Path:     bindingPath(parentPath, "session"),
		Severity: SeverityError,
		Summary:  fmt.Sprintf("http session %q is not declared in stage http.sessions", name),
	}}
}

func resolveEffectiveHTTPAuthSelection(
	hasAuth bool,
	authMode literalHTTPAuthMode,
	authName string,
	authKnown bool,
	identity HTTPIdentitySpec,
	identityResolved bool,
) (literalHTTPAuthMode, string, bool) {
	mode := literalHTTPAuthDefault
	name := ""
	known := true
	if identityResolved && identity.Auth != "" {
		mode = literalHTTPAuthNamed
		name = identity.Auth
	}
	if hasAuth {
		mode = authMode
		name = authName
		known = authKnown
	}

	return mode, name, known
}

func validateEffectiveHTTPAuthSelection(
	httpSpec *HTTPSpec,
	parentPath string,
	mode literalHTTPAuthMode,
	name string,
	known bool,
) []Diagnostic {
	if !known || mode != literalHTTPAuthNamed {
		return nil
	}
	if hasHTTPAuth(httpSpec, name) {
		return nil
	}

	return []Diagnostic{{
		Code:     "unknown_http_auth_ref",
		Path:     bindingPath(parentPath, "auth"),
		Severity: SeverityError,
		Summary:  fmt.Sprintf("http auth %q is not declared in stage http.auth", name),
	}}
}

func validateHTTPManagedSessionAndFormConflicts(
	parentPath string,
	headers map[string]struct{},
	hasForm bool,
	hasJSON bool,
	hasBody bool,
	effectiveSessionMode literalHTTPSessionMode,
	effectiveSessionKnown bool,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if effectiveSessionKnown && effectiveSessionMode != httpclientSessionNone && hasCookieHeaderBinding(headers) {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "conflicting_http_cookie_header",
			Path:     bindingPath(parentPath, "headers"),
			Severity: SeverityError,
			Summary:  "headers.Cookie is incompatible with managed HTTP sessions",
		})
	}
	if hasForm && hasBody {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "conflicting_http_form_body",
			Path:     bindingPath(parentPath, "form"),
			Severity: SeverityError,
			Summary:  "form is incompatible with body",
		})
	}
	if hasJSON && hasBody {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "conflicting_http_json_body",
			Path:     bindingPath(parentPath, "json"),
			Severity: SeverityError,
			Summary:  "json is incompatible with body",
		})
	}
	if hasJSON && hasForm {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "conflicting_http_json_form",
			Path:     bindingPath(parentPath, "json"),
			Severity: SeverityError,
			Summary:  "json is incompatible with form",
		})
	}

	return diagnostics
}

func validateHTTPFormContentType(
	parentPath string,
	bindings map[string]bindingPlan,
	hasForm bool,
	authResolved bool,
	auth HTTPAuthSpec,
) []Diagnostic {
	if !hasForm && (!authResolved || !authUsesForm(auth)) {
		return nil
	}

	contentType, ok := bindingObjectString(headersBinding(bindings), "Content-Type")
	if !ok || isFormContentType(contentType) {
		return nil
	}

	return []Diagnostic{{
		Code:     "conflicting_http_form_content_type",
		Path:     bindingPath(parentPath, "headers"),
		Severity: SeverityError,
		Summary:  `headers.Content-Type must be "application/x-www-form-urlencoded" when form is used`,
	}}
}

func validateHTTPJSONContentType(
	parentPath string,
	bindings map[string]bindingPlan,
	hasJSON bool,
) []Diagnostic {
	if !hasJSON {
		return nil
	}

	contentType, ok := bindingObjectString(headersBinding(bindings), "Content-Type")
	if !ok || isJSONContentType(contentType) {
		return nil
	}

	return []Diagnostic{{
		Code:     "conflicting_http_json_content_type",
		Path:     bindingPath(parentPath, "headers"),
		Severity: SeverityError,
		Summary:  `headers.Content-Type must be "application/json" when json is used`,
	}}
}

func validateHTTPManualAuthConflicts(
	parentPath string,
	headers map[string]struct{},
	formFields map[string]struct{},
	rawURL string,
	hasBody bool,
	auth HTTPAuthSpec,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	seenHeaders := make(map[string]struct{}, len(headers))
	for key := range headers {
		seenHeaders[http.CanonicalHeaderKey(key)] = struct{}{}
	}

	parsedURL, _ := url.Parse(rawURL)
	query := url.Values(nil)
	if parsedURL != nil {
		query = parsedURL.Query()
	}

	for i := range auth.Attach {
		diagnostics = append(
			diagnostics,
			validateHTTPManualAuthConflict(parentPath, seenHeaders, formFields, query, hasBody, auth.Attach[i])...,
		)
	}

	return diagnostics
}

func validateHTTPManualAuthConflict(
	parentPath string,
	headers map[string]struct{},
	formFields map[string]struct{},
	query url.Values,
	hasBody bool,
	attachment HTTPAuthAttachmentSpec,
) []Diagnostic {
	switch {
	case attachment.Bearer != nil, attachment.Basic != nil:
		return validateHTTPAuthorizationHeaderConflict(parentPath, headers)
	case attachment.APIKey != nil && attachment.APIKey.In == HTTPAPIKeyInHeader:
		return validateHTTPHeaderTargetConflict(parentPath, headers, attachment.APIKey.Name)
	case attachment.APIKey != nil && attachment.APIKey.In == HTTPAPIKeyInQuery:
		return validateHTTPQueryTargetConflict(parentPath, query, attachment.APIKey.Name)
	case attachment.HeaderSlot != nil:
		return validateHTTPHeaderTargetConflict(parentPath, headers, attachment.HeaderSlot.Name)
	case attachment.QuerySlot != nil:
		return validateHTTPQueryTargetConflict(parentPath, query, attachment.QuerySlot.Name)
	case attachment.FormSlot != nil:
		return validateHTTPFormTargetConflict(parentPath, formFields, hasBody, attachment.FormSlot.Name)
	default:
		return nil
	}
}

func validateHTTPAuthorizationHeaderConflict(parentPath string, headers map[string]struct{}) []Diagnostic {
	if _, ok := headers["Authorization"]; !ok {
		return nil
	}

	return []Diagnostic{{
		Code:     "conflicting_http_auth_header",
		Path:     bindingPath(parentPath, "headers"),
		Severity: SeverityError,
		Summary:  `headers.Authorization is incompatible with typed HTTP auth`,
	}}
}

func validateHTTPHeaderTargetConflict(parentPath string, headers map[string]struct{}, name string) []Diagnostic {
	canonical := http.CanonicalHeaderKey(name)
	if _, ok := headers[canonical]; !ok {
		return nil
	}

	return []Diagnostic{{
		Code:     "conflicting_http_auth_header",
		Path:     bindingPath(parentPath, "headers"),
		Severity: SeverityError,
		Summary:  fmt.Sprintf("headers.%s is incompatible with typed HTTP auth", canonical),
	}}
}

func validateHTTPQueryTargetConflict(parentPath string, query url.Values, name string) []Diagnostic {
	if query == nil {
		return nil
	}
	if _, ok := query[name]; !ok {
		return nil
	}

	return []Diagnostic{{
		Code:     "conflicting_http_auth_query",
		Path:     bindingPath(parentPath, "url"),
		Severity: SeverityError,
		Summary:  fmt.Sprintf("url query parameter %q is incompatible with typed HTTP auth", name),
	}}
}

func validateHTTPFormTargetConflict(
	parentPath string,
	formFields map[string]struct{},
	hasBody bool,
	name string,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0, 2)
	if hasBody {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "conflicting_http_auth_form_body",
			Path:     bindingPath(parentPath, "body"),
			Severity: SeverityError,
			Summary:  "body is incompatible with typed HTTP auth form fields",
		})
	}
	if _, ok := formFields[name]; ok {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "conflicting_http_auth_form",
			Path:     bindingPath(parentPath, "form"),
			Severity: SeverityError,
			Summary:  fmt.Sprintf("form.%s is incompatible with typed HTTP auth", name),
		})
	}

	return diagnostics
}

type literalHTTPSessionMode int

const (
	httpclientSessionUnknown literalHTTPSessionMode = iota
	httpclientSessionDefault
	httpclientSessionNamed
	httpclientSessionNone
)

type literalHTTPAuthMode int

const (
	literalHTTPAuthUnknown literalHTTPAuthMode = iota
	literalHTTPAuthDefault
	literalHTTPAuthNamed
	literalHTTPAuthNone
)

func literalHTTPSessionSelection(binding bindingPlan, present bool) (literalHTTPSessionMode, string, bool) {
	if !present {
		return httpclientSessionDefault, "", true
	}

	value, ok := literalBindingString(binding, present)
	if !ok {
		return httpclientSessionUnknown, "", false
	}
	if value == "" {
		return httpclientSessionNamed, "", true
	}
	if value == HTTPSessionNone {
		return httpclientSessionNone, "", true
	}

	return httpclientSessionNamed, value, true
}

func literalHTTPAuthSelection(binding bindingPlan, present bool) (literalHTTPAuthMode, string, bool) {
	if !present {
		return literalHTTPAuthDefault, "", true
	}

	value, ok := literalBindingString(binding, present)
	if !ok {
		return literalHTTPAuthUnknown, "", false
	}
	if value == "" {
		return literalHTTPAuthNamed, "", true
	}
	if value == HTTPAuthNone {
		return literalHTTPAuthNone, "", true
	}

	return literalHTTPAuthNamed, value, true
}

func headersBinding(bindings map[string]bindingPlan) bindingPlan {
	if bindings == nil {
		return bindingPlan{}
	}
	return bindings["headers"]
}

func literalURLBinding(bindings map[string]bindingPlan) string {
	value, _ := literalBindingString(bindings["url"], true)
	return value
}

func literalBindingString(binding bindingPlan, present bool) (string, bool) {
	if !present || binding.Kind != BindingKindLiteral {
		return "", false
	}

	value, ok := binding.Value.(string)
	return value, ok
}

func bindingObjectKeys(binding bindingPlan) map[string]struct{} {
	if binding.Kind == BindingKindObject {
		keys := make(map[string]struct{}, len(binding.Object))
		for key := range binding.Object {
			keys[key] = struct{}{}
		}
		return keys
	}

	if binding.Kind != BindingKindLiteral {
		return nil
	}

	value := reflect.ValueOf(binding.Value)
	if !value.IsValid() || value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
		return nil
	}

	keys := make(map[string]struct{}, value.Len())
	iter := value.MapRange()
	for iter.Next() {
		keys[iter.Key().String()] = struct{}{}
	}
	return keys
}

func bindingObjectString(binding bindingPlan, key string) (string, bool) {
	if binding.Kind == BindingKindObject {
		child, ok := binding.Object[key]
		if !ok {
			return "", false
		}
		return literalBindingString(child, true)
	}

	if binding.Kind != BindingKindLiteral {
		return "", false
	}

	value := reflect.ValueOf(binding.Value)
	if !value.IsValid() || value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
		return "", false
	}

	entry := value.MapIndex(reflect.ValueOf(key))
	if !entry.IsValid() {
		return "", false
	}
	if entry.Kind() == reflect.Interface && !entry.IsNil() {
		entry = entry.Elem()
	}
	if entry.Kind() != reflect.String {
		return "", false
	}

	return entry.String(), true
}

func hasCookieHeaderBinding(headers map[string]struct{}) bool {
	for key := range headers {
		if http.CanonicalHeaderKey(key) == "Cookie" {
			return true
		}
	}

	return false
}

func hasHTTPSession(httpSpec *HTTPSpec, name string) bool {
	if httpSpec == nil {
		return false
	}
	_, ok := httpSpec.Sessions[name]
	return ok
}

func hasHTTPAuth(httpSpec *HTTPSpec, name string) bool {
	if httpSpec == nil {
		return false
	}
	_, ok := httpSpec.Auth[name]
	return ok
}

func hasHTTPIdentity(httpSpec *HTTPSpec, name string) bool {
	if httpSpec == nil {
		return false
	}
	_, ok := httpSpec.Identities[name]
	return ok
}

func authUsesForm(auth HTTPAuthSpec) bool {
	for i := range auth.Attach {
		if auth.Attach[i].FormSlot != nil {
			return true
		}
	}

	return false
}

func authUsesSlot(auth HTTPAuthSpec, slot string) bool {
	for i := range auth.Attach {
		attachment := auth.Attach[i]
		switch {
		case attachment.HeaderSlot != nil && attachment.HeaderSlot.Slot == slot:
			return true
		case attachment.QuerySlot != nil && attachment.QuerySlot.Slot == slot:
			return true
		case attachment.FormSlot != nil && attachment.FormSlot.Slot == slot:
			return true
		}
	}

	return false
}

func httpAttachmentKindCount(attachment HTTPAuthAttachmentSpec) int {
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

func isFormContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return strings.EqualFold(mediaType, "application/x-www-form-urlencoded")
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return strings.EqualFold(mediaType, "application/json")
}

func httpSessionPath(stagePath, name string) string {
	return stagePath + "/http/" + runtimePathCodec{}.Join("session", name)
}

func httpAuthPath(stagePath, name string) string {
	return stagePath + "/http/" + runtimePathCodec{}.Join("auth", name)
}

func httpIdentityPath(stagePath, name string) string {
	return stagePath + "/http/" + runtimePathCodec{}.Join("identity", name)
}

func httpAttachPath(authPath string, index int) string {
	return fmt.Sprintf("%s/attach[%d]", authPath, index)
}

func httpCapturePath(actPath string) string {
	return actPath + "/capture_auth"
}

func httpCaptureSlotPath(capturePath, slot string) string {
	return joinChildPath(capturePath, "slot", slot)
}
