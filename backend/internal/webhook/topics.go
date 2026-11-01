package webhook

import (
	"time"

	"github.com/google/uuid"
)

// The organisation's topics, as docs/integration.md lists them.
const (
	TopicHolderChanged         = "position.holder_changed"
	TopicVacated               = "position.vacated"
	TopicReportingLineChanged  = "reporting_line.changed"
	TopicUnitChanged           = "unit.changed"
	TopicResponsibilityChanged = "responsibility.changed"
)

// Topics are the events an endpoint may take besides TopicAny.
var Topics = []string{TopicHolderChanged, TopicVacated, TopicReportingLineChanged, TopicUnitChanged, TopicResponsibilityChanged}

// Change is a row a model table wrote, as webhook_change keeps it.
type Change struct {
	ID        uuid.UUID
	At        time.Time
	ActorID   *uuid.UUID
	Table     string
	Operation string
	Before    map[string]any
	After     map[string]any
}

// Event is one topic a change stands for, with its payload.
type Event struct {
	Topic   string
	Payload map[string]any
}

// Lookup reads what a payload names beside the ids a change holds: each answers nil for what is gone.
type Lookup interface {
	Position(id string) map[string]any
	Person(id string) map[string]any
	Unit(id string) map[string]any
	Role(id string) map[string]any
	Process(id string) map[string]any
	Actor(id uuid.UUID) map[string]any
	// Vacant says whether the position has no holder on the day, written 2006-01-02.
	Vacant(positionID, day string) bool
}

// row is the side of a change that still stands: the row after, or before for a deletion.
func (c Change) row() map[string]any {
	if c.After != nil {
		return c.After
	}
	return c.Before
}

func (c Change) str(key string) string {
	s, _ := c.row()[key].(string)
	return s
}

// kind is what happened to the row, in the words the payloads use.
func (c Change) kind() string {
	switch c.Operation {
	case "insert":
		return "added"
	case "delete":
		return "removed"
	}
	return "changed"
}

// Announce is the events a change stands for: none for a table no topic follows.
func Announce(c Change, look Lookup, today string) []Event {
	base := func(fields map[string]any) map[string]any {
		fields["change"] = c.kind()
		if c.ActorID != nil {
			fields["actor"] = look.Actor(*c.ActorID)
		} else {
			fields["actor"] = nil
		}
		return fields
	}
	dated := func() map[string]any {
		return map[string]any{"from": c.row()["valid_from"], "to": c.row()["valid_to"]}
	}
	switch c.Table {
	case "holding":
		holding := dated()
		holding["id"], holding["share"] = c.str("id"), c.row()["share"]
		if role := c.str("role_id"); role != "" {
			return []Event{{TopicResponsibilityChanged, base(map[string]any{"cause": "role_holder", "role": look.Role(role),
				"person": look.Person(c.str("person_id")), "holding": holding})}}
		}
		position := c.str("position_id")
		out := []Event{{TopicHolderChanged, base(map[string]any{"position": look.Position(position), "person": look.Person(c.str("person_id")),
			"holding": holding})}}
		if on := leftOn(c, today); on != "" && look.Vacant(position, on) {
			out = append(out, Event{TopicVacated, base(map[string]any{"position": look.Position(position), "on": on,
				"person": look.Person(c.str("person_id"))})})
		}
		return out
	case "reporting_line":
		line := dated()
		line["kind"] = c.row()["kind"]
		return []Event{{TopicReportingLineChanged, base(map[string]any{"position": look.Position(c.str("position_id")),
			"reportsTo": look.Position(c.str("reports_to_id")), "line": line})}}
	case "unit":
		unit := look.Unit(c.str("id"))
		if unit == nil {
			unit = map[string]any{"id": c.str("id"), "name": c.row()["name"]}
		}
		return []Event{{TopicUnitChanged, base(map[string]any{"cause": "unit", "unit": unit})}}
	case "unit_placement":
		placement := dated()
		placement["parentId"] = c.row()["parent_id"]
		return []Event{{TopicUnitChanged, base(map[string]any{"cause": "placement", "unit": look.Unit(c.str("unit_id")), "placement": placement})}}
	case "position_role":
		return []Event{{TopicResponsibilityChanged, base(map[string]any{"cause": "role_carrier", "role": look.Role(c.str("role_id")),
			"position": look.Position(c.str("position_id")), "carried": dated()})}}
	case "deputy":
		return []Event{{TopicResponsibilityChanged, base(map[string]any{"cause": "deputy", "position": look.Position(c.str("position_id")),
			"deputy": look.Position(c.str("deputy_id")), "rank": c.row()["rank"]})}}
	case "process_version":
		return []Event{{TopicResponsibilityChanged, base(map[string]any{"cause": "process_published", "process": look.Process(c.str("process_id")),
			"version": c.row()["number"]})}}
	}
	return nil
}

// leftOn is the day a change to a position's holding leaves it, if it does: the new end of a holding cut short, or
// today for one removed while it held.
func leftOn(c Change, today string) string {
	switch c.Operation {
	case "update":
		before, _ := c.Before["valid_to"].(string)
		after, _ := c.After["valid_to"].(string)
		if after != "" && after != before {
			return after
		}
	case "delete":
		from, _ := c.Before["valid_from"].(string)
		to, _ := c.Before["valid_to"].(string)
		if from <= today && (to == "" || to > today) {
			return today
		}
	}
	return ""
}
