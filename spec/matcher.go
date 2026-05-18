package spec

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Supported YAML sugar forms for matcher lowering.
const (
	SugarFormNone       SugarForm = "none"
	SugarFormUnary      SugarForm = "unary"
	SugarFormFixedTuple SugarForm = "fixed_tuple"
)

// SugarForm identifies how YAML matcher sugar lowers into canonical assert
// args.
type SugarForm string

// Matcher checks one actual runtime value.
//
// Ordinary non-match results may be classified separately from matcher-internal
// failures so wrapper matchers can invert only logical mismatches.
type Matcher interface {
	Check(ctx context.Context, actual any) error
}

// MatcherCompileContext lets matchers recursively compile nested canonical
// matcher specs when they own structured matcher-local payloads.
type MatcherCompileContext interface {
	Compile(ref string, args Values) (Matcher, error)
	ResolveSugarKey(key string) (MatcherDescriptor, error)
}

// MatcherArg describes one named matcher argument.
type MatcherArg struct {
	Name     string
	Accepts  ValueContract
	Required bool
	Summary  string
}

// MatcherPreflightPolicy declares matcher-specific constraints for scenario
// preflight use. A nil policy means the matcher is not safe for preflight.
type MatcherPreflightPolicy struct {
	Args map[string]MatcherPreflightArgRule
}

// MatcherPreflightArgRule declares preflight-only validation for one static
// matcher argument.
type MatcherPreflightArgRule struct {
	ValidateLiteral func(value any) *MatcherPreflightArgIssue
}

// MatcherPreflightArgIssue describes one descriptor-owned preflight arg
// validation failure.
type MatcherPreflightArgIssue struct {
	Code    string
	Summary string
}

// SugarSpec describes the shorthand keys and positional arg mapping supported
// by a matcher.
type SugarSpec struct {
	Keys           []string
	Form           SugarForm
	PositionalArgs []string
}

// MatcherDescriptor describes a registered matcher contract and compile hook.
type MatcherDescriptor struct {
	Ref       string
	Summary   string
	Args      []MatcherArg
	Actual    ValueContract
	Preflight *MatcherPreflightPolicy
	Sugar     SugarSpec
	Compile   func(ctx MatcherCompileContext, args Values) (Matcher, error)
}

// MatcherCatalog stores matchers indexed by ref and YAML sugar key.
type MatcherCatalog struct {
	byRef      map[string]MatcherDescriptor
	bySugarKey map[string]MatcherDescriptor
}

// NewMatcherCatalog validates and indexes matcher descriptors.
func NewMatcherCatalog(descriptors ...MatcherDescriptor) (*MatcherCatalog, error) {
	catalog := &MatcherCatalog{
		byRef:      make(map[string]MatcherDescriptor, len(descriptors)),
		bySugarKey: make(map[string]MatcherDescriptor),
	}

	for i := range descriptors {
		descriptor := descriptors[i]
		if err := validateMatcherDescriptor(descriptor); err != nil {
			return nil, err
		}

		if _, ok := catalog.byRef[descriptor.Ref]; ok {
			return nil, fmt.Errorf("matcher %q is already registered", descriptor.Ref)
		}

		catalog.byRef[descriptor.Ref] = descriptor
		for _, key := range descriptor.Sugar.Keys {
			if _, ok := catalog.bySugarKey[key]; ok {
				return nil, fmt.Errorf("matcher sugar key %q is already registered", key)
			}

			catalog.bySugarKey[key] = descriptor
		}
	}

	return catalog, nil
}

// Resolve returns the matcher descriptor registered for ref.
func (c *MatcherCatalog) Resolve(ref string) (MatcherDescriptor, error) {
	descriptor, ok := c.byRef[ref]
	if !ok {
		return MatcherDescriptor{}, fmt.Errorf("matcher %q is not registered", ref)
	}

	return descriptor, nil
}

// ResolveSugarKey returns the matcher descriptor registered for a YAML sugar
// key.
func (c *MatcherCatalog) ResolveSugarKey(key string) (MatcherDescriptor, error) {
	descriptor, ok := c.bySugarKey[key]
	if !ok {
		return MatcherDescriptor{}, fmt.Errorf("matcher sugar %q is not registered", key)
	}

	return descriptor, nil
}

// Compile resolves and compiles one matcher using the catalog as the recursive
// compile context for nested matcher payloads.
func (c *MatcherCatalog) Compile(ref string, args Values) (Matcher, error) {
	return matcherCompileContext{resolver: c}.Compile(ref, args)
}

// Descriptors returns all registered matcher descriptors sorted by ref.
func (c *MatcherCatalog) Descriptors() []MatcherDescriptor {
	if c == nil {
		return nil
	}

	descriptors := make([]MatcherDescriptor, 0, len(c.byRef))
	for ref := range c.byRef {
		descriptors = append(descriptors, c.byRef[ref])
	}

	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].Ref < descriptors[j].Ref
	})

	return descriptors
}

func (f SugarForm) Valid() bool {
	switch f {
	case SugarFormNone, SugarFormUnary, SugarFormFixedTuple:
		return true
	default:
		return false
	}
}

func validateMatcherDescriptor(descriptor MatcherDescriptor) error {
	if descriptor.Ref == "" {
		return errors.New("matcher ref is required")
	}

	if descriptor.Compile == nil {
		return fmt.Errorf("matcher %q compile function is required", descriptor.Ref)
	}

	if err := validateMatcherArgs(descriptor.Ref, descriptor.Args); err != nil {
		return err
	}

	if err := validateMatcherActual(descriptor.Ref, descriptor.Actual); err != nil {
		return err
	}

	if err := validateSugarSpec(descriptor.Ref, descriptor.Args, descriptor.Sugar); err != nil {
		return err
	}

	if err := validateMatcherPreflightPolicy(descriptor.Ref, descriptor.Args, descriptor.Preflight); err != nil {
		return err
	}

	return nil
}

type matcherCompileContext struct {
	resolver interface {
		Resolve(ref string) (MatcherDescriptor, error)
		ResolveSugarKey(key string) (MatcherDescriptor, error)
	}
}

func (c matcherCompileContext) Compile(ref string, args Values) (Matcher, error) {
	descriptor, err := c.resolver.Resolve(ref)
	if err != nil {
		return nil, err
	}

	return descriptor.Compile(c, args)
}

func (c matcherCompileContext) ResolveSugarKey(key string) (MatcherDescriptor, error) {
	return c.resolver.ResolveSugarKey(key)
}

func validateMatcherArgs(ref string, args []MatcherArg) error {
	seen := make(map[string]struct{}, len(args))
	for i := range args {
		arg := args[i]
		if arg.Name == "" {
			return fmt.Errorf("matcher %q has an arg with empty name", ref)
		}

		if _, ok := seen[arg.Name]; ok {
			return fmt.Errorf("matcher %q arg %q is duplicated", ref, arg.Name)
		}

		if !arg.Accepts.Valid() {
			return fmt.Errorf("matcher %q arg %q must declare a valid accepts contract", ref, arg.Name)
		}

		seen[arg.Name] = struct{}{}
	}

	return nil
}

func validateMatcherActual(ref string, actual ValueContract) error {
	if actual.Kind == "" && len(actual.Kinds) == 0 {
		return nil
	}

	if !actual.Valid() {
		return fmt.Errorf("matcher %q must declare a valid actual contract", ref)
	}

	return nil
}

func validateSugarSpec(ref string, args []MatcherArg, sugar SugarSpec) error {
	if !sugar.Form.Valid() {
		return fmt.Errorf("matcher %q sugar form %q is invalid", ref, sugar.Form)
	}

	switch sugar.Form {
	case SugarFormNone:
		if len(sugar.Keys) != 0 || len(sugar.PositionalArgs) != 0 {
			return fmt.Errorf("matcher %q sugar none must not declare keys or positional args", ref)
		}
	case SugarFormUnary:
		if len(sugar.Keys) == 0 {
			return fmt.Errorf("matcher %q unary sugar must declare at least one key", ref)
		}

		if len(sugar.PositionalArgs) != 1 {
			return fmt.Errorf("matcher %q unary sugar must declare exactly one positional arg", ref)
		}
	case SugarFormFixedTuple:
		if len(sugar.Keys) == 0 {
			return fmt.Errorf("matcher %q fixed tuple sugar must declare at least one key", ref)
		}

		if len(sugar.PositionalArgs) == 0 {
			return fmt.Errorf("matcher %q fixed tuple sugar must declare positional args", ref)
		}
	}

	for _, key := range sugar.Keys {
		if key == "" {
			return fmt.Errorf("matcher %q sugar key is required", ref)
		}
	}

	seenKeys := make(map[string]struct{}, len(sugar.Keys))
	for _, key := range sugar.Keys {
		if _, ok := seenKeys[key]; ok {
			return fmt.Errorf("matcher %q sugar keys must be unique", ref)
		}

		seenKeys[key] = struct{}{}
	}

	availableArgs := make(map[string]struct{}, len(args))
	for i := range args {
		availableArgs[args[i].Name] = struct{}{}
	}

	for _, argName := range sugar.PositionalArgs {
		if _, ok := availableArgs[argName]; !ok {
			return fmt.Errorf("matcher %q sugar arg %q is not declared", ref, argName)
		}
	}

	return nil
}

func validateMatcherPreflightPolicy(ref string, args []MatcherArg, policy *MatcherPreflightPolicy) error {
	if policy == nil {
		return nil
	}

	argNames := make(map[string]struct{}, len(args))
	for i := range args {
		argNames[args[i].Name] = struct{}{}
	}

	for name := range policy.Args {
		if _, ok := argNames[name]; !ok {
			return fmt.Errorf("matcher %q preflight arg %q is not declared by matcher args", ref, name)
		}
	}

	return nil
}
