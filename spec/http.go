package spec

// Supported HTTP API key attachment locations.
const (
	HTTPSessionNone                 = "none"
	HTTPAuthNone                    = "none"
	HTTPAPIKeyInHeader HTTPAPIKeyIn = "header"
	HTTPAPIKeyInQuery  HTTPAPIKeyIn = "query"
)

// HTTPAPIKeyIn identifies where an API key attachment is applied.
type HTTPAPIKeyIn string

// HTTPSpec declares shared stage-level HTTP sessions, auth configs, and
// request identities.
type HTTPSpec struct {
	Sessions   map[string]HTTPSessionSpec  `yaml:"sessions,omitempty" json:"sessions,omitempty"`
	Auth       map[string]HTTPAuthSpec     `yaml:"auth,omitempty" json:"auth,omitempty"`
	Identities map[string]HTTPIdentitySpec `yaml:"identities,omitempty" json:"identities,omitempty"`
}

// HTTPSessionSpec reserves a named managed cookie session.
type HTTPSessionSpec struct{}

// HTTPAuthSpec declares one reusable HTTP auth attachment set.
type HTTPAuthSpec struct {
	Attach []HTTPAuthAttachmentSpec `yaml:"attach,omitempty" json:"attach,omitempty"`
}

// HTTPAuthBindingSpec initializes declared auth slots for one scenario
// execution from scenario-start bindings.
type HTTPAuthBindingSpec struct {
	Slots map[string]BindingSpec `yaml:"slots,omitempty" json:"slots,omitempty"`
}

// HTTPIdentitySpec bundles one optional session ref and one optional auth ref
// for request ergonomics.
type HTTPIdentitySpec struct {
	Session string `yaml:"session,omitempty" json:"session,omitempty"`
	Auth    string `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// HTTPAuthAttachmentSpec declares one typed request auth attachment.
type HTTPAuthAttachmentSpec struct {
	Bearer     *HTTPBearerAuthSpec     `yaml:"bearer,omitempty" json:"bearer,omitempty"`
	Basic      *HTTPBasicAuthSpec      `yaml:"basic,omitempty" json:"basic,omitempty"`
	APIKey     *HTTPAPIKeyAuthSpec     `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	HeaderSlot *HTTPHeaderSlotAuthSpec `yaml:"header_slot,omitempty" json:"header_slot,omitempty"`
	QuerySlot  *HTTPQuerySlotAuthSpec  `yaml:"query_slot,omitempty" json:"query_slot,omitempty"`
	FormSlot   *HTTPFormSlotAuthSpec   `yaml:"form_slot,omitempty" json:"form_slot,omitempty"`
}

// HTTPBearerAuthSpec attaches a bearer token to Authorization. Exactly one of
// Token or TokenSlot must be set.
type HTTPBearerAuthSpec struct {
	Token     string `yaml:"token,omitempty" json:"token,omitempty"`
	TokenSlot string `yaml:"token_slot,omitempty" json:"token_slot,omitempty"`
}

// HTTPBasicAuthSpec attaches HTTP Basic credentials.
type HTTPBasicAuthSpec struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// HTTPAPIKeyAuthSpec attaches an API key to a header or query parameter.
type HTTPAPIKeyAuthSpec struct {
	In    HTTPAPIKeyIn `yaml:"in" json:"in"`
	Name  string       `yaml:"name" json:"name"`
	Value string       `yaml:"value" json:"value"`
}

// HTTPHeaderSlotAuthSpec attaches one captured auth slot to a request header.
type HTTPHeaderSlotAuthSpec struct {
	Name string `yaml:"name" json:"name"`
	Slot string `yaml:"slot" json:"slot"`
}

// HTTPQuerySlotAuthSpec attaches one captured auth slot to a query parameter.
type HTTPQuerySlotAuthSpec struct {
	Name string `yaml:"name" json:"name"`
	Slot string `yaml:"slot" json:"slot"`
}

// HTTPFormSlotAuthSpec attaches one captured auth slot to a form field.
type HTTPFormSlotAuthSpec struct {
	Name string `yaml:"name" json:"name"`
	Slot string `yaml:"slot" json:"slot"`
}

// HTTPAuthCaptureSpec captures response material into named auth slots after
// a successful HTTP action.
type HTTPAuthCaptureSpec struct {
	Auth  string                           `yaml:"auth" json:"auth"`
	Slots map[string]HTTPCaptureSourceSpec `yaml:"slots,omitempty" json:"slots,omitempty"`
}

// HTTPCaptureSourceSpec selects one response source for a captured auth slot.
type HTTPCaptureSourceSpec struct {
	ResponseHeader string      `yaml:"response_header,omitempty" json:"response_header,omitempty"`
	ResponseCookie string      `yaml:"response_cookie,omitempty" json:"response_cookie,omitempty"`
	JSONPointer    JSONPointer `yaml:"json_pointer,omitempty" json:"json_pointer,omitempty"`
	FormField      string      `yaml:"form_field,omitempty" json:"form_field,omitempty"`
}

func (k HTTPAPIKeyIn) Valid() bool {
	switch k {
	case HTTPAPIKeyInHeader, HTTPAPIKeyInQuery:
		return true
	default:
		return false
	}
}

// Clone returns a deep copy of the HTTP spec.
func (s *HTTPSpec) Clone() *HTTPSpec {
	if s == nil {
		return nil
	}

	cloned := &HTTPSpec{
		Sessions:   make(map[string]HTTPSessionSpec, len(s.Sessions)),
		Auth:       make(map[string]HTTPAuthSpec, len(s.Auth)),
		Identities: make(map[string]HTTPIdentitySpec, len(s.Identities)),
	}

	for name, session := range s.Sessions {
		cloned.Sessions[name] = session
	}

	for name, auth := range s.Auth {
		cloned.Auth[name] = auth.Clone()
	}
	for name, identity := range s.Identities {
		cloned.Identities[name] = identity
	}

	if len(cloned.Sessions) == 0 {
		cloned.Sessions = nil
	}
	if len(cloned.Auth) == 0 {
		cloned.Auth = nil
	}
	if len(cloned.Identities) == 0 {
		cloned.Identities = nil
	}

	return cloned
}

// Clone returns a deep copy of the auth spec.
func (s HTTPAuthSpec) Clone() HTTPAuthSpec {
	cloned := HTTPAuthSpec{
		Attach: make([]HTTPAuthAttachmentSpec, len(s.Attach)),
	}

	for i := range s.Attach {
		cloned.Attach[i] = s.Attach[i].Clone()
	}
	return cloned
}

// Clone returns a deep copy of the auth binding spec.
func (s HTTPAuthBindingSpec) Clone() HTTPAuthBindingSpec {
	cloned := HTTPAuthBindingSpec{
		Slots: make(map[string]BindingSpec, len(s.Slots)),
	}
	for name := range s.Slots {
		cloned.Slots[name] = s.Slots[name].Clone()
	}
	if len(cloned.Slots) == 0 {
		cloned.Slots = nil
	}

	return cloned
}

// Clone returns a deep copy of the attachment spec.
func (s HTTPAuthAttachmentSpec) Clone() HTTPAuthAttachmentSpec {
	cloned := s
	if s.Bearer != nil {
		bearer := *s.Bearer
		cloned.Bearer = &bearer
	}
	if s.Basic != nil {
		basic := *s.Basic
		cloned.Basic = &basic
	}
	if s.APIKey != nil {
		apiKey := *s.APIKey
		cloned.APIKey = &apiKey
	}
	if s.HeaderSlot != nil {
		slot := *s.HeaderSlot
		cloned.HeaderSlot = &slot
	}
	if s.QuerySlot != nil {
		slot := *s.QuerySlot
		cloned.QuerySlot = &slot
	}
	if s.FormSlot != nil {
		slot := *s.FormSlot
		cloned.FormSlot = &slot
	}
	return cloned
}

// Clone returns a deep copy of the capture spec.
func (s *HTTPAuthCaptureSpec) Clone() *HTTPAuthCaptureSpec {
	if s == nil {
		return nil
	}

	cloned := &HTTPAuthCaptureSpec{
		Auth:  s.Auth,
		Slots: make(map[string]HTTPCaptureSourceSpec, len(s.Slots)),
	}
	for name, source := range s.Slots {
		cloned.Slots[name] = source
	}
	if len(cloned.Slots) == 0 {
		cloned.Slots = nil
	}

	return cloned
}
