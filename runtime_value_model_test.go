package theater

import (
	"strings"
	"testing"
)

func TestValidateResolvedContractAcceptsStringMapContainers(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("headers", ValueContract{Kind: ValueKindObject}, map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		t.Fatalf("expected object contract to accept string map container, got %v", err)
	}
}

func TestValidateResolvedContractAcceptsStringSliceContainers(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("args", ValueContract{Kind: ValueKindList}, []string{"one", "two"})
	if err != nil {
		t.Fatalf("expected list contract to accept string slice container, got %v", err)
	}
}

func TestValidateResolvedContractAcceptsNestedNativeContainers(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("headers", ValueContract{Kind: ValueKindObject}, map[string]any{
		"Content-Type": []string{"application/json"},
	})
	if err != nil {
		t.Fatalf("expected nested native containers to normalize, got %v", err)
	}
}

func TestValidateResolvedContractRejectsStructObjectContainers(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("profile", ValueContract{Kind: ValueKindObject}, runtimeValueProfile{
		Token: "issued-token",
	})
	if err == nil {
		t.Fatal("expected struct container to stay outside runtime object contract")
	}

	if !strings.Contains(err.Error(), "expects object") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateResolvedContractRejectsMissingRequiredObjectField(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("profile", ValueContract{
		Kind: ValueKindObject,
		Fields: map[string]ValueContract{
			"token": {Kind: ValueKindString, Required: true},
		},
	}, map[string]any{})
	if err == nil {
		t.Fatal("expected missing required object field error")
	}

	if !strings.Contains(err.Error(), `profile field "token" is required`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateResolvedContractRejectsUndeclaredObjectField(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("profile", ValueContract{
		Kind: ValueKindObject,
		Fields: map[string]ValueContract{
			"token": {Kind: ValueKindString},
		},
	}, map[string]any{
		"token": "issued-token",
		"role":  "admin",
	})
	if err == nil {
		t.Fatal("expected undeclared object field error")
	}

	if !strings.Contains(err.Error(), `profile field "role" is not declared`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateResolvedContractSupportsObjectFieldsAndElemTogether(t *testing.T) {
	t.Parallel()

	err := validateResolvedContract("profile", ValueContract{
		Kind: ValueKindObject,
		Fields: map[string]ValueContract{
			"token": {Kind: ValueKindString, Required: true},
		},
		Elem: &ValueContract{Kind: ValueKindNumber},
	}, map[string]any{
		"token":   "issued-token",
		"attempt": 2,
	})
	if err != nil {
		t.Fatalf("expected object value to satisfy fields and elem, got %v", err)
	}
}

type runtimeValueProfile struct {
	Token string
}
