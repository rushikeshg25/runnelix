// Package definition validates workflow data and compiles deterministic plans.
package definition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	product "github.com/rushikeshg25/runnelix/contracts/product"
	"github.com/rushikeshg25/runnelix/internal/mapping"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const MaxBytes = 256 * 1024

type Node struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Version int            `json:"version"`
	Config  map[string]any `json:"config"`
}
type Endpoint struct {
	Node string `json:"node"`
	Port string `json:"port"`
}
type Edge struct {
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`
}
type Graph struct {
	FormatVersion int    `json:"formatVersion"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}
type Descriptor struct {
	HTTPPolicy           *HTTPPolicy    `json:"httpPolicy,omitempty"`
	Type                 string         `json:"type"`
	Version              int            `json:"version"`
	Kind                 string         `json:"kind"`
	Title                string         `json:"title"`
	Inputs               []string       `json:"inputs"`
	Outputs              []string       `json:"outputs"`
	ImplementationDigest string         `json:"implementationDigest"`
	ConfigSchema         map[string]any `json:"configSchema"`
	InputSchema          map[string]any `json:"inputSchema"`
	OutputSchema         map[string]any `json:"outputSchema"`
	RetrySafety          string         `json:"retrySafety"`
}
type Catalog []Descriptor
type Step struct {
	Body        *Step  `json:"body,omitempty"`
	Kind        string `json:"kind,omitempty"`
	RetrySafety string `json:"retrySafety,omitempty"`
	Node
	ImplementationDigest string `json:"implementationDigest"`
}
type Plan struct {
	FormatVersion      int    `json:"formatVersion"`
	InterpreterVersion int    `json:"interpreterVersion"`
	Steps              []Step `json:"steps"`
	Edges              []Edge `json:"edges"`
}
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Invalid struct {
	Issues []Issue `json:"issues"`
}

func (e *Invalid) Error() string { return "workflow validation failed" }

// Decode rejects ambiguous duplicate keys, excessive depth, and nonportable numbers.
func Decode(data []byte, target any) error {
	if len(data) > MaxBytes {
		return fmt.Errorf("JSON exceeds byte limit")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := walk(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(target)
}
func walk(d *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("JSON exceeds depth limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch v := token.(type) {
	case json.Delim:
		switch v {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				k, ok := key.(string)
				if !ok || keys[k] {
					return fmt.Errorf("duplicate JSON field")
				}
				keys[k] = true
				if e = walk(d, depth+1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e := walk(d, depth+1); e != nil {
					return e
				}
			}
		default:
			return fmt.Errorf("invalid JSON structure")
		}
		_, err = d.Token()
		return err
	case json.Number:
		f, e := v.Float64()
		if e != nil || f > 9007199254740991 || f < -9007199254740991 {
			return fmt.Errorf("number exceeds portable range")
		}
	}
	return nil
}
func Schema(name string, value any) error {
	data, err := product.Files.ReadFile(name)
	if err != nil {
		return err
	}
	var resource any
	if err = json.Unmarshal(data, &resource); err != nil {
		return err
	}
	return ValidateSchema(resource, value)
}
func ValidateSchema(schema, value any) error {
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", schema); err != nil {
		return err
	}
	s, err := c.Compile("schema.json")
	if err != nil {
		return err
	}
	return s.Validate(value)
}

// SchemaError exposes field paths without echoing values or schema error details.
func SchemaError(err error, prefix string) *Invalid {
	issues := []Issue{}
	var validation *jsonschema.ValidationError
	if errors.As(err, &validation) {
		var visit func(*jsonschema.ValidationError)
		visit = func(v *jsonschema.ValidationError) {
			if len(issues) >= 10 {
				return
			}
			if len(v.Causes) > 0 {
				for _, c := range v.Causes {
					visit(c)
				}
				return
			}
			path := prefix
			for _, part := range v.InstanceLocation {
				path += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
			}
			if path == "" {
				path = "/"
			}
			issues = append(issues, Issue{path, "schema", "Value does not match the declared schema"})
		}
		visit(validation)
	}
	if len(issues) == 0 {
		issues = append(issues, Issue{prefix, "schema", "Invalid schema or value"})
	}
	return &Invalid{issues}
}
func (c Catalog) Find(kind string, version int) (Descriptor, bool) {
	for _, d := range c {
		if d.Type == kind && d.Version == version {
			return d, true
		}
	}
	return Descriptor{}, false
}
func Compile(data []byte, catalog Catalog) (Graph, Plan, string, error) {
	var g Graph
	var raw any
	bad := func(path, code, msg string) (Graph, Plan, string, error) {
		return Graph{}, Plan{}, "", &Invalid{[]Issue{{path, code, msg}}}
	}
	if err := Decode(data, &raw); err != nil {
		return bad("/", "invalid_json", err.Error())
	}
	if err := Schema("workflow.schema.json", raw); err != nil {
		return Graph{}, Plan{}, "", SchemaError(err, "")
	}
	if err := Decode(data, &g); err != nil {
		return bad("/", "invalid_json", "Invalid workflow fields")
	}
	nodes := map[string]Node{}
	descriptors := map[string]Descriptor{}
	incoming := map[string]int{}
	adjacency := map[string][]string{}
	ports := map[Endpoint]bool{}
	triggers := []string{}
	for i, n := range g.Nodes {
		path := fmt.Sprintf("/nodes/%d", i)
		if _, ok := nodes[n.ID]; ok {
			return bad(path+"/id", "duplicate_id", "Node IDs must be unique")
		}
		d, ok := catalog.Find(n.Type, n.Version)
		if !ok {
			return bad(path+"/type", "unknown_type", "Node type or version is unavailable")
		}
		if d.Kind != "trigger" && d.Kind != "action" && !(d.Kind == "control" && (d.Type == "transform.map" || d.Type == "control.condition" || d.Type == "control.merge" || d.Type == "control.wait" || d.Type == "control.map")) {
			return bad(path, "unsupported_kind", "Control nodes are not supported yet")
		}
		if d.ImplementationDigest == "" {
			return bad(path, "unavailable_artifact", "Node has no pinned implementation")
		}
		if err := ValidateSchema(d.ConfigSchema, n.Config); err != nil {
			return bad(path+"/config", "invalid_config", "Configuration does not match the node schema")
		}
		if d.Type == "transform.map" || d.Type == "control.condition" {
			expression, _ := n.Config["expression"].(string)
			if mapping.Validate(expression) != nil {
				return bad(path+"/config/expression", "invalid_expression", "Expression is invalid or exceeds limits")
			}
		}
		if d.Type == "control.map" {
			expression, _ := n.Config["collectionExpression"].(string)
			if mapping.Validate(expression) != nil {
				return bad(path+"/config/collectionExpression", "invalid_expression", "Invalid collection expression")
			}
			typ, _ := n.Config["bodyType"].(string)
			version, _ := n.Config["bodyVersion"].(float64)
			config, _ := n.Config["bodyConfig"].(map[string]any)
			body, ok := catalog.Find(typ, int(version))
			if !ok || body.Kind != "action" || ValidateSchema(body.ConfigSchema, config) != nil {
				return bad(path+"/config", "invalid_map_body", "Map body must be one available action with valid configuration")
			}
		}
		nodes[n.ID] = n
		descriptors[n.ID] = d
		if d.Kind == "trigger" {
			triggers = append(triggers, n.ID)
		}
	}
	if len(triggers) != 1 {
		return bad("/nodes", "trigger_count", "Exactly one trigger is required")
	}
	contains := func(values []string, s string) bool {
		for _, v := range values {
			if v == s {
				return true
			}
		}
		return false
	}
	for i, e := range g.Edges {
		path := fmt.Sprintf("/edges/%d", i)
		from, ok1 := descriptors[e.From.Node]
		to, ok2 := descriptors[e.To.Node]
		if !ok1 || !ok2 {
			return bad(path, "dangling_edge", "Both edge nodes must exist")
		}
		if !contains(from.Outputs, e.From.Port) || !contains(to.Inputs, e.To.Port) {
			return bad(path, "unknown_port", "Edge uses an unknown port")
		}
		if ports[e.To] {
			return bad(path, "multiple_inputs", "An input port accepts one edge")
		}
		ports[e.To] = true
		incoming[e.To.Node]++
		adjacency[e.From.Node] = append(adjacency[e.From.Node], e.To.Node)
	}
	for _, n := range g.Nodes {
		for _, port := range descriptors[n.ID].Inputs {
			if !ports[Endpoint{n.ID, port}] {
				return bad("/nodes/"+n.ID, "missing_input", "A required input port is not connected")
			}
		}
	}
	ready := []string{}
	for id := range nodes {
		if incoming[id] == 0 {
			ready = append(ready, id)
		}
	}
	p := Plan{FormatVersion: 1, InterpreterVersion: 3, Steps: []Step{}, Edges: append([]Edge{}, g.Edges...)}
	for len(ready) > 0 {
		sort.Strings(ready)
		id := ready[0]
		ready = ready[1:]
		step := Step{Node: nodes[id], ImplementationDigest: descriptors[id].ImplementationDigest, Kind: descriptors[id].Kind, RetrySafety: descriptors[id].RetrySafety}
		if step.Type == "control.map" {
			version := int(step.Config["bodyVersion"].(float64))
			typ := step.Config["bodyType"].(string)
			body, _ := catalog.Find(typ, version)
			step.Body = &Step{Node: Node{ID: id, Type: typ, Version: version, Config: step.Config["bodyConfig"].(map[string]any)}, Kind: "action", RetrySafety: body.RetrySafety, ImplementationDigest: body.ImplementationDigest}
		}
		p.Steps = append(p.Steps, step)
		for _, next := range adjacency[id] {
			incoming[next]--
			if incoming[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	if len(p.Steps) != len(nodes) {
		return bad("/edges", "cycle", "Graph contains a cycle")
	}
	reached := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if reached[id] {
			return
		}
		reached[id] = true
		for _, next := range adjacency[id] {
			visit(next)
		}
	}
	visit(triggers[0])
	if len(reached) != len(nodes) {
		return bad("/nodes", "unreachable", "Every node must be reachable from the trigger")
	}
	sort.Slice(p.Edges, func(i, j int) bool {
		a, _ := json.Marshal(p.Edges[i])
		b, _ := json.Marshal(p.Edges[j])
		return string(a) < string(b)
	})
	encoded, err := json.Marshal(p)
	if err != nil {
		return Graph{}, Plan{}, "", err
	}
	sum := sha256.Sum256(encoded)
	return g, p, hex.EncodeToString(sum[:]), nil
}
