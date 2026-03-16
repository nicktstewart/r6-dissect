package test

import (
	"testing"

	"github.com/redraskal/r6-dissect/dissect"
)

func TestUnknownOperatorRoleForGameVersion(t *testing.T) {
	if role, ok := dissect.Operator(1).RoleForGameVersion("Y11S2"); !ok || role != dissect.Attack {
		t.Fatalf("Y11S2 unknown role = %q, %v; want %q, true", role, ok, dissect.Attack)
	}
	if role, ok := dissect.Operator(1).RoleForGameVersion("Y11S3"); !ok || role != dissect.Defense {
		t.Fatalf("Y11S3 unknown role = %q, %v; want %q, true", role, ok, dissect.Defense)
	}
	if role, ok := dissect.Operator(1).RoleForGameVersion("Y11S4"); ok || role != "" {
		t.Fatalf("Y11S4 unknown role = %q, %v; want empty, false", role, ok)
	}
}

func TestUnknownOperatorNameForGameVersion(t *testing.T) {
	tests := []struct {
		name        string
		gameVersion string
		wantName    string
	}{
		{name: "Y11S2 unknown maps to Dokkaebi", gameVersion: "Y11S2", wantName: "Dokkaebi"},
		{name: "Y11S3 unknown maps to season fallback", gameVersion: "Y11S3", wantName: "Y11S3NewDefender"},
		{name: "Y11S4 unknown maps to generic fallback", gameVersion: "Y11S4", wantName: "Y11S4UnknownOperator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dissect.Operator(123456789).NameForGameVersion(tt.gameVersion)
			if got != tt.wantName {
				t.Fatalf("operator name = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestRoleReturnsEmptyForUnknownOperator(t *testing.T) {
	if got := dissect.Operator(123456789).Role(); got != "" {
		t.Fatalf("Role() = %q, want empty", got)
	}
}
