package types

// PythonScriptCreateRequest is the request for creating a Python script or api-mode tool.
// Kind must be "agent" or "tool"; tool rows require ToolDescription and a valid InputSchema JSON Schema.
type PythonScriptCreateRequest struct {
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Description     string  `json:"description,omitempty"`
	Code            string  `json:"code,omitempty"`
	FilePath        *string `json:"file_path,omitempty"`
	ToolDescription string  `json:"tool_description,omitempty"`
	InputSchema     string  `json:"input_schema,omitempty"`
	TimeoutSec      int     `json:"timeout_sec,omitempty"`
}

// PythonScriptUpdateRequest is the request for updating a Python script.
// Kind is immutable and may not be changed after creation.
type PythonScriptUpdateRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Code            *string `json:"code,omitempty"`
	FilePath        *string `json:"file_path,omitempty"`
	ToolDescription *string `json:"tool_description,omitempty"`
	InputSchema     *string `json:"input_schema,omitempty"`
	TimeoutSec      *int    `json:"timeout_sec,omitempty"`
}

// ValidatePythonScriptRequest is the request for validating Python code syntax
type ValidatePythonScriptRequest struct {
	Code string `json:"code"`
}
