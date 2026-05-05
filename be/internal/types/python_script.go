package types

// PythonScriptCreateRequest is the request for creating a Python script
type PythonScriptCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Code        string `json:"code,omitempty"`
}

// PythonScriptUpdateRequest is the request for updating a Python script
type PythonScriptUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Code        *string `json:"code,omitempty"`
}

// ValidatePythonScriptRequest is the request for validating Python code syntax
type ValidatePythonScriptRequest struct {
	Code string `json:"code"`
}
