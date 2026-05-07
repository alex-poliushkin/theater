package generator

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
)

func TestGenerateSequenceUsesStartAndStep(t *testing.T) {
	t.Parallel()

	value, err := generateSequence(generatorTestRequest(theater.Args{
		"start": 10,
		"step":  2,
	}, 3))
	if err != nil {
		t.Fatalf("generate sequence failed: %v", err)
	}

	if got, want := value, int64(14); got != want {
		t.Fatalf("sequence mismatch: got %v want %v", got, want)
	}
}

func TestGenerateEmailUsesDomainAndScenarioUniqueness(t *testing.T) {
	t.Parallel()

	first, err := generateEmail(generatorTestRequest(theater.Args{
		"domain": "example.test",
		"stem":   "demo-user",
	}, 1))
	if err != nil {
		t.Fatalf("generate email failed: %v", err)
	}

	second, err := generateEmail(generatorTestRequest(theater.Args{
		"domain": "example.test",
		"stem":   "demo-user",
	}, 2))
	if err != nil {
		t.Fatalf("generate second email failed: %v", err)
	}

	if got := first.(string); !strings.HasSuffix(got, "@example.test") {
		t.Fatalf("email domain mismatch: got %q", got)
	}
	if first == second {
		t.Fatalf("email values must differ across scenario sequence, got %q", first)
	}
}

func TestGeneratePhoneFailsOnExhaustion(t *testing.T) {
	t.Parallel()

	_, err := generatePhone(generatorTestRequest(theater.Args{
		"prefix": "+1415555",
		"digits": 1,
	}, 11))
	if err == nil {
		t.Fatal("expected phone exhaustion error")
	}

	if got := err.Error(); !strings.Contains(got, "exhausted") {
		t.Fatalf("phone exhaustion mismatch: got %q", got)
	}
}

func TestGeneratePhoneUsesSequentialSuffixByDefault(t *testing.T) {
	t.Parallel()

	first, err := generatePhone(generatorTestRequest(theater.Args{
		"prefix": "+1415555",
		"digits": 3,
	}, 1))
	if err != nil {
		t.Fatalf("generate first phone failed: %v", err)
	}

	third, err := generatePhone(generatorTestRequest(theater.Args{
		"prefix": "+1415555",
		"digits": 3,
	}, 3))
	if err != nil {
		t.Fatalf("generate third phone failed: %v", err)
	}

	if got, want := first, "+1415555000"; got != want {
		t.Fatalf("first sequential phone mismatch: got %v want %v", got, want)
	}
	if got, want := third, "+1415555002"; got != want {
		t.Fatalf("third sequential phone mismatch: got %v want %v", got, want)
	}
}

func TestGeneratePhoneRandomModeShufflesSuffixSpaceDeterministically(t *testing.T) {
	t.Parallel()

	const (
		prefix = "+1415555"
		digits = 1
	)

	seen := make(map[string]struct{}, 10)
	values := make([]string, 0, 10)
	sequential := make([]string, 0, 10)
	for scenarioSeq := 1; scenarioSeq <= 10; scenarioSeq++ {
		value, err := generatePhone(generatorTestRequest(theater.Args{
			"prefix": prefix,
			"digits": digits,
			"random": true,
		}, scenarioSeq))
		if err != nil {
			t.Fatalf("generate random phone failed for scenario %d: %v", scenarioSeq, err)
		}

		phone := value.(string)
		if _, ok := seen[phone]; ok {
			t.Fatalf("random phone must stay unique within finite space, duplicate %q", phone)
		}
		seen[phone] = struct{}{}
		values = append(values, phone)
		sequential = append(sequential, fmt.Sprintf("%s%0*d", prefix, digits, scenarioSeq-1))
	}

	if len(seen) != 10 {
		t.Fatalf("random phone uniqueness mismatch: got %d unique values", len(seen))
	}
	if strings.Join(values, ",") == strings.Join(sequential, ",") {
		t.Fatalf("random phone order must differ from sequential order: got %v", values)
	}

	replayed := make([]string, 0, 10)
	for scenarioSeq := 1; scenarioSeq <= 10; scenarioSeq++ {
		value, err := generatePhone(generatorTestRequest(theater.Args{
			"prefix": prefix,
			"digits": digits,
			"random": true,
		}, scenarioSeq))
		if err != nil {
			t.Fatalf("replay random phone failed for scenario %d: %v", scenarioSeq, err)
		}

		replayed = append(replayed, value.(string))
	}

	if strings.Join(values, ",") != strings.Join(replayed, ",") {
		t.Fatalf("random phone mode must stay deterministic: first=%v replay=%v", values, replayed)
	}
}

func TestGenerateUUIDSupportsV4AndV7(t *testing.T) {
	t.Parallel()

	v4, err := generateUUID(generatorTestRequest(theater.Args{}, 1))
	if err != nil {
		t.Fatalf("generate v4 uuid failed: %v", err)
	}

	v7, err := generateUUID(generatorTestRequest(theater.Args{"version": "v7"}, 1))
	if err != nil {
		t.Fatalf("generate v7 uuid failed: %v", err)
	}

	if got := v4.(string)[14]; got != '4' {
		t.Fatalf("uuid v4 version nibble mismatch: got %q", string(got))
	}
	if got := v7.(string)[14]; got != '7' {
		t.Fatalf("uuid v7 version nibble mismatch: got %q", string(got))
	}
}

func TestGenerateSlugHonorsMaxLength(t *testing.T) {
	t.Parallel()

	value, err := generateSlug(generatorTestRequest(theater.Args{
		"prefix":     "Very Long Prefix For Slug",
		"max_length": 18,
	}, 2))
	if err != nil {
		t.Fatalf("generate slug failed: %v", err)
	}

	slug := value.(string)
	if len(slug) > 18 {
		t.Fatalf("slug length mismatch: got %d want <= 18", len(slug))
	}
	if !strings.Contains(slug, "-2") {
		t.Fatalf("slug ordinal mismatch: got %q", slug)
	}
}

func generatorTestRequest(args theater.Args, scenarioSeq int) theater.GeneratorRequest {
	return theater.GeneratorRequest{
		Args: args,
		Generation: theater.GenerationMetadata{
			Seed:     "0123456789abcdef0123456789abcdef",
			BaseTime: time.Date(2026, time.March, 28, 9, 0, 0, 0, time.UTC),
		},
		BindingPath:    "stage.main/scenario.generate/act.fixtures/output.email",
		ScenarioCallID: "generate-call",
		ScenarioSeq:    scenarioSeq,
	}
}
