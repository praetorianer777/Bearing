package main

import (
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/auth"
)

func TestMembersAreEmailAndRolePairs(t *testing.T) {
	got, err := parseMembers(" alice@suite.test=administrator, bob@suite.test=member ,")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["alice@suite.test"] != auth.RoleAdministrator || got["bob@suite.test"] != auth.RoleMember {
		t.Errorf("%v", got)
	}
	for _, bad := range []string{"alice@suite.test", "alice@suite.test=boss"} {
		if _, err := parseMembers(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
