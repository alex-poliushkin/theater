package spec_test

import (
	"context"
	"testing"

	specmodel "github.com/alex-poliushkin/theater/spec"
)

type noopMatcher struct{}

func TestLeafPackageExposesAuthoringAndMatcherModel(t *testing.T) {
	t.Parallel()

	pointer, err := specmodel.ParseJSONPointer("/token")
	if err != nil {
		t.Fatalf("parse json pointer failed: %v", err)
	}

	catalog, err := specmodel.NewMatcherCatalog(specmodel.MatcherDescriptor{
		Ref:    "expectation.token",
		Actual: specmodel.ValueContract{Kind: specmodel.ValueKindString},
		Sugar:  specmodel.SugarSpec{Form: specmodel.SugarFormNone},
		Compile: func(specmodel.MatcherCompileContext, specmodel.Values) (specmodel.Matcher, error) {
			return noopMatcher{}, nil
		},
	})
	if err != nil {
		t.Fatalf("new matcher catalog failed: %v", err)
	}

	stage := specmodel.StageSpec{
		ID: "main",
		Scenarios: []specmodel.ScenarioSpec{{
			ID: "login",
			Acts: []specmodel.ActSpec{{
				ID:     "submit",
				Action: specmodel.ActionSpec{Use: "action.login"},
				Expectations: []specmodel.ExpectationSpec{{
					ID:      "token",
					Subject: specmodel.SubjectSpec{Field: "token", Path: pointer},
					Assert:  specmodel.AssertSpec{Ref: "expectation.token"},
				}},
			}},
		}},
	}

	if got, want := stage.Scenarios[0].Acts[0].Expectations[0].Subject.Path.String(), "/token"; got != want {
		t.Fatalf("subject path mismatch: got %q want %q", got, want)
	}

	descriptors := catalog.Descriptors()
	if got, want := len(descriptors), 1; got != want {
		t.Fatalf("descriptor count mismatch: got %d want %d", got, want)
	}
	if got, want := descriptors[0].Ref, "expectation.token"; got != want {
		t.Fatalf("descriptor ref mismatch: got %q want %q", got, want)
	}
}

func (noopMatcher) Check(context.Context, any) error {
	return nil
}
