package theater

import specmodel "github.com/alex-poliushkin/theater/spec"

// Supported YAML sugar forms for matcher lowering.
const (
	SugarFormNone       SugarForm = specmodel.SugarFormNone
	SugarFormUnary      SugarForm = specmodel.SugarFormUnary
	SugarFormFixedTuple SugarForm = specmodel.SugarFormFixedTuple
)

// SugarForm identifies how YAML matcher sugar lowers into canonical assert
// args.
type SugarForm = specmodel.SugarForm

// Matcher checks one actual runtime value.
//
// Ordinary non-match results may be classified separately from matcher-internal
// failures so wrapper matchers can invert only logical mismatches.
type Matcher = specmodel.Matcher

// MatcherCompileContext lets matchers recursively compile nested canonical
// matcher specs when they own structured matcher-local payloads.
type MatcherCompileContext = specmodel.MatcherCompileContext

// MatcherArg describes one named matcher argument.
type MatcherArg = specmodel.MatcherArg

// SugarSpec describes the shorthand keys and positional arg mapping supported
// by a matcher.
type SugarSpec = specmodel.SugarSpec

// MatcherDescriptor describes a registered matcher contract and compile hook.
type MatcherDescriptor = specmodel.MatcherDescriptor

// MatcherCatalog stores matchers indexed by ref and YAML sugar key.
type MatcherCatalog = specmodel.MatcherCatalog

// NewMatcherCatalog validates and indexes matcher descriptors.
func NewMatcherCatalog(descriptors ...MatcherDescriptor) (*MatcherCatalog, error) {
	return specmodel.NewMatcherCatalog(descriptors...)
}
