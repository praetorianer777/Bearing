package openapi

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type priority string

type ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type thing struct {
	ID        string         `json:"id"`
	Summary   string         `json:"summary"`
	Assignee  *ref           `json:"assignee,omitempty"`
	Parent    *ref           `json:"parent"`
	Priority  priority       `json:"priority"`
	Tags      []string       `json:"tags"`
	Counts    map[string]int `json:"counts,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

func TestValidateAgainstTheBuiltSchema(t *testing.T) {
	b := NewBuilder()
	b.Enums[reflect.TypeOf(priority(""))] = []string{"low", "high"}
	s := b.SchemaOf(thing{})
	d := &Document{Components: Components{Schemas: b.Components()}}

	good := `{"id":"1","summary":"x","parent":null,"priority":"low","tags":["a"],"createdAt":"2026-01-01T00:00:00Z","assignee":{"id":"u","name":"n"},"counts":{"a":1}}`
	if err := d.ValidateJSON(s, []byte(good)); err != nil {
		t.Fatalf("a well formed value should pass: %v", err)
	}
	for name, raw := range map[string]string{
		"missing required":  `{"id":"1","summary":"x","parent":null,"priority":"low","createdAt":"2026-01-01T00:00:00Z"}`,
		"wrong type":        `{"id":1,"summary":"x","parent":null,"priority":"low","tags":[],"createdAt":"2026-01-01T00:00:00Z"}`,
		"bad enum":          `{"id":"1","summary":"x","parent":null,"priority":"urgent","tags":[],"createdAt":"2026-01-01T00:00:00Z"}`,
		"null where not":    `{"id":"1","summary":"x","parent":null,"priority":"low","tags":null,"createdAt":"2026-01-01T00:00:00Z"}`,
		"nested wrong":      `{"id":"1","summary":"x","parent":{"id":"p"},"priority":"low","tags":[],"createdAt":"2026-01-01T00:00:00Z"}`,
		"map value wrong":   `{"id":"1","summary":"x","parent":null,"priority":"low","tags":[],"createdAt":"2026-01-01T00:00:00Z","counts":{"a":"one"}}`,
		"fraction as count": `{"id":"1","summary":"x","parent":null,"priority":"low","tags":[],"createdAt":"2026-01-01T00:00:00Z","counts":{"a":1.5}}`,
		"not json":          `{"id":`,
	} {
		if err := d.ValidateJSON(s, []byte(raw)); err == nil {
			t.Errorf("%s should fail", name)
		}
	}
	// A client must tolerate a server that says more than it promised.
	if err := d.ValidateJSON(s, []byte(strings.Replace(good, `"id":"1"`, `"id":"1","extra":true`, 1))); err != nil {
		t.Errorf("extra properties should pass: %v", err)
	}
}

// A pointer to an enum is null or one of its values: null joins as an
// alternative, since a type list beside the enum would still refuse it.
func TestANullableEnumTakesNullAndItsValues(t *testing.T) {
	b := NewBuilder()
	b.Enums[reflect.TypeOf(priority(""))] = []string{"low", "high"}
	s := b.SchemaOf(struct {
		Priority *priority `json:"priority"`
	}{})
	d := &Document{Components: Components{Schemas: b.Components()}}
	for _, raw := range []string{`{"priority":null}`, `{"priority":"low"}`} {
		if err := d.ValidateJSON(s, []byte(raw)); err != nil {
			t.Errorf("%s should pass: %v", raw, err)
		}
	}
	if err := d.ValidateJSON(s, []byte(`{"priority":"urgent"}`)); err == nil {
		t.Error("a value outside the enum should fail")
	}
}

type day struct{ s string }

type span struct {
	From  day  `json:"from"`
	To    *day `json:"to"`
	Until *day `json:"until,omitempty"`
}

// A field of a type described by hand is as required as any other: always there unless it may be left out.
func TestAnOverriddenFieldIsRequiredUnlessItMayBeLeftOut(t *testing.T) {
	b := NewBuilder()
	b.Overrides[reflect.TypeOf(day{})] = &Schema{Type: "string", Format: "date"}
	s := b.SchemaOf(span{})
	d := &Document{Components: Components{Schemas: b.Components()}}
	if err := d.ValidateJSON(s, []byte(`{"to":null}`)); err == nil {
		t.Error("a span without its from passed")
	}
	if err := d.ValidateJSON(s, []byte(`{"from":"2026-01-01","to":null}`)); err != nil {
		t.Errorf("a span with from and a null to failed: %v", err)
	}
}
