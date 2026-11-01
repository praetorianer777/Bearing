package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// toolCall is one call of a tool: who asked, through the request they made, and the arguments.
type toolCall struct {
	s     *Server
	outer *http.Request
	args  map[string]json.RawMessage
}

// mcpTool is one tool an assistant may call. Every tool reads; none changes anything.
type mcpTool struct {
	name, title, description string
	params                   []toolParam
	run                      func(*toolCall) (any, error)
}

// toolParam is one argument a tool takes.
type toolParam struct {
	name, kind, description string
	required                bool
}

// input is the tool's arguments as a JSON schema.
func (t mcpTool) input() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range t.params {
		props[p.name] = map[string]any{"type": p.kind, "description": p.description}
		if p.required {
			required = append(required, p.name)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

var (
	asOfParam   = toolParam{name: "asOf", kind: "string", description: "The day to answer for, written 2006-01-02; today when left out."}
	dottedParam = toolParam{name: "dotted", kind: "boolean", description: "Follow dotted reporting lines as well as solid ones."}
)

func idParam(name, of string, required bool) toolParam {
	return toolParam{name: name, kind: "string", description: "The id of the " + of + ", as search finds it.", required: required}
}

// str is a string argument, empty when it is left out.
func (c *toolCall) str(name string) (string, error) {
	raw, ok := c.args[name]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("%s is a string", name)
	}
	return strings.TrimSpace(v), nil
}

// flag is a boolean argument as a query parameter: "true", or empty when it is left out or false.
func (c *toolCall) flag(name string) (string, error) {
	raw, ok := c.args[name]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("%s is true or false", name)
	}
	if v {
		return "true", nil
	}
	return "", nil
}

// id is an id argument, checked to be one.
func (c *toolCall) id(name string) (string, error) {
	v, err := c.str(name)
	if err != nil || v == "" {
		return v, err
	}
	if _, err := uuid.Parse(v); err != nil {
		return "", fmt.Errorf("%s is an id such as search answers, not %q", name, v)
	}
	return v, nil
}

// known checks that only the tool's own arguments were given, and that the required ones were.
func (c *toolCall) known(t mcpTool) error {
	for name := range c.args {
		found := false
		for _, p := range t.params {
			found = found || p.name == name
		}
		if !found {
			return fmt.Errorf("%s takes no argument %s", t.name, name)
		}
	}
	for _, p := range t.params {
		if _, ok := c.args[p.name]; p.required && !ok {
			return fmt.Errorf("%s needs %s", t.name, p.name)
		}
	}
	return nil
}

// oneOf is the one id argument of several that is given, and its name; exactly one must be.
func (c *toolCall) oneOf(names ...string) (string, string, error) {
	var name, value string
	for _, n := range names {
		v, err := c.id(n)
		if err != nil {
			return "", "", err
		}
		if v == "" {
			continue
		}
		if value != "" {
			return "", "", errors.New("name one of " + strings.Join(names, ", ") + ", not several")
		}
		name, value = n, v
	}
	if value == "" {
		return "", "", errors.New("name one of " + strings.Join(names, ", "))
	}
	return name, value, nil
}

// searchLimit bounds the hits of each kind search answers.
const searchLimit = 20

var mcpTools = []mcpTool{
	{name: "whoami", title: "Who am I",
		description: "The account the assistant acts for, with its role, and the person in the organisation it is linked to, with what they hold.",
		params:      []toolParam{asOfParam},
		run: func(c *toolCall) (any, error) {
			asOf, err := c.str("asOf")
			if err != nil {
				return nil, err
			}
			me, err := c.get("/auth/me", nil)
			if err != nil {
				return nil, err
			}
			entry, err := c.get("/people/me", map[string]string{"asOf": asOf})
			var notFound *toolError
			if errors.As(err, &notFound) && notFound.status == http.StatusNotFound {
				return map[string]any{"account": me["account"], "person": nil}, nil
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"account": me["account"], "person": entry}, nil
		}},
	{name: "search", title: "Search the organisation",
		description: "Find people, positions, units, roles, processes and process steps by the start of their words, ignoring case and accents, best first, with their ids for the other tools. A step's id is its process's, with its stepKey.",
		params:      []toolParam{{name: "query", kind: "string", description: "A word or the start of one, such as invoice or Müller.", required: true}, asOfParam},
		run: func(c *toolCall) (any, error) {
			query, err := c.str("query")
			if err != nil {
				return nil, err
			}
			if len([]rune(strings.TrimSpace(query))) < 2 {
				return nil, errors.New("search for two letters or more")
			}
			asOf, err := c.str("asOf")
			if err != nil {
				return nil, err
			}
			return c.get("/search", map[string]string{"q": query, "asOf": asOf, "limit": strconv.Itoa(searchLimit)})
		}},
	{name: "get_person", title: "A person",
		description: "One person, with the positions and roles they hold on a day.",
		params:      []toolParam{idParam("personId", "person", true), asOfParam},
		run:         byID("personId", "/people/%s")},
	{name: "get_unit", title: "A unit",
		description: "One unit as it stands on a day, with who holds a position in it; with subUnits, in the units inside it too.",
		params: []toolParam{idParam("unitId", "unit", true), asOfParam,
			{name: "subUnits", kind: "boolean", description: "Count in the people of the units inside it too."}},
		run: func(c *toolCall) (any, error) {
			id, err := c.id("unitId")
			if err != nil {
				return nil, err
			}
			asOf, err := c.str("asOf")
			if err != nil {
				return nil, err
			}
			sub, err := c.flag("subUnits")
			if err != nil {
				return nil, err
			}
			unit, err := c.get("/units/"+url.PathEscape(id), map[string]string{"asOf": asOf})
			if err != nil {
				return nil, err
			}
			members, err := c.get("/units/"+url.PathEscape(id)+"/members", map[string]string{"asOf": asOf, "subUnits": sub})
			if err != nil {
				return nil, err
			}
			unit["members"] = members["members"]
			return unit, nil
		}},
	{name: "get_position", title: "A position",
		description: "One position as it stands on a day: its unit, whom it reports to, its holders and deputies.",
		params:      []toolParam{idParam("positionId", "position", true), asOfParam},
		run:         byID("positionId", "/positions/%s")},
	{name: "management_chain", title: "Management chain",
		description: "Whom a person or a position reports to on a day, nearest first, up to the top, each with its holders.",
		params:      []toolParam{idParam("personId", "person", false), idParam("positionId", "position", false), asOfParam, dottedParam},
		run:         hierarchy("chain")},
	{name: "reports", title: "Reports",
		description: "Who reports to a person or a position on a day: the direct reports, or with all every one below, nearest first.",
		params: []toolParam{idParam("personId", "person", false), idParam("positionId", "position", false), asOfParam, dottedParam,
			{name: "all", kind: "boolean", description: "Every position below, not only the direct reports."}},
		run: hierarchy("reports")},
	{name: "responsibilities_of", title: "Responsibilities of",
		description: "What a person, position, role or unit is responsible, accountable, consulted or informed for on a day, step by step across the newest published version of every process.",
		params: []toolParam{idParam("personId", "person", false), idParam("positionId", "position", false), idParam("roleId", "role", false),
			idParam("unitId", "unit", false), asOfParam},
		run: func(c *toolCall) (any, error) {
			name, id, err := c.oneOf("personId", "positionId", "roleId", "unitId")
			if err != nil {
				return nil, err
			}
			asOf, err := c.str("asOf")
			if err != nil {
				return nil, err
			}
			return c.get("/responsibilities", map[string]string{name: id, "asOf": asOf})
		}},
	{name: "who_is_responsible", title: "Who is responsible",
		description: "The steps of published processes whose name, or whose process's name, holds a word, with who is responsible, accountable, consulted and informed for each on a day, down to the people, deputies standing in.",
		params:      []toolParam{{name: "query", kind: "string", description: "A word of the step or the process, such as invoice or approve.", required: true}, asOfParam},
		run: func(c *toolCall) (any, error) {
			query, err := c.str("query")
			if err != nil {
				return nil, err
			}
			if len([]rune(query)) < 2 {
				return nil, errors.New("ask with two letters or more")
			}
			asOf, err := c.str("asOf")
			if err != nil {
				return nil, err
			}
			return c.get("/steps", map[string]string{"q": query, "asOf": asOf})
		}},
	{name: "get_process", title: "A process",
		description: "A process: its newest published version, or another by number, with its steps, decisions and arrows, and who each step's responsibilities come down to on a day.",
		params: []toolParam{idParam("processId", "process", true), asOfParam,
			{name: "version", kind: "integer", description: "The published version to read; the newest when left out."}},
		run: func(c *toolCall) (any, error) {
			id, err := c.id("processId")
			if err != nil {
				return nil, err
			}
			asOf, err := c.str("asOf")
			if err != nil {
				return nil, err
			}
			var version string
			if raw, ok := c.args["version"]; ok && string(raw) != "null" {
				var n int
				if err := json.Unmarshal(raw, &n); err != nil || n < 1 {
					return nil, errors.New("version is a whole number from 1")
				}
				version = strconv.Itoa(n)
			}
			process, err := c.get("/processes/"+url.PathEscape(id), nil)
			if err != nil {
				return nil, err
			}
			p, _ := process["process"].(map[string]any)
			out := map[string]any{"id": p["id"], "key": p["key"], "name": p["name"], "area": p["area"], "description": p["description"],
				"ownerId": p["ownerId"], "versions": p["versions"]}
			if version == "" {
				if latest, ok := p["latest"].(float64); ok {
					version = strconv.Itoa(int(latest))
				}
			}
			if version == "" {
				out["published"] = nil
				return out, nil
			}
			graph, err := c.get("/processes/"+url.PathEscape(id)+"/versions/"+version, nil)
			if err != nil {
				return nil, err
			}
			people, err := c.get("/processes/"+url.PathEscape(id)+"/people", map[string]string{"asOf": asOf, "version": version})
			if err != nil {
				return nil, err
			}
			out["published"] = graph["version"]
			out["asOf"] = people["asOf"]
			out["people"] = people["steps"]
			return out, nil
		}},
}

// byID is a tool that reads one thing by its id on a day.
func byID(param, path string) func(*toolCall) (any, error) {
	return func(c *toolCall) (any, error) {
		id, err := c.id(param)
		if err != nil {
			return nil, err
		}
		asOf, err := c.str("asOf")
		if err != nil {
			return nil, err
		}
		return c.get(fmt.Sprintf(path, url.PathEscape(id)), map[string]string{"asOf": asOf})
	}
}

// hierarchy is a tool that walks the reporting lines from a person or a position: chain up, reports down.
func hierarchy(way string) func(*toolCall) (any, error) {
	return func(c *toolCall) (any, error) {
		name, id, err := c.oneOf("personId", "positionId")
		if err != nil {
			return nil, err
		}
		asOf, err := c.str("asOf")
		if err != nil {
			return nil, err
		}
		dotted, err := c.flag("dotted")
		if err != nil {
			return nil, err
		}
		all := ""
		if way == "reports" {
			if all, err = c.flag("all"); err != nil {
				return nil, err
			}
		}
		base := "/people/"
		if name == "positionId" {
			base = "/positions/"
		}
		return c.get(base+url.PathEscape(id)+"/"+way, map[string]string{"asOf": asOf, "dotted": dotted, "all": all})
	}
}
