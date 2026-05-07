package spec

import (
	"errors"
	"fmt"
	"strings"
)

// JSONPointer is an RFC 6901 pointer used for structural traversal after a
// ref or field root is selected.
type JSONPointer string

// RefSpec names a root value and optional decode/path traversal.
type RefSpec struct {
	Name    string
	Decode  DecodeKind
	Path    JSONPointer
	Through []ThroughStepSpec
}

// ThroughStepSpec declares one pure post-selection value transform.
type ThroughStepSpec struct {
	Path   JSONPointer     `yaml:"path,omitempty" json:"path,omitempty"`
	Pick   *PickStepSpec   `yaml:"pick,omitempty" json:"pick,omitempty"`
	Regexp *RegexpStepSpec `yaml:"regexp,omitempty" json:"regexp,omitempty"`
}

// PickStepSpec selects exactly one list item. The compact at/equals form
// matches one relative value against one expected binding; the where form
// matches all bounded relative clauses against each item.
type PickStepSpec struct {
	At     JSONPointer           `yaml:"at,omitempty" json:"at,omitempty"`
	Equals BindingSpec           `yaml:"equals,omitempty" json:"equals,omitempty"`
	Where  []PickWhereClauseSpec `yaml:"where,omitempty" json:"where,omitempty"`
}

// PickWhereClauseSpec describes one bounded relative matcher clause inside a
// compound pick selector.
type PickWhereClauseSpec struct {
	Subject RelativeSubjectSpec `yaml:"subject,omitempty" json:"subject,omitempty"`
	Assert  AssertSpec          `yaml:"assert" json:"assert"`
}

// RelativeSubjectSpec selects a single value relative to a collection item.
type RelativeSubjectSpec struct {
	Decode DecodeKind  `yaml:"decode,omitempty" json:"decode,omitempty"`
	Path   JSONPointer `yaml:"path,omitempty" json:"path,omitempty"`
}

// RegexpStepSpec extracts one regexp match or capture group from text input.
type RegexpStepSpec struct {
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Group   int    `yaml:"group,omitempty" json:"group,omitempty"`
}

// ParseJSONPointer validates raw and returns it as a JSONPointer.
func ParseJSONPointer(raw string) (JSONPointer, error) {
	pointer := JSONPointer(raw)
	if err := pointer.Validate(); err != nil {
		return "", err
	}

	return pointer, nil
}

func (p *JSONPointer) IsRoot() bool {
	if p == nil {
		return true
	}

	return *p == ""
}

func (p *JSONPointer) IsZero() bool {
	return p.IsRoot()
}

func (p *JSONPointer) String() string {
	if p == nil {
		return ""
	}

	return string(*p)
}

func (p *JSONPointer) Validate() error {
	if p == nil {
		return nil
	}

	return validateJSONPointer(*p)
}

func validateJSONPointer(pointer JSONPointer) error {
	if pointer.IsRoot() {
		return nil
	}

	raw := pointer.String()
	switch {
	case strings.HasPrefix(raw, "#/"):
		return errors.New(`use RFC 6901 string form "/token/id", not URI fragment form "#/token/id"`)
	case raw == "/":
		return errors.New(`"/" selects the member with key ""; omit path for the whole value`)
	case !strings.HasPrefix(raw, "/"):
		return errors.New("path must start with /")
	}

	_, err := pointerTokens(pointer)
	return err
}

func decodePointerToken(token string) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] < 0x20 {
			return "", errors.New("path contains control characters")
		}

		if token[i] != '~' {
			builder.WriteByte(token[i])
			continue
		}

		if i+1 >= len(token) {
			return "", errors.New("path escape is truncated")
		}

		switch token[i+1] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", fmt.Errorf("path escape ~%c is invalid", token[i+1])
		}

		i++
	}

	return builder.String(), nil
}

func pointerTokens(pointer JSONPointer) ([]string, error) {
	if pointer.IsRoot() {
		return nil, nil
	}

	segments := strings.Split(pointer.String()[1:], "/")
	tokens := make([]string, 0, len(segments))
	for _, segment := range segments {
		token, err := decodePointerToken(segment)
		if err != nil {
			return nil, err
		}

		if token == "-" {
			return nil, errors.New(`path token "-" is not supported`)
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}
