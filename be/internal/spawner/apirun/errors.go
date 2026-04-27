package apirun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// classifyProviderError categorises a provider error and returns the agent
// final status ("FAIL" or "CANCELLED") and a human-readable system message.
func classifyProviderError(ctx context.Context, err error) (status, message string) {
	if ctx.Err() != nil {
		return "CANCELLED", fmt.Sprintf("cancelled: %s", ctx.Err().Error())
	}

	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
			return "FAIL", fmt.Sprintf("auth_error: %s", apiErr.Error())
		case apiErr.StatusCode == 429:
			return "FAIL", fmt.Sprintf("rate_limit: %s", apiErr.Error())
		case apiErr.StatusCode >= 500 && apiErr.StatusCode < 600:
			return "FAIL", fmt.Sprintf("provider_error: %s", apiErr.Error())
		default:
			return "FAIL", fmt.Sprintf("provider_error: %s", apiErr.Error())
		}
	}

	var jsonErr *json.SyntaxError
	if errors.As(err, &jsonErr) {
		return "FAIL", fmt.Sprintf("provider_protocol_error: %s", err.Error())
	}
	var unmarshalErr *json.UnmarshalTypeError
	if errors.As(err, &unmarshalErr) {
		return "FAIL", fmt.Sprintf("provider_protocol_error: %s", err.Error())
	}

	return "FAIL", fmt.Sprintf("provider_error: %s", err.Error())
}
