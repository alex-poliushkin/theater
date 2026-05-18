package theater

import "testing"

func TestHTTPDiagnosticsForExpectationFailureClassifiesKnownSubjects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		expectation expectationPlan
		failure     *Failure
		want        HTTPDiagnosticFailureKind
	}{
		{
			name: "status",
			expectation: expectationPlan{
				Subject: subjectPlan{Field: "status_code"},
			},
			failure: &Failure{Kind: FailureKindExpectation},
			want:    HTTPDiagnosticFailureStatus,
		},
		{
			name: "headers",
			expectation: expectationPlan{
				Subject: subjectPlan{Field: "headers"},
			},
			failure: &Failure{Kind: FailureKindExpectation},
			want:    HTTPDiagnosticFailureHeader,
		},
		{
			name: "body parse",
			expectation: expectationPlan{
				Subject: subjectPlan{
					Field:        "body",
					selectorPlan: selectorPlan{Decode: DecodeJSON},
				},
			},
			failure: &Failure{Kind: FailureKindObservation},
			want:    HTTPDiagnosticFailureBodyParse,
		},
		{
			name: "body expectation",
			expectation: expectationPlan{
				Subject: subjectPlan{Field: "body"},
			},
			failure: &Failure{Kind: FailureKindExpectation},
			want:    HTTPDiagnosticFailureExpectation,
		},
		{
			name: "property subject",
			expectation: expectationPlan{
				Subject: subjectPlan{From: SubjectFromProperty, Ref: "profile"},
			},
			failure: &Failure{Kind: FailureKindExpectation},
			want:    "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := httpDiagnosticFailureKindForExpectation(tc.expectation, tc.failure)
			if got != tc.want {
				t.Fatalf("failure kind mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestHTTPDiagnosticsForExpectationFailureClonesBeforeAnnotating(t *testing.T) {
	t.Parallel()

	diagnostics := []NodeDiagnostic{
		{
			Kind: NodeDiagnosticKindHTTP,
			HTTP: &HTTPDiagnostic{
				Method: "GET",
			},
		},
	}

	got := httpDiagnosticsForExpectationFailure(diagnostics, expectationPlan{
		Subject: subjectPlan{Field: "status_code"},
	}, &Failure{Kind: FailureKindExpectation})

	if got[0].HTTP.FailureKind != HTTPDiagnosticFailureStatus {
		t.Fatalf("annotated failure kind mismatch: got %q", got[0].HTTP.FailureKind)
	}
	if diagnostics[0].HTTP.FailureKind != "" {
		t.Fatalf("source diagnostic was mutated: %#v", diagnostics[0].HTTP)
	}
}
