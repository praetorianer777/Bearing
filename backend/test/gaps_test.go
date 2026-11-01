//go:build integration

package test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

type gapJSON struct {
	Kind, SubjectKind, SubjectID, Name, From, To string
	UnitID                                       *string
	Size                                         float64
}

func gapsOf(t *testing.T, b *browser, query string) []gapJSON {
	t.Helper()
	var out struct{ Gaps []gapJSON }
	if status := b.do(http.MethodGet, "/gaps?"+query, nil, &out); status != http.StatusOK {
		t.Fatalf("the gaps answered %d", status)
	}
	return out.Gaps
}

// about are the gaps of one subject, written "kind from-to size".
func about(gaps []gapJSON, id string) []string {
	var out []string
	for _, g := range gaps {
		if g.SubjectID == id {
			out = append(out, fmt.Sprintf("%s %s-%s %g", g.Kind, g.From, g.To, g.Size))
		}
	}
	return out
}

func TestGapsFromBearingsOwnData(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	editor, _ := withRole(t, admin, "editor")
	chain := units(t, editor, unitType(t, admin), rootUnit(t, admin), "Sales", "Inside sales")
	sales, inside := chain[0], chain[1]
	head := positions(t, editor, sales, "Head of Sales")[0]
	rep := positions(t, editor, inside, "Sales rep")[0]
	editor.do(http.MethodPatch, "/positions/"+rep, map[string]any{"seats": 2}, nil)
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": head, "from": day(-10)})
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": rep, "share": 50, "from": day(-20), "to": day(-5)})
	give(t, editor, map[string]any{"personId": somebody(t, editor), "positionId": rep, "from": day(-3)})
	approver := orgRole(t, editor, "Deal approver")
	if status := editor.do(http.MethodPost, "/positions/"+rep+"/roles", map[string]any{"roleId": approver.ID, "from": day(-10)}, nil); status != http.StatusCreated {
		t.Fatalf("carrying the role answered %d", status)
	}
	setTarget(t, editor, map[string]any{"unitId": sales, "fte": 3, "from": day(-20)})
	setTarget(t, editor, map[string]any{"positionId": head, "fte": 0.5, "from": day(-10)})

	gaps := gapsOf(t, editor, "unitId="+sales+"&subUnits=true&from="+day(-20)+"&to="+day(-1))
	span := func(kind string, from, to int, size float64) string {
		return fmt.Sprintf("%s %s-%s %g", kind, day(from), day(to), size)
	}
	for _, c := range []struct {
		name, id string
		want     []string
	}{
		{"the head", head, []string{span("vacant", -20, -11, 1), span("over_target", -10, -1, 0.5)}},
		{"the rep", rep, []string{span("vacant", -20, -6, 1.5), span("vacant", -5, -4, 2), span("vacant", -3, -1, 1)}},
		{"Sales", sales, []string{span("under_target", -20, -11, 2.5), span("under_target", -10, -6, 1.5), span("under_target", -5, -4, 2),
			span("under_target", -3, -1, 1)}},
		{"the role", approver.ID, []string{span("unheld", -20, -11, 0), span("single_holder", -10, -6, 1), span("unheld", -5, -4, 0),
			span("single_holder", -3, -1, 1)}},
	} {
		if got := about(gaps, c.id); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i].From < gaps[i-1].From {
			t.Fatalf("not earliest first: %+v before %+v", gaps[i-1], gaps[i])
		}
	}
	for _, g := range gaps {
		if g.SubjectID == rep && (g.UnitID == nil || *g.UnitID != inside || g.Name != "Sales rep") {
			t.Errorf("the rep's gap names its unit: %+v", g)
		}
	}

	alone := gapsOf(t, editor, "unitId="+sales+"&from="+day(-20)+"&to="+day(-1))
	if about(alone, rep) != nil || about(alone, approver.ID) != nil || about(alone, head) == nil {
		t.Errorf("Sales alone has the head's gaps but not the team's or its role's: %+v", alone)
	}
	whole := gapsOf(t, editor, "from="+day(-20)+"&to="+day(-1))
	if !slices.Equal(about(whole, rep), about(gaps, rep)) || !slices.Equal(about(whole, approver.ID), about(gaps, approver.ID)) {
		t.Errorf("the whole organisation has the same gaps of the team")
	}
}

func TestGapsRefuseWhatTheyCannotSearch(t *testing.T) {
	editor, _ := orgFixture(t)
	for _, query := range []string{"unitId=nope", "unitId=0195a3c4-0000-7000-8000-000000000000", "from=soon", "from=" + day(0) + "&to=" + day(-1)} {
		if status := editor.do(http.MethodGet, "/gaps?"+query, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d", query, status)
		}
	}
}
