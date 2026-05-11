package registry

import (
	"strings"
	"testing"
)

func TestPluginEntryValidateHostEnvironmentGrants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		grants Grants
		want   string
	}{
		{
			name: "empty host grant name",
			grants: Grants{
				EnvFromHost: []string{""},
			},
			want: "env_from_host grant name must not be empty",
		},
		{
			name: "duplicate host grant name",
			grants: Grants{
				EnvFromHost: []string{"THEATER_TOKEN", "THEATER_TOKEN"},
			},
			want: `env_from_host grant "THEATER_TOKEN" is duplicated`,
		},
		{
			name: "invalid host grant name",
			grants: Grants{
				EnvFromHost: []string{" THEATER_TOKEN "},
			},
			want: `env_from_host grant name " THEATER_TOKEN " must be a valid environment variable name`,
		},
		{
			name: "invalid literal grant name",
			grants: Grants{
				Env: map[string]string{
					"THEATER-TOKEN": "literal",
				},
			},
			want: `env grant name "THEATER-TOKEN" must be a valid environment variable name`,
		},
		{
			name: "literal and host grant overlap",
			grants: Grants{
				Env: map[string]string{
					"THEATER_TOKEN": "literal",
				},
				EnvFromHost: []string{"THEATER_TOKEN"},
			},
			want: `env grant "THEATER_TOKEN" cannot be both literal and env_from_host`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			entry := validPluginEntry()
			entry.Grants = test.grants
			err := entry.Validate("example")
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error mismatch: got %q want substring %q", err, test.want)
			}
		})
	}
}

func TestPluginEntryValidateAcceptsHostEnvironmentGrants(t *testing.T) {
	t.Parallel()

	entry := validPluginEntry()
	entry.Grants = Grants{
		EnvFromHost: []string{"THEATER_TOKEN", "THEATER_ENDPOINT"},
	}
	if err := entry.Validate("example"); err != nil {
		t.Fatalf("validate entry: %v", err)
	}
}

func validPluginEntry() PluginEntry {
	return PluginEntry{
		Manifest: "manifest.json",
		Exec: ExecSpec{
			Command: []string{"plugin"},
		},
		AllowCapabilities: []string{"action.example.run"},
	}
}
