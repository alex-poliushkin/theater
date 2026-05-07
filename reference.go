package theater

import (
	"errors"
	"fmt"
	"strings"

	specmodel "github.com/alex-poliushkin/theater/spec"
)

// JSONPointer is an RFC 6901 pointer used for structural traversal after a
// ref or field root is selected.
type JSONPointer = specmodel.JSONPointer

// RefSpec names a root value and optional decode/path traversal.
type RefSpec = specmodel.RefSpec

// ThroughStepSpec declares one pure post-selection value transform.
type ThroughStepSpec = specmodel.ThroughStepSpec

// PickStepSpec selects exactly one list item by a compact at/equals predicate
// or by bounded relative where clauses.
type PickStepSpec = specmodel.PickStepSpec

// PickWhereClauseSpec describes one relative matcher clause inside a compound
// pick selector.
type PickWhereClauseSpec = specmodel.PickWhereClauseSpec

// RelativeSubjectSpec selects a single value relative to a collection item.
type RelativeSubjectSpec = specmodel.RelativeSubjectSpec

// RegexpStepSpec extracts a regexp match or capture group from text input.
type RegexpStepSpec = specmodel.RegexpStepSpec

// ParseJSONPointer validates raw and returns it as a JSONPointer.
func ParseJSONPointer(raw string) (JSONPointer, error) {
	return specmodel.ParseJSONPointer(raw)
}

func invalidRefNameDetail(name string) string {
	switch {
	case strings.HasPrefix(name, "#/"):
		return fmt.Sprintf("ref %q is invalid: use RFC 6901 path in ref.path, not URI fragment form", name)
	case strings.Contains(name, "."):
		root := strings.Split(name, ".")[0]
		remainder := strings.TrimPrefix(name, root)
		path := strings.ReplaceAll(remainder, ".", "/")
		return fmt.Sprintf(
			"ref %q is invalid: refs name values; use ref.name: %s and path: %s",
			name,
			root,
			path,
		)
	case strings.ContainsAny(name, "[]"):
		return fmt.Sprintf("ref %q is invalid: array traversal belongs in ref.path, not ref.name", name)
	case strings.Contains(name, "/"):
		return fmt.Sprintf("ref %q is invalid: traversal belongs in ref.path, not ref.name", name)
	default:
		return fmt.Sprintf("ref %q is invalid", name)
	}
}

func validateRefName(name string) error {
	if name == "" {
		return errors.New("ref name is required")
	}

	if strings.ContainsAny(name, ".[]/#") || strings.Contains(name, "/") {
		return fmt.Errorf("%s", invalidRefNameDetail(name))
	}

	return nil
}
