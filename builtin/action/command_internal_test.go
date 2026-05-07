package action

import (
	"reflect"
	"testing"
)

func TestMergeCommandEnvDeduplicatesBaseAndAppliesOverrides(t *testing.T) {
	t.Parallel()

	got := mergeCommandEnvWithCase(
		[]string{
			"HOME=/tmp/home",
			"PATH=/bin",
			"PATH=/usr/bin",
			"LANG=en_US.UTF-8",
		},
		map[string]string{
			"LANG": "C",
			"TMP":  "/tmp/work",
		},
		false,
	)

	want := []string{
		"HOME=/tmp/home",
		"PATH=/usr/bin",
		"LANG=C",
		"TMP=/tmp/work",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged env mismatch: got %v want %v", got, want)
	}
}

func TestMergeCommandEnvKeepsDistinctCasingOnCaseSensitivePlatforms(t *testing.T) {
	t.Parallel()

	got := mergeCommandEnvWithCase(
		[]string{"Path=/base/bin"},
		map[string]string{"PATH": "/override/bin"},
		false,
	)

	want := []string{
		"Path=/base/bin",
		"PATH=/override/bin",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("case-sensitive merged env mismatch: got %v want %v", got, want)
	}
}

func TestMergeCommandEnvCollapsesWindowsStyleCaseCollisionsDeterministically(t *testing.T) {
	t.Parallel()

	got := mergeCommandEnvWithCase(
		[]string{
			"Path=/base/bin",
			"HOME=/tmp/home",
		},
		map[string]string{
			"PATH": "/override/bin",
			"Path": "/winner/bin",
		},
		true,
	)

	want := []string{
		"Path=/winner/bin",
		"HOME=/tmp/home",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("windows-style merged env mismatch: got %v want %v", got, want)
	}
}
