package service

import "encoding/json"

// maskConfig replaces secret fields in the stored config JSON with masked values.
func maskConfig(kind, configJSON string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return configJSON
	}

	switch kind {
	case "slack":
		if v, ok := m["webhook_url"].(string); ok && v != "" {
			m["webhook_url"] = maskURL(v)
		}
	case "telegram":
		if v, ok := m["bot_token"].(string); ok && v != "" {
			m["bot_token"] = maskToken(v)
		}
		// chat_id passthrough — not secret
	}

	b, err := json.Marshal(m)
	if err != nil {
		return configJSON
	}
	return string(b)
}

// applyConfigPatch merges incoming config onto stored config, preserving secrets
// when the incoming value matches the masked value.
func applyConfigPatch(kind, storedJSON, incomingJSON string) string {
	var stored map[string]interface{}
	var incoming map[string]interface{}
	if err := json.Unmarshal([]byte(storedJSON), &stored); err != nil {
		stored = map[string]interface{}{}
	}
	if err := json.Unmarshal([]byte(incomingJSON), &incoming); err != nil {
		return storedJSON
	}

	masked := maskConfig(kind, storedJSON)
	var maskedMap map[string]interface{}
	_ = json.Unmarshal([]byte(masked), &maskedMap)

	for k, newVal := range incoming {
		newStr, _ := newVal.(string)
		maskedVal, _ := maskedMap[k].(string)
		if newStr != "" && newStr == maskedVal {
			// Incoming value matches masked — preserve stored secret.
			continue
		}
		stored[k] = newVal
	}

	b, err := json.Marshal(stored)
	if err != nil {
		return storedJSON
	}
	return string(b)
}

// maskURL masks the last 4 characters of the URL path segment.
// e.g. https://hooks.slack.com/services/ABC/DEF/GHIJ -> https://hooks.slack.com/services/ABC/DEF/****GHIJ
func maskURL(u string) string {
	if len(u) <= 4 {
		return "****"
	}
	return u[:len(u)-4] + "****"
}

// maskToken masks a token as <first4>****<last4>.
func maskToken(t string) string {
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "****" + t[len(t)-4:]
}
