package decorator_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alex-poliushkin/theater"
	builtindecorator "github.com/alex-poliushkin/theater/builtin/decorator"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

const (
	jsonUnexpectedEOFFragment     = "unexpected EOF"
	csvDecoratorInputTypeFragment = "csv decorator input must be string or []byte"
	csvCommentConflictMessage     = "comment must differ from comma"
	csvDuplicateHeaderMessage     = `csv header "email" is duplicated`
)

type recordingDecoratorRegistrar struct {
	decorators map[string]theater.DecoratorDef
}

func (r *recordingDecoratorRegistrar) RegisterDecorator(ref string, decorator theater.DecoratorDef) error {
	if r.decorators == nil {
		r.decorators = make(map[string]theater.DecoratorDef)
	}

	r.decorators[ref] = decorator
	return nil
}

func TestRegisterAcceptsDecoratorRegistrarPort(t *testing.T) {
	t.Parallel()

	registrar := &recordingDecoratorRegistrar{}
	if err := builtindecorator.Register(registrar); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	if _, ok := registrar.decorators[builtindecorator.JSONRef]; !ok {
		t.Fatalf("expected %q to be registered", builtindecorator.JSONRef)
	}

	if _, ok := registrar.decorators[builtindecorator.CSVRef]; !ok {
		t.Fatalf("expected %q to be registered", builtindecorator.CSVRef)
	}
}

func TestRegisterRegistersDecorators(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	if _, err := catalog.ResolveDecorator(builtindecorator.JSONRef); err != nil {
		t.Fatalf("resolve json decorator failed: %v", err)
	}

	if _, err := catalog.ResolveDecorator(builtindecorator.CSVRef); err != nil {
		t.Fatalf("resolve csv decorator failed: %v", err)
	}
}

func TestJSONDecoratorDecodesJSONString(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.JSONRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	value, err := transform(`{"token":"issued-token","count":2}`)
	if err != nil {
		t.Fatalf("transform json failed: %v", err)
	}

	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("decoded value type mismatch: got %T", value)
	}

	if got, want := object["token"], "issued-token"; got != want {
		t.Fatalf("token mismatch: got %v want %v", got, want)
	}
}

func TestJSONDecoratorPreservesSecretInputSensitivity(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.JSONRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	value, err := transform(theater.NewSecret(`{"token":"issued-token"}`))
	if err != nil {
		t.Fatalf("transform json failed: %v", err)
	}

	assertSecretValue(t, value, map[string]any{"token": "issued-token"})
}

func TestJSONDecoratorRejectsInvalidJSONInput(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.JSONRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	cases := []struct {
		name  string
		value string
		check func(*testing.T, error)
	}{
		{
			name:  "invalid json",
			value: `{"token":`,
			check: func(t *testing.T, err error) {
				t.Helper()
				errtest.RequireContains(t, err, jsonUnexpectedEOFFragment)
			},
		},
		{
			name:  "trailing garbage",
			value: `{"token":"issued-token"} garbage`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError
				errtest.RequireAs(t, err, &syntaxErr)
				if syntaxErr.Offset == 0 {
					t.Fatal("expected syntax error offset to be populated")
				}
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := transform(tt.value)
			if err == nil {
				t.Fatal("expected JSON decode error, got nil")
			}

			tt.check(t, err)
		})
	}
}

func TestCSVDecoratorDecodesCSVIntoObjects(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.CSVRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(theater.Values{
		"comma":              ";",
		"trim_leading_space": true,
	})
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	value, err := transform("email;role\n alice@example.com;admin\nbob@example.com;user\n")
	if err != nil {
		t.Fatalf("transform csv failed: %v", err)
	}

	rows, ok := value.([]any)
	if !ok {
		t.Fatalf("decoded csv type mismatch: got %T", value)
	}

	if got, want := len(rows), 2; got != want {
		t.Fatalf("row count mismatch: got %d want %d", got, want)
	}

	firstRow, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("first row type mismatch: got %T", rows[0])
	}

	secondRow, ok := rows[1].(map[string]any)
	if !ok {
		t.Fatalf("second row type mismatch: got %T", rows[1])
	}

	if got, want := firstRow["email"], "alice@example.com"; got != want {
		t.Fatalf("first row email mismatch: got %v want %v", got, want)
	}

	if got, want := secondRow["role"], "user"; got != want {
		t.Fatalf("second row role mismatch: got %v want %v", got, want)
	}
}

func TestCSVDecoratorPreservesSecretInputSensitivity(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.CSVRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	value, err := transform(theater.NewSecret("email,role\nalice@example.com,admin\n"))
	if err != nil {
		t.Fatalf("transform csv failed: %v", err)
	}

	assertSecretValue(t, value, []any{
		map[string]any{
			"email": "alice@example.com",
			"role":  "admin",
		},
	})
}

func TestCSVDecoratorRejectsDuplicateHeaders(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.CSVRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	_, err = transform("email,email\nalice@example.com,admin\n")
	if err == nil {
		t.Fatal("expected duplicate header error, got nil")
	}

	errtest.RequireEqual(t, err, csvDuplicateHeaderMessage)
}

func TestCSVDecoratorRejectsUnsupportedInputType(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.CSVRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	transform, err := registeredDecorator.Compile(nil)
	if err != nil {
		t.Fatalf("compile decorator failed: %v", err)
	}

	_, err = transform(42)
	if err == nil {
		t.Fatal("expected csv decorator type error, got nil")
	}

	errtest.RequireContains(t, err, csvDecoratorInputTypeFragment)
}

func TestCSVDecoratorRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	catalog := theater.NewCatalog()
	if err := builtindecorator.Register(catalog); err != nil {
		t.Fatalf("register decorators failed: %v", err)
	}

	registeredDecorator, err := catalog.ResolveDecorator(builtindecorator.CSVRef)
	if err != nil {
		t.Fatalf("resolve decorator failed: %v", err)
	}

	_, err = registeredDecorator.Compile(theater.Values{
		"comma":   ";",
		"comment": ";",
	})
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}

	errtest.RequireEqual(t, err, csvCommentConflictMessage)
}

func assertSecretValue(t *testing.T, value any, want any) {
	t.Helper()

	secret, ok := value.(theater.Secret)
	if !ok {
		t.Fatalf("value type mismatch: got %T", value)
	}

	if got := fmt.Sprintf("%v", secret); strings.Contains(got, "issued-token") || strings.Contains(got, "alice@example.com") {
		t.Fatalf("formatted secret leaked raw value: %q", got)
	}

	encoded, err := json.Marshal(map[string]any{"value": secret})
	if err != nil {
		t.Fatalf("marshal secret failed: %v", err)
	}

	if strings.Contains(string(encoded), "issued-token") || strings.Contains(string(encoded), "alice@example.com") {
		t.Fatalf("json secret leaked raw value: %q", string(encoded))
	}

	if got := secret.Reveal(); fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", want) {
		t.Fatalf("revealed value mismatch: got %#v want %#v", got, want)
	}
}
