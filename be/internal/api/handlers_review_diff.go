package api

import "encoding/json"

// diffResult holds the key-by-key comparison of two JSON objects.
type diffResult struct {
	Added   map[string]interface{} `json:"added"`
	Removed map[string]interface{} `json:"removed"`
	Changed map[string]interface{} `json:"changed"`
}

// diffJSON compares input and draft as JSON objects and returns added/removed/changed keys.
// Returns nil if either argument is not valid JSON or not a JSON object.
func diffJSON(input, draft string) *diffResult {
	var a, b map[string]interface{}
	if err := json.Unmarshal([]byte(input), &a); err != nil {
		return nil
	}
	if err := json.Unmarshal([]byte(draft), &b); err != nil {
		return nil
	}

	result := &diffResult{
		Added:   make(map[string]interface{}),
		Removed: make(map[string]interface{}),
		Changed: make(map[string]interface{}),
	}

	for k, bv := range b {
		if av, ok := a[k]; !ok {
			result.Added[k] = bv
		} else if toJSON(av) != toJSON(bv) {
			result.Changed[k] = map[string]interface{}{"from": av, "to": bv}
		}
	}
	for k, av := range a {
		if _, ok := b[k]; !ok {
			result.Removed[k] = av
		}
	}
	return result
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
