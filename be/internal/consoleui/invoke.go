package consoleui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// invokeQuery mirrors slashQuery for the "/invoke " directive: the draft
// must start with the literal "/invoke " prefix (trailing space required —
// bare "/invoke" still falls through to slashQuery/skill filtering) and stay
// on a single line. The remainder is the tool-name filter.
func invokeQuery(value string) (string, bool) {
	const prefix = "/invoke "
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	if strings.Contains(value, "\n") {
		return "", false
	}
	return value[len(prefix):], true
}

// argField is one prompt-able input for an invoked tool, derived from its
// JSON Schema input_schema.
type argField struct {
	Name     string
	Type     string
	Required bool
	Default  string
	Scalar   bool
}

type schemaDoc struct {
	Properties map[string]schemaProp `json:"properties"`
	Required   []string              `json:"required"`
}

type schemaProp struct {
	Type    string          `json:"type"`
	Default json.RawMessage `json:"default"`
}

// toolArgFields parses a tool's input_schema into an ordered list of
// prompt-able fields: required properties first (in the schema's `required`
// array order), then the remaining properties sorted alphabetically. Scalar
// reports whether the field's declared type is a JSON Schema scalar
// (string/number/integer/boolean); an empty/unparseable schema yields nil.
func toolArgFields(schema json.RawMessage) []argField {
	if len(strings.TrimSpace(string(schema))) == 0 {
		return nil
	}
	var doc schemaDoc
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil
	}

	seen := make(map[string]bool, len(doc.Properties))
	fields := make([]argField, 0, len(doc.Properties))
	for _, name := range doc.Required {
		prop, ok := doc.Properties[name]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		fields = append(fields, buildArgField(name, prop, true))
	}

	remaining := make([]string, 0, len(doc.Properties))
	for name := range doc.Properties {
		if seen[name] {
			continue
		}
		remaining = append(remaining, name)
	}
	sort.Strings(remaining)
	for _, name := range remaining {
		fields = append(fields, buildArgField(name, doc.Properties[name], false))
	}
	return fields
}

func buildArgField(name string, prop schemaProp, required bool) argField {
	def := ""
	if len(prop.Default) > 0 {
		def = defaultString(prop.Default)
	}
	return argField{Name: name, Type: prop.Type, Required: required, Default: def, Scalar: isScalarType(prop.Type)}
}

func isScalarType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean":
		return true
	}
	return false
}

func defaultString(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// invokePhase tracks which step of the /invoke flow is active.
type invokePhase int

const (
	invokePhaseArgs invokePhase = iota
	invokePhaseConfirm
)

// invokeState is the pure state machine backing the /invoke composer flow:
// no *model, no terminal, so it's fully unit-testable. Zero value is the
// inactive state.
type invokeState struct {
	active bool
	tool   string
	fields []argField
	index  int
	values map[string]string
	inform bool
	phase  invokePhase
}

// startInvoke begins invoking tool by name with the given (already parsed)
// arg fields: phase is args when there are fields to prompt for, otherwise
// it jumps straight to confirm. inform defaults to true.
func startInvoke(tool string, fields []argField) invokeState {
	st := invokeState{active: true, tool: tool, fields: fields, values: make(map[string]string), inform: true}
	if len(fields) == 0 {
		st.phase = invokePhaseConfirm
	} else {
		st.phase = invokePhaseArgs
	}
	return st
}

// acceptArg stores value for the current field and advances to the next one,
// moving to the confirm phase once the last field is accepted. No-op outside
// the args phase or past the last field.
func acceptArg(st invokeState, value string) invokeState {
	if st.phase != invokePhaseArgs || st.index < 0 || st.index >= len(st.fields) {
		return st
	}
	values := make(map[string]string, len(st.values)+1)
	for k, v := range st.values {
		values[k] = v
	}
	values[st.fields[st.index].Name] = value
	st.values = values
	st.index++
	if st.index >= len(st.fields) {
		st.phase = invokePhaseConfirm
	}
	return st
}

// cancelInvoke resets to the zero (inactive) state.
func cancelInvoke() invokeState {
	return invokeState{}
}

// toggleInform flips the inform-model flag; a no-op outside confirm phase is
// left to the caller (the key handler only wires it in confirm phase).
func toggleInform(st invokeState) invokeState {
	st.inform = !st.inform
	return st
}

// buildInvokeArguments assembles the JSON body for POST .../invoke from the
// field definitions and collected values: strings pass through as JSON
// strings, number/integer/boolean values parse into their native JSON types
// (falling back to a string on parse failure), and empty optional fields are
// omitted entirely. Required fields are always included, even when empty.
func buildInvokeArguments(fields []argField, values map[string]string) json.RawMessage {
	args := make(map[string]any, len(fields))
	for _, f := range fields {
		v, ok := values[f.Name]
		if !ok {
			continue
		}
		if v == "" && !f.Required {
			continue
		}
		args[f.Name] = typedArgValue(f.Type, v)
	}
	data, err := json.Marshal(args)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func typedArgValue(fieldType, v string) any {
	switch fieldType {
	case "number":
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	case "integer":
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	case "boolean":
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	case "string":
		return v
	default:
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return parsed
		}
	}
	return v
}
