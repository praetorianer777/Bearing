package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAPersonalTokenNeedsANameAndALifetime(t *testing.T) {
	s := &Service{}
	for _, c := range []struct {
		in    TokenInput
		field string
	}{
		{TokenInput{Name: " ", Days: 30}, "name"},
		{TokenInput{Name: strings.Repeat("n", MaxTokenNameLength+1), Days: 30}, "name"},
		{TokenInput{Name: "script", Days: 0}, "days"},
		{TokenInput{Name: "script", Days: MaxPersonalTokenDays + 1}, "days"},
	} {
		_, _, _, err := s.MakePersonalToken(t.Context(), uuid.New(), c.in)
		var invalid TokenInvalid
		if !errors.As(err, &invalid) || invalid[c.field] == "" || invalid.Error() == "" {
			t.Errorf("%+v: %v", c.in, err)
		}
	}
	if (TokenInvalid{}).Error() == "" {
		t.Error("an empty refusal says nothing")
	}
}
