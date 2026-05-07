package theater

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveSubjectDecodesJSONAndAppliesPointer(t *testing.T) {
	t.Parallel()

	actual, err := newReferenceResolver(Values{
		"body": `{"token":{"id":"abc123"}}`,
	}).ResolveSubject(subjectPlan{
		Field: "body",
		selectorPlan: selectorPlan{
			Decode: DecodeJSON,
			Path:   JSONPointer("/token/id"),
		},
	})
	if err != nil {
		t.Fatalf("resolve subject failed: %v", err)
	}

	if got, want := actual, "abc123"; got != want {
		t.Fatalf("subject value mismatch: got %v want %v", got, want)
	}
}

func TestResolveSubjectPreservesJSONNumbers(t *testing.T) {
	t.Parallel()

	actual, err := newReferenceResolver(Values{
		"body": `{"retry_after":2}`,
	}).ResolveSubject(subjectPlan{
		Field: "body",
		selectorPlan: selectorPlan{
			Decode: DecodeJSON,
		},
	})
	if err != nil {
		t.Fatalf("resolve subject failed: %v", err)
	}

	object, ok := actual.(map[string]any)
	if !ok {
		t.Fatalf("decoded value type mismatch: got %T", actual)
	}

	number, ok := object["retry_after"].(json.Number)
	if !ok {
		t.Fatalf("decoded number type mismatch: got %T", object["retry_after"])
	}

	if got, want := number.String(), "2"; got != want {
		t.Fatalf("decoded number mismatch: got %q want %q", got, want)
	}
}

func TestResolveSubjectPreservesSecretSelection(t *testing.T) {
	t.Parallel()

	actual, err := newReferenceResolver(Values{
		"payload": NewSecret(map[string]any{
			"token": map[string]any{"id": "abc123"},
		}),
	}).ResolveSubject(subjectPlan{
		Field: "payload",
		selectorPlan: selectorPlan{
			Path: JSONPointer("/token/id"),
		},
	})
	if err != nil {
		t.Fatalf("resolve subject failed: %v", err)
	}

	value, ok := actual.(Secret)
	if !ok {
		t.Fatalf("subject value type mismatch: got %T", actual)
	}

	if got, want := value.Reveal(), any("abc123"); got != want {
		t.Fatalf("subject value mismatch: got %#v want %#v", got, want)
	}
}

func TestResolveRefPreservesSecretSelection(t *testing.T) {
	t.Parallel()

	actual, err := newReferenceResolver(Values{
		"payload": NewSecret(map[string]any{
			"token": map[string]any{"id": "abc123"},
		}),
	}).ResolveRef(refPlan{
		Name: "payload",
		selectorPlan: selectorPlan{
			Path: JSONPointer("/token/id"),
		},
	})
	if err != nil {
		t.Fatalf("resolve ref failed: %v", err)
	}

	value, ok := actual.(Secret)
	if !ok {
		t.Fatalf("ref value type mismatch: got %T", actual)
	}

	if got, want := value.Reveal(), any("abc123"); got != want {
		t.Fatalf("ref value mismatch: got %#v want %#v", got, want)
	}
}

func TestResolveSubjectDecodeJSONPreservesSecretSelection(t *testing.T) {
	t.Parallel()

	actual, err := newReferenceResolver(Values{
		"body": NewSecret(`{"token":{"id":"abc123"}}`),
	}).ResolveSubject(subjectPlan{
		Field: "body",
		selectorPlan: selectorPlan{
			Decode: DecodeJSON,
			Path:   JSONPointer("/token/id"),
		},
	})
	if err != nil {
		t.Fatalf("resolve subject failed: %v", err)
	}

	value, ok := actual.(Secret)
	if !ok {
		t.Fatalf("subject value type mismatch: got %T", actual)
	}

	if got, want := value.Reveal(), any("abc123"); got != want {
		t.Fatalf("subject value mismatch: got %#v want %#v", got, want)
	}
}

func TestResolveSubjectSelectsCurrentActPropertyValues(t *testing.T) {
	t.Parallel()

	actual, err := newSubjectResolver(
		Values{"body": `{"token":{"id":"wrong"}}`},
		Values{"payload": map[string]any{"token": map[string]any{"id": "abc123"}}},
		Values{"body": `{"token":{"id":"wrong"}}`},
	).ResolveSubject(subjectPlan{
		From: SubjectFromProperty,
		Ref:  "payload",
		selectorPlan: selectorPlan{
			Path: JSONPointer("/token/id"),
		},
	})
	if err != nil {
		t.Fatalf("resolve property subject failed: %v", err)
	}

	if got, want := actual, "abc123"; got != want {
		t.Fatalf("property subject value mismatch: got %v want %v", got, want)
	}
}

func TestResolveSubjectRejectsInvalidJSONInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "invalid json",
			value: `{"token":`,
			want:  "unexpected EOF",
		},
		{
			name:  "trailing garbage",
			value: `{"ok":true} garbage`,
			want:  "invalid character",
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := newReferenceResolver(Values{
				"body": tt.value,
			}).ResolveSubject(subjectPlan{
				Field: "body",
				selectorPlan: selectorPlan{
					Decode: DecodeJSON,
				},
			})
			if err == nil {
				t.Fatal("expected JSON decode error, got nil")
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveSubjectAppliesThroughPipeline(t *testing.T) {
	t.Parallel()

	actual, err := newSubjectResolver(
		Values{
			"body":  `{"notifications":[{"receiverAddress":"demo@example.test","body":"Verification Code 654321"}]}`,
			"email": "demo@example.test",
		},
		nil,
		Values{"email": "demo@example.test"},
	).ResolveSubject(subjectPlan{
		Field: "body",
		selectorPlan: selectorPlan{
			Decode: DecodeJSON,
			Path:   JSONPointer("/notifications"),
			Through: []throughStepPlan{
				{
					Pick: &pickStepPlan{
						At: JSONPointer("/receiverAddress"),
						Equals: bindingPlan{
							Kind: BindingKindRef,
							Ref:  &refPlan{Name: "email"},
						},
					},
				},
				{Path: JSONPointer("/body")},
				{Regexp: &regexpStepPlan{Pattern: `\b(\d{6})\b`, Group: 1}},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve subject failed: %v", err)
	}

	if got, want := actual, "654321"; got != want {
		t.Fatalf("subject value mismatch: got %v want %v", got, want)
	}
}

func TestParseJSONPointerRejectsUnsupportedForms(t *testing.T) {
	t.Parallel()

	cases := []string{"#/token/id", "/", "/items/-/id"}
	for _, raw := range cases {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseJSONPointer(raw); err == nil {
				t.Fatalf("expected pointer %q to be rejected", raw)
			}
		})
	}
}
