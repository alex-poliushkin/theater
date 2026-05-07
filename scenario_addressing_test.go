package theater

import "testing"

func TestInternalScenarioAccessPolicyAllowsOnlyPublicScenarioAddresses(t *testing.T) {
	t.Parallel()

	policy := internalScenarioAccessPolicy{}
	tests := []struct {
		name    string
		address scenarioAddress
		allow   bool
	}{
		{name: "public address", address: "identity/login", allow: true},
		{name: "nested internal namespace", address: "identity/internal/bootstrap", allow: false},
		{name: "root internal namespace", address: "internal/bootstrap", allow: false},
		{name: "terminal internal segment only", address: "identity/internal", allow: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, want := policy.AllowsDirectCall(tt.address), tt.allow; got != want {
				t.Fatalf("AllowsDirectCall(%q) = %v, want %v", tt.address, got, want)
			}
		})
	}
}
