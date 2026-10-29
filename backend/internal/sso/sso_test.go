package sso

import "testing"

func TestTheBackchannelIsPublicReachablePairs(t *testing.T) {
	got, err := ParseBackchannel(" http://localhost:8180/=http://keycloak:8080 , ,https://id.example=http://id:80")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["http://localhost:8180"] != "http://keycloak:8080" || got["https://id.example"] != "http://id:80" {
		t.Errorf("%v", got)
	}
	for _, bad := range []string{"http://a", "=http://b", "http://a="} {
		if _, err := ParseBackchannel(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
