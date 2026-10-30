package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/audit"
)

// Who ran a keys command: the --operator flag (anywhere after "keys"), else
// SNIP_OPERATOR, else "unknown" with a warning. Bad names are refused.
func TestKeyOperator(t *testing.T) {
	tests := []struct {
		name, env    string
		args         []string
		wantRest     []string
		wantOperator string
		wantWarning  bool
		wantErr      bool
	}{
		{name: "flag at the end", args: []string{"keys", "rotate", "1", "--operator", "alice"},
			wantRest: []string{"keys", "rotate", "1"}, wantOperator: "alice"},
		{name: "flag before the subcommand's argument", args: []string{"keys", "create", "--operator=bob", "ci"},
			wantRest: []string{"keys", "create", "ci"}, wantOperator: "bob"},
		{name: "single dash", args: []string{"keys", "revoke", "-operator", "carol", "7"},
			wantRest: []string{"keys", "revoke", "7"}, wantOperator: "carol"},
		{name: "flag beats SNIP_OPERATOR", env: "ci", args: []string{"keys", "create", "me", "--operator", "dave"},
			wantRest: []string{"keys", "create", "me"}, wantOperator: "dave"},
		{name: "SNIP_OPERATOR", env: "github-actions", args: []string{"keys", "create", "me"},
			wantRest: []string{"keys", "create", "me"}, wantOperator: "github-actions"},
		{name: "neither: unknown, with a warning", args: []string{"keys", "create", "me"},
			wantRest: []string{"keys", "create", "me"}, wantOperator: audit.UnknownOperator, wantWarning: true},
		{name: "flag without a name", args: []string{"keys", "create", "me", "--operator"}, wantErr: true},
		{name: "empty name", args: []string{"keys", "create", "me", "--operator="}, wantErr: true},
		{name: "blank name", env: "  ", args: []string{"keys", "create", "me"}, wantErr: true},
		{name: "control characters", args: []string{"keys", "create", "me", "--operator", "eve\n{\"forged\":1}"}, wantErr: true},
		{name: "too long", args: []string{"keys", "create", "me", "--operator", strings.Repeat("x", 65)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			rest, op, err := keyOperator(tt.args, tt.env, &stderr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("keyOperator = %q, %q, want an error", rest, op)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(rest, tt.wantRest) || op != tt.wantOperator {
				t.Errorf("keyOperator = %q, %q, want %q, %q", rest, op, tt.wantRest, tt.wantOperator)
			}
			if warned := strings.Contains(stderr.String(), "warning"); warned != tt.wantWarning {
				t.Errorf("stderr = %q, want a warning: %v", stderr.String(), tt.wantWarning)
			}
		})
	}
}
