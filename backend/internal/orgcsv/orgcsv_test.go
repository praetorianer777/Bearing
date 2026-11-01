package orgcsv

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/org"
)

func TestAFileIsReadByItsColumnsInAnyOrder(t *testing.T) {
	sh, errs := read("people", "\uFEFFEmail, Name ,ref\nana@x.test,Ana,a\n\n , , \n\"b@x.test\",\"Bo, Jr\",b\n")
	if len(errs) != 0 {
		t.Fatalf("errors: %+v", errs)
	}
	if len(sh.rows) != 2 || sh.rows[1]["name"] != "Bo, Jr" || sh.rows[0]["email"] != "ana@x.test" || !slices.Equal(sh.nums, []int{2, 5}) {
		t.Errorf("rows %+v at %v", sh.rows, sh.nums)
	}
	if _, errs := read("units", ""); errs != nil {
		t.Errorf("an empty file is no file: %+v", errs)
	}
	if _, errs := read("units", "ref,name,colour\n"); len(errs) != 1 || !strings.Contains(errs[0].Message, `"colour"`) || errs[0].Row != 1 {
		t.Errorf("an unknown column: %+v", errs)
	}
	if _, errs := read("units", "ref,name\n\"a,b\n"); len(errs) != 1 || errs[0].Row != 2 {
		t.Errorf("a broken quote: %+v", errs)
	}
	long := "ref,name\n" + strings.Repeat("a,b\n", MaxRows+1)
	if sh, errs := read("units", long); len(sh.rows) != MaxRows || len(errs) != 1 || errs[0].Row != MaxRows+2 {
		t.Errorf("too many rows: %d %+v", len(sh.rows), errs)
	}
}

func TestUnitsGoOutParentsFirst(t *testing.T) {
	root, a, b := uuid.New(), uuid.New(), uuid.New()
	got := parentsFirst([]org.Unit{{ID: b, ParentID: &a, Name: "b"}, {ID: a, ParentID: &root, Name: "a"}, {ID: root, Name: "root"}})
	if len(got) != 3 || got[0].ID != root || got[1].ID != a || got[2].ID != b {
		t.Errorf("order: %v", []string{got[0].Name, got[1].Name, got[2].Name})
	}
}

func TestAPlanCountsItsRowsAndARefusalReadsAsASentence(t *testing.T) {
	p := Plan{Changes: []Change{{Action: "create"}, {Action: "create"}, {Action: "unchanged"}}}
	if got := p.Counts(); got["create"] != 2 || got["unchanged"] != 1 || got["update"] != 0 {
		t.Errorf("counts: %v", got)
	}
	if got := sentence("more people would hold the position then than it has seats"); got != "More people would hold the position then than it has seats." {
		t.Errorf("sentence: %q", got)
	}
	e := &ApplyError{RowError{"holdings", 4, "No."}, nil}
	if e.Error() != "holdings row 4: No." {
		t.Errorf("error: %q", e.Error())
	}
}
