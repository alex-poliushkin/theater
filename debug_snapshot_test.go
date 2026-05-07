package theater

import "testing"

func TestDebugSnapshotBuilderValuesSectionSortsFieldsAndTracksOmittedCount(t *testing.T) {
	t.Parallel()

	builder := debugSnapshotBuilder{
		previewLimit:    32,
		collectionLimit: 8,
		sectionLimit:    2,
	}

	section := builder.valuesSection(
		Values{
			"beta":  "two",
			"alpha": "one",
			"gamma": "three",
		},
		nil,
		"debug.input",
	)

	if got, want := len(section.Fields), 2; got != want {
		t.Fatalf("field count mismatch: got %d want %d", got, want)
	}
	if got, want := section.Omitted, 1; got != want {
		t.Fatalf("omitted count mismatch: got %d want %d", got, want)
	}
	if got, want := section.Fields[0].Key, "alpha"; got != want {
		t.Fatalf("field[0] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[0].Origin, "debug.input.alpha"; got != want {
		t.Fatalf("field[0] origin mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[1].Key, "beta"; got != want {
		t.Fatalf("field[1] key mismatch: got %q want %q", got, want)
	}
}

func TestDebugSnapshotBuilderSafeValueUsesRedactionTruncationAndCollectionCaps(t *testing.T) {
	t.Parallel()

	builder := debugSnapshotBuilder{
		previewLimit:    5,
		collectionLimit: 2,
		sectionLimit:    8,
	}

	spec := ValueContract{
		Kind: ValueKindObject,
		Fields: map[string]ValueContract{
			"items": {
				Kind: ValueKindList,
				Elem: &ValueContract{Kind: ValueKindString},
			},
			"note": {
				Kind: ValueKindString,
			},
			"secret": {
				Kind:        ValueKindString,
				Sensitivity: SensitivitySecret,
			},
		},
	}

	value := map[string]any{
		"items":  []any{"first", "second", "third"},
		"note":   "abcdefghij",
		"secret": "issued-token",
	}

	safe := builder.safeValue(value, spec)
	if got, want := safe.Kind, "object"; got != want {
		t.Fatalf("kind mismatch: got %q want %q", got, want)
	}
	if got, want := len(safe.Children), 2; got != want {
		t.Fatalf("child count mismatch: got %d want %d", got, want)
	}
	if got, want := safe.Omitted, 1; got != want {
		t.Fatalf("child omitted mismatch: got %d want %d", got, want)
	}

	items := safe.Children[0]
	if got, want := items.Key, "items"; got != want {
		t.Fatalf("items key mismatch: got %q want %q", got, want)
	}
	if got, want := len(items.Value.Children), 2; got != want {
		t.Fatalf("items child count mismatch: got %d want %d", got, want)
	}
	if got, want := items.Value.Omitted, 1; got != want {
		t.Fatalf("items omitted mismatch: got %d want %d", got, want)
	}
	if got, want := items.Value.Children[0].Key, "0"; got != want {
		t.Fatalf("items child key mismatch: got %q want %q", got, want)
	}

	note := safe.Children[1]
	if got, want := note.Key, "note"; got != want {
		t.Fatalf("note key mismatch: got %q want %q", got, want)
	}
	if !note.Value.Truncated {
		t.Fatal("note value must be marked truncated")
	}
	if got, want := note.Value.Text, "a...j"; got != want {
		t.Fatalf("note preview mismatch: got %q want %q", got, want)
	}

	secretValue := builder.safeValue("issued-token", ValueContract{
		Kind:        ValueKindString,
		Sensitivity: SensitivitySecret,
	})
	if !secretValue.Redacted {
		t.Fatal("secret value must be redacted")
	}
	if got, want := secretValue.Text, redactedPreview; got != want {
		t.Fatalf("secret preview mismatch: got %q want %q", got, want)
	}
}

func TestDebugSnapshotBuilderScopeSectionPrefersNearestFrame(t *testing.T) {
	t.Parallel()

	parent := newValueScope(nil)
	parent.writeAll(Values{
		"shared": "parent",
		"outer":  "outer-value",
	})

	child := newValueScope(parent)
	child.writeAll(Values{
		"shared": "child",
		"inner":  "inner-value",
	})

	section := newDebugSnapshotBuilder().scopeSection(child)
	if got, want := len(section.Fields), 3; got != want {
		t.Fatalf("scope field count mismatch: got %d want %d", got, want)
	}

	if got, want := section.Fields[0].Key, "inner"; got != want {
		t.Fatalf("field[0] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[0].Origin, "scope.current"; got != want {
		t.Fatalf("field[0] origin mismatch: got %q want %q", got, want)
	}

	if got, want := section.Fields[1].Key, "outer"; got != want {
		t.Fatalf("field[1] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[1].Origin, "scope.parent.1"; got != want {
		t.Fatalf("field[1] origin mismatch: got %q want %q", got, want)
	}

	if got, want := section.Fields[2].Key, "shared"; got != want {
		t.Fatalf("field[2] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[2].Origin, "scope.current"; got != want {
		t.Fatalf("field[2] origin mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[2].Value.Text, "child"; got != want {
		t.Fatalf("field[2] value mismatch: got %q want %q", got, want)
	}
}

func TestDebugSnapshotBuilderActionSectionsDropUndeclaredFieldsWithoutContracts(t *testing.T) {
	t.Parallel()

	builder := newDebugSnapshotBuilder()
	contract := ActionContract{}

	inputs := builder.actionInputsSection(Args{
		"token": "issued-token",
	}, contract, nil)
	if got := len(inputs.Fields); got != 0 {
		t.Fatalf("input fields must be empty without declared inputs: got %d", got)
	}

	outputs := builder.actionOutputsSection(Outputs{
		"token": "issued-token",
	}, contract)
	if got := len(outputs.Fields); got != 0 {
		t.Fatalf("output fields must be empty without declared outputs: got %d", got)
	}
}

func TestDebugSnapshotBuilderExpectationInputsReserveActualKey(t *testing.T) {
	t.Parallel()

	builder := newDebugSnapshotBuilder()
	section := builder.expectationInputsSection(
		"subject-value",
		ValueContract{Kind: ValueKindString},
		Values{
			"actual": "matcher-shadow",
			"limit":  3,
		},
		[]MatcherArg{
			{
				Name:    "actual",
				Accepts: ValueContract{Kind: ValueKindString},
			},
			{
				Name:    "limit",
				Accepts: ValueContract{Kind: ValueKindNumber},
			},
		},
		nil,
	)
	if got, want := len(section.Fields), 3; got != want {
		t.Fatalf("field count mismatch: got %d want %d", got, want)
	}

	if got, want := section.Fields[0].Key, "actual"; got != want {
		t.Fatalf("field[0] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[0].Origin, "expectation.actual"; got != want {
		t.Fatalf("field[0] origin mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[0].Value.Text, "subject-value"; got != want {
		t.Fatalf("field[0] value mismatch: got %q want %q", got, want)
	}

	if got, want := section.Fields[1].Key, "arg.actual"; got != want {
		t.Fatalf("field[1] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[1].Origin, "expectation.arg.actual"; got != want {
		t.Fatalf("field[1] origin mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[1].Value.Text, "matcher-shadow"; got != want {
		t.Fatalf("field[1] value mismatch: got %q want %q", got, want)
	}

	if got, want := section.Fields[2].Key, "arg.limit"; got != want {
		t.Fatalf("field[2] key mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[2].Origin, "expectation.arg.limit"; got != want {
		t.Fatalf("field[2] origin mismatch: got %q want %q", got, want)
	}
	if got, want := section.Fields[2].Value.Text, "3"; got != want {
		t.Fatalf("field[2] value mismatch: got %q want %q", got, want)
	}
}
