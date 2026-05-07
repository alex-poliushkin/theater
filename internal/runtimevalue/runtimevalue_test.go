package runtimevalue

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alex-poliushkin/theater/internal/secretvalue"
	"github.com/alex-poliushkin/theater/internal/testkit/errtest"
)

const (
	runtimevalueUnexpectedEOFFragment = "unexpected EOF"
)

func TestWrapKindDetectsCanonicalRuntimeShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  Kind
	}{
		{name: "null", value: nil, want: KindNull},
		{name: "bytes", value: []byte("payload"), want: KindBytes},
		{name: "string", value: "payload", want: KindString},
		{name: "secret string", value: secretvalue.New("payload"), want: KindString},
		{name: "number", value: json.Number("2"), want: KindNumber},
		{name: "bool", value: true, want: KindBool},
		{name: "object", value: map[string]any{"token": "issued-token"}, want: KindObject},
		{name: "string map", value: map[string]string{"token": "issued-token"}, want: KindObject},
		{name: "secret object", value: secretvalue.New(map[string]any{"token": "issued-token"}), want: KindObject},
		{name: "list", value: []any{"one"}, want: KindList},
		{name: "string slice", value: []string{"one"}, want: KindList},
		{name: "unknown", value: struct{ Token string }{Token: "issued-token"}, want: KindUnknown},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Wrap(tt.value).Kind(); got != tt.want {
				t.Fatalf("kind mismatch: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestWrapAccessorsKeepRuntimeSemantics(t *testing.T) {
	t.Parallel()

	t.Run("scalar accessors", func(t *testing.T) {
		t.Parallel()

		if got, err := Wrap("issued-token").String("token"); err != nil || got != "issued-token" {
			t.Fatalf("string accessor mismatch: got %q err %v", got, err)
		}

		if got, err := Wrap(true).Bool("enabled"); err != nil || !got {
			t.Fatalf("bool accessor mismatch: got %v err %v", got, err)
		}

		if got, err := Wrap(json.Number("2.5")).Float64("count"); err != nil || got != 2.5 {
			t.Fatalf("float accessor mismatch: got %v err %v", got, err)
		}

		if got, err := Wrap(json.Number("3")).Int("retries"); err != nil || got != 3 {
			t.Fatalf("int accessor mismatch: got %v err %v", got, err)
		}
	})

	t.Run("wrapped secret accessors", func(t *testing.T) {
		t.Parallel()

		if got, err := Wrap(secretvalue.New("issued-token")).String("token"); err != nil || got != "issued-token" {
			t.Fatalf("wrapped string accessor mismatch: got %q err %v", got, err)
		}

		object, err := Wrap(secretvalue.New(map[string]any{"token": "issued-token"})).Object("payload")
		if err != nil {
			t.Fatalf("wrapped object accessor failed: %v", err)
		}

		if got, want := object["token"], "issued-token"; got != want {
			t.Fatalf("wrapped object value mismatch: got %v want %v", got, want)
		}
	})

	t.Run("composite accessors", func(t *testing.T) {
		t.Parallel()

		object, err := Wrap(map[string]any{"token": "issued-token"}).Object("payload")
		if err != nil {
			t.Fatalf("object accessor failed: %v", err)
		}

		if got, want := object["token"], "issued-token"; got != want {
			t.Fatalf("object value mismatch: got %v want %v", got, want)
		}

		list, err := Wrap([]any{"one", "two"}).List("items")
		if err != nil {
			t.Fatalf("list accessor failed: %v", err)
		}

		if got, want := list, []any{"one", "two"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("list accessor mismatch: got %#v want %#v", got, want)
		}
	})

	t.Run("native container normalization", func(t *testing.T) {
		t.Parallel()

		object, err := Wrap(map[string]string{"token": "issued-token"}).Object("payload")
		if err != nil {
			t.Fatalf("native object accessor failed: %v", err)
		}

		if got, want := object["token"], "issued-token"; got != want {
			t.Fatalf("native object value mismatch: got %v want %v", got, want)
		}

		list, err := Wrap([]string{"one", "two"}).List("items")
		if err != nil {
			t.Fatalf("native list accessor failed: %v", err)
		}

		if got, want := list, []any{"one", "two"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("native list accessor mismatch: got %#v want %#v", got, want)
		}
	})

	t.Run("bytes clone", func(t *testing.T) {
		t.Parallel()

		original := []byte("payload")
		cloned, err := Wrap(original).Bytes("body")
		if err != nil {
			t.Fatalf("bytes accessor failed: %v", err)
		}

		original[0] = 'P'
		if got, want := string(cloned), "payload"; got != want {
			t.Fatalf("bytes accessor mismatch: got %q want %q", got, want)
		}
	})
}

func TestWrapDecodeJSONKeepsStrictSingleValuePolicy(t *testing.T) {
	t.Parallel()

	decoded, err := Wrap(`{"retry_after":2}`).DecodeJSON("body")
	if err != nil {
		t.Fatalf("decode JSON failed: %v", err)
	}

	object, ok := decoded.Raw().(map[string]any)
	if !ok {
		t.Fatalf("decoded value type mismatch: got %T", decoded.Raw())
	}

	number, ok := object["retry_after"].(json.Number)
	if !ok {
		t.Fatalf("decoded number type mismatch: got %T", object["retry_after"])
	}

	if got, want := number.String(), "2"; got != want {
		t.Fatalf("decoded number mismatch: got %q want %q", got, want)
	}

	if _, err := Wrap(`{"retry_after":2} garbage`).DecodeJSON("body"); err == nil {
		t.Fatal("expected trailing content error, got nil")
	}
}

func TestWrapDecodeJSONPreservesSecretInputs(t *testing.T) {
	t.Parallel()

	decoded, err := Wrap(secretvalue.New(`{"token":"issued-token"}`)).DecodeJSON("body")
	if err != nil {
		t.Fatalf("decode JSON failed: %v", err)
	}

	revealed, ok := secretvalue.Reveal(decoded.Raw())
	if !ok {
		t.Fatalf("decoded value type mismatch: got %T", decoded.Raw())
	}

	object, ok := revealed.(map[string]any)
	if !ok {
		t.Fatalf("revealed decoded value type mismatch: got %T", revealed)
	}

	if got, want := object["token"], "issued-token"; got != want {
		t.Fatalf("decoded token mismatch: got %v want %v", got, want)
	}
}

func TestWrapAccessorsReportFieldSpecificErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "string",
			run: func() error {
				_, err := Wrap(2).String("token")
				return err
			},
			want: "token must be string",
		},
		{
			name: "bool",
			run: func() error {
				_, err := Wrap("true").Bool("enabled")
				return err
			},
			want: "enabled must be bool",
		},
		{
			name: "bytes",
			run: func() error {
				_, err := Wrap(2).Bytes("body")
				return err
			},
			want: "body must be string or []byte",
		},
		{
			name: "object",
			run: func() error {
				_, err := Wrap([]any{"one"}).Object("payload")
				return err
			},
			want: "payload must be object",
		},
		{
			name: "list",
			run: func() error {
				_, err := Wrap(map[string]any{"token": "issued-token"}).List("items")
				return err
			},
			want: "items must be list",
		},
		{
			name: "float",
			run: func() error {
				_, err := Wrap("two").Float64("count")
				return err
			},
			want: "count must be numeric",
		},
		{
			name: "int",
			run: func() error {
				_, err := Wrap(2.5).Int("retries")
				return err
			},
			want: "retries must be integer",
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.run()
			if err == nil {
				t.Fatal("expected accessor error, got nil")
			}

			errtest.RequireContains(t, err, tt.want)
		})
	}
}

func TestWrapCloneDeepCopiesCanonicalContainers(t *testing.T) {
	t.Parallel()

	value := Wrap(map[string]any{
		"token": "issued-token",
		"items": []any{
			map[string]any{"id": "first"},
		},
	})

	cloned, ok := value.Clone().(map[string]any)
	if !ok {
		t.Fatalf("clone type mismatch: got %T", value.Clone())
	}

	cloned["token"] = "changed"
	cloned["items"].([]any)[0].(map[string]any)["id"] = "mutated"

	original := value.Raw().(map[string]any)
	if got, want := original["token"], "issued-token"; got != want {
		t.Fatalf("token clone mismatch: got %v want %v", got, want)
	}

	if got, want := original["items"].([]any)[0].(map[string]any)["id"], "first"; got != want {
		t.Fatalf("nested clone mismatch: got %v want %v", got, want)
	}
}

func TestCloneDeepCopiesCanonicalContainers(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"token": "issued-token",
		"items": []any{
			map[string]any{"id": "first"},
		},
	}

	cloned, ok := Clone(original).(map[string]any)
	if !ok {
		t.Fatalf("clone type mismatch: got %T", Clone(original))
	}

	cloned["token"] = "changed"
	items := cloned["items"].([]any)
	items[0].(map[string]any)["id"] = "mutated"

	if got, want := original["token"], "issued-token"; got != want {
		t.Fatalf("token clone mismatch: got %v want %v", got, want)
	}

	originalItems := original["items"].([]any)
	if got, want := originalItems[0].(map[string]any)["id"], "first"; got != want {
		t.Fatalf("nested clone mismatch: got %v want %v", got, want)
	}
}

func TestCloneNormalizesCommonNativeContainers(t *testing.T) {
	t.Parallel()

	original := map[string][]string{
		"token": {"issued-token"},
	}

	cloned, ok := Clone(original).(map[string]any)
	if !ok {
		t.Fatalf("clone type mismatch: got %T", Clone(original))
	}

	values, ok := cloned["token"].([]any)
	if !ok {
		t.Fatalf("nested clone type mismatch: got %T", cloned["token"])
	}

	if got, want := values, []any{"issued-token"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized clone mismatch: got %#v want %#v", got, want)
	}
}

func TestClonePreservesSecretWrappers(t *testing.T) {
	t.Parallel()

	original := secretvalue.New(map[string]any{
		"token": secretvalue.New("issued-token"),
		"items": []any{
			map[string]any{"id": "first"},
		},
	})

	cloned, ok := Clone(original).(secretvalue.Value)
	if !ok {
		t.Fatalf("clone type mismatch: got %T", Clone(original))
	}

	revealed, ok := cloned.Reveal().(map[string]any)
	if !ok {
		t.Fatalf("revealed clone type mismatch: got %T", cloned.Reveal())
	}

	revealed["items"].([]any)[0].(map[string]any)["id"] = "mutated"

	originalRevealed := original.Reveal().(map[string]any)
	if got, want := originalRevealed["items"].([]any)[0].(map[string]any)["id"], "first"; got != want {
		t.Fatalf("secret clone mutated original: got %v want %v", got, want)
	}
}

func TestCloneClonesByteSlice(t *testing.T) {
	t.Parallel()

	original := []byte("payload")
	cloned, ok := Clone(original).([]byte)
	if !ok {
		t.Fatalf("clone type mismatch: got %T", Clone(original))
	}

	cloned[0] = 'P'

	if got, want := string(original), "payload"; got != want {
		t.Fatalf("byte clone mismatch: got %q want %q", got, want)
	}
}

func TestBytesClonesByteSlice(t *testing.T) {
	t.Parallel()

	original := []byte("payload")
	cloned, err := Bytes(original, "body")
	if err != nil {
		t.Fatalf("bytes conversion failed: %v", err)
	}

	original[0] = 'P'

	if got, want := string(cloned), "payload"; got != want {
		t.Fatalf("cloned bytes mismatch: got %q want %q", got, want)
	}
}

func TestRevealStripsSecretWrappersRecursively(t *testing.T) {
	t.Parallel()

	revealed := Reveal(map[string]any{
		"token": secretvalue.New("issued-token"),
		"nested": []any{
			secretvalue.New(map[string]any{"id": "first"}),
		},
	})

	if got, want := revealed, map[string]any{
		"token": "issued-token",
		"nested": []any{
			map[string]any{"id": "first"},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("revealed value mismatch: got %#v want %#v", got, want)
	}
}

func TestStringSliceMapSupportsStringAndListMembers(t *testing.T) {
	t.Parallel()

	values, err := StringSliceMap(map[string]any{
		"Content-Type": "application/json",
		"X-Test":       []any{"one", "two"},
	}, "headers")
	if err != nil {
		t.Fatalf("string slice map failed: %v", err)
	}

	if got, want := values["Content-Type"], []string{"application/json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("single header mismatch: got %#v want %#v", got, want)
	}

	if got, want := values["X-Test"], []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list header mismatch: got %#v want %#v", got, want)
	}
}

func TestValidateCanonicalAcceptsCommonNativeContainers(t *testing.T) {
	t.Parallel()

	err := ValidateCanonical("headers", map[string]any{
		"Content-Type": []string{"application/json"},
	})
	if err != nil {
		t.Fatalf("validate canonical failed: %v", err)
	}
}

func TestDecodeJSONUsesStrictSingleValuePolicy(t *testing.T) {
	t.Parallel()

	t.Run("valid json", func(t *testing.T) {
		t.Parallel()

		value, err := DecodeJSON(`{"retry_after":2}`, "body")
		if err != nil {
			t.Fatalf("decode JSON failed: %v", err)
		}

		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("decoded value type mismatch: got %T", value)
		}

		number, ok := object["retry_after"].(json.Number)
		if !ok {
			t.Fatalf("decoded number type mismatch: got %T", object["retry_after"])
		}

		if got, want := number.String(), "2"; got != want {
			t.Fatalf("decoded number mismatch: got %q want %q", got, want)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		_, err := DecodeJSON(`{"retry_after":`, "body")
		if err == nil {
			t.Fatal("expected JSON decode error, got nil")
		}

		errtest.RequireContains(t, err, runtimevalueUnexpectedEOFFragment)
	})

	t.Run("trailing garbage", func(t *testing.T) {
		t.Parallel()

		_, err := DecodeJSON(`{"retry_after":2} garbage`, "body")
		if err == nil {
			t.Fatal("expected trailing content error, got nil")
		}

		var syntaxErr *json.SyntaxError
		errtest.RequireAs(t, err, &syntaxErr)
		if syntaxErr.Offset == 0 {
			t.Fatal("expected syntax error offset to be populated")
		}
	})

	t.Run("secret input", func(t *testing.T) {
		t.Parallel()

		value, err := DecodeJSON(secretvalue.New(`{"retry_after":2}`), "body")
		if err != nil {
			t.Fatalf("decode JSON failed: %v", err)
		}

		revealed, ok := secretvalue.Reveal(value)
		if !ok {
			t.Fatalf("decoded value type mismatch: got %T", value)
		}

		object, ok := revealed.(map[string]any)
		if !ok {
			t.Fatalf("revealed decoded value type mismatch: got %T", revealed)
		}

		number, ok := object["retry_after"].(json.Number)
		if !ok {
			t.Fatalf("decoded number type mismatch: got %T", object["retry_after"])
		}

		if got, want := number.String(), "2"; got != want {
			t.Fatalf("decoded number mismatch: got %q want %q", got, want)
		}
	})
}
