package org

import (
	"slices"
	"strings"
	"testing"
)

func TestSearchWordsAreRunsOfLettersAndDigits(t *testing.T) {
	if got := SearchWords("  Zoë's  team-2: 'drop' & | ! "); !slices.Equal(got, []string{"Zoë", "s", "team", "2", "drop"}) {
		t.Errorf("words: %q", got)
	}
	if got := SearchWords(strings.Repeat("a ", 30)); len(got) != maxSearchWords {
		t.Errorf("at most %d words: %d", maxSearchWords, len(got))
	}
	if got := SearchWords("!!! ---"); len(got) != 0 {
		t.Errorf("no words: %q", got)
	}
}

func TestEveryWordIsAskedForAsAPrefix(t *testing.T) {
	if got := prefixQuery([]string{"Supplier", "INV"}); got != "'supplier':* & 'inv':*" {
		t.Errorf("prefix query: %s", got)
	}
}
