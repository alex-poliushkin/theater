package theater

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	selectorContractCodeDecode    = "decode"
	selectorContractCodePath      = "path"
	selectorContractCodeRegexp    = "regexp"
	selectorContractCodeTransform = "transform"
)

type selectorContractError struct {
	code    string
	summary string
	cause   error
}

func (e selectorContractError) Error() string {
	return e.summary
}

func (e selectorContractError) Unwrap() error {
	return e.cause
}

func validateSelectorContract(selector selectorPlan, contract ValueContract, decorators DecoratorResolver) error {
	_, _, err := selectedSelectorContract(selector, contract, decorators)
	return err
}

func selectedSelectorContract(
	selector selectorPlan,
	contract ValueContract,
	decorators DecoratorResolver,
) (selected ValueContract, known bool, err error) {
	if selector.Decode == DecodeJSON {
		if !contractSupportsOnly(contract, ValueKindString, ValueKindBytes) {
			return ValueContract{}, false, selectorContractError{
				code:    selectorContractCodeDecode,
				summary: fmt.Sprintf("decode %q requires string or bytes, got %s", selector.Decode, contractKindString(contract)),
			}
		}

		// Decoded JSON shape is author-defined rather than declared by the source contract.
		return ValueContract{}, false, nil
	}

	selected, known, err = selectedPathContract(selector.Path, contract)
	if err != nil {
		return ValueContract{}, false, err
	}
	if !known {
		return ValueContract{}, false, nil
	}

	current := selected
	for i := range selector.Through {
		current, known, err = selectedThroughContract(selector.Through[i], current, decorators)
		if err != nil {
			return ValueContract{}, false, err
		}
		if !known {
			return ValueContract{}, false, nil
		}
	}

	return current.Clone(), true, nil
}

func selectedPathContract(path JSONPointer, contract ValueContract) (selected ValueContract, known bool, err error) {
	if path.IsRoot() {
		return contract.Clone(), true, nil
	}

	current := contract
	tokens, err := pointerTokens(path)
	if err != nil {
		return ValueContract{}, false, err
	}

	for _, token := range tokens {
		next, nextKnown, nextErr := nextSelectorContract(current, path, token)
		if nextErr != nil {
			return ValueContract{}, false, selectorContractError{
				code:    selectorContractCodePath,
				summary: nextErr.Error(),
				cause:   nextErr,
			}
		}
		if !nextKnown {
			return ValueContract{}, false, nil
		}

		current = next
	}

	return current.Clone(), true, nil
}

func contractSupportsOnly(contract ValueContract, allowed ...ValueKind) bool {
	allowedKinds := make(map[ValueKind]struct{}, len(allowed))
	for _, kind := range allowed {
		allowedKinds[kind] = struct{}{}
	}

	kinds := contract.KindsSet()
	if len(kinds) == 0 {
		return false
	}

	for kind := range kinds {
		if _, ok := allowedKinds[kind]; !ok {
			return false
		}
	}

	return true
}

func nextSelectorContract(
	current ValueContract,
	pointer JSONPointer,
	token string,
) (next ValueContract, known bool, err error) {
	kinds := current.KindsSet()
	switch {
	case len(kinds) == 1 && kinds.Contains(ValueKindObject):
		next, ok := objectMemberContract(current, token)
		if !ok {
			return ValueContract{}, false, fmt.Errorf(
				"path %q member %q is not declared by contract %s",
				pointer,
				token,
				contractKindString(current),
			)
		}

		return next, true, nil
	case len(kinds) == 1 && kinds.Contains(ValueKindList):
		if err := validateSelectorIndexToken(token); err != nil {
			return ValueContract{}, false, fmt.Errorf("path %q %w", pointer, err)
		}

		if current.Elem == nil {
			return ValueContract{Kind: ValueKindAny}, true, nil
		}

		return *current.Elem, true, nil
	case contractMaySupportTraversal(current):
		return ValueContract{}, false, nil
	default:
		return ValueContract{}, false, fmt.Errorf("path %q is incompatible with contract %s", pointer, contractKindString(current))
	}
}

func contractMaySupportTraversal(contract ValueContract) bool {
	kinds := contract.KindsSet()
	return kinds.Contains(ValueKindAny) || kinds.Contains(ValueKindObject) || kinds.Contains(ValueKindList)
}

func validateSelectorIndexToken(token string) error {
	if token == "-" {
		return errors.New(`token "-" is not supported`)
	}

	if len(token) > 1 && token[0] == '0' {
		return fmt.Errorf("token %q must not contain leading zeroes", token)
	}

	index, err := strconv.Atoi(token)
	if err != nil {
		return newCausalError(fmt.Sprintf("token %q must be array index", token), err)
	}
	if index < 0 {
		return fmt.Errorf("token %q must be array index", token)
	}

	return nil
}

func selectedThroughContract(
	step throughStepPlan,
	contract ValueContract,
	decorators DecoratorResolver,
) (selected ValueContract, known bool, err error) {
	switch {
	case !step.Path.IsRoot():
		return selectedSelectorContract(selectorPlan{Path: step.Path}, contract, decorators)
	case step.Pick != nil:
		return selectedPickContract(*step.Pick, contract)
	case step.Regexp != nil:
		if !contractSupportsOnly(contract, ValueKindString, ValueKindBytes) {
			return ValueContract{}, false, selectorContractError{
				code:    selectorContractCodeRegexp,
				summary: "regexp requires string or bytes, got " + contractKindString(contract),
			}
		}

		return ValueContract{Kind: ValueKindString}, true, nil
	case step.Transform != nil:
		return selectedDecoratorContract(*step.Transform, contract, decorators)
	default:
		return ValueContract{}, false, selectorContractError{
			code:    selectorContractCodePath,
			summary: "through step is invalid",
		}
	}
}

func selectedDecoratorContract(
	step decoratorPlan,
	contract ValueContract,
	decorators DecoratorResolver,
) (selected ValueContract, known bool, err error) {
	def, ok, err := resolveSelectorDecorator(step, decorators)
	if !ok || err != nil {
		return ValueContract{}, false, err
	}

	if err := contractCompatibilityError(contract, def.Contract.Accepts); err != nil {
		return ValueContract{}, false, selectorContractError{
			code:    selectorContractCodeTransform,
			summary: fmt.Sprintf("transform %q input is incompatible with contract %s: %v", step.Use, contractKindString(def.Contract.Accepts), err),
			cause:   err,
		}
	}

	return def.Contract.Produces.Clone(), true, nil
}

func resolveSelectorDecorator(
	step decoratorPlan,
	decorators DecoratorResolver,
) (def DecoratorDef, ok bool, err error) {
	if dependencyMissing(decorators) {
		return DecoratorDef{}, false, nil
	}

	def, err = decorators.ResolveDecorator(step.Use)
	if err != nil {
		return DecoratorDef{}, false, selectorContractError{
			code:    selectorContractCodeTransform,
			summary: err.Error(),
			cause:   err,
		}
	}

	if err := validateDecoratorContract(def.Contract); err != nil {
		return DecoratorDef{}, false, selectorContractError{
			code:    selectorContractCodeTransform,
			summary: fmt.Sprintf("transform %q contract is invalid: %v", step.Use, err),
			cause:   err,
		}
	}

	if _, err := resolveDecoratorArgs(step.With, def.Contract.Params); err != nil {
		return DecoratorDef{}, false, selectorContractError{
			code:    selectorContractCodeTransform,
			summary: fmt.Sprintf("transform %q config is invalid: %v", step.Use, err),
			cause:   err,
		}
	}

	return def, true, nil
}

func selectedPickContract(_ pickStepPlan, contract ValueContract) (selected ValueContract, known bool, err error) {
	kinds := contract.KindsSet()
	switch {
	case len(kinds) == 1 && kinds.Contains(ValueKindList):
		if contract.Elem == nil {
			return ValueContract{Kind: ValueKindAny}, true, nil
		}

		return *contract.Elem, true, nil
	case contractMaySupportTraversal(contract):
		return ValueContract{}, false, nil
	default:
		return ValueContract{}, false, selectorContractError{
			code:    selectorContractCodePath,
			summary: "pick is incompatible with contract " + contractKindString(contract),
		}
	}
}
