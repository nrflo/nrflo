package apirun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// RetryClass categorises whether a provider error is retriable and by which mechanism.
type RetryClass int

const (
	RetryClassNone      RetryClass = 0 // unrecognised or not retriable
	RetryClassRateLimit RetryClass = 1 // rate-limit or server overloaded — exponential backoff
	RetryClassError     RetryClass = 2 // non-retriable provider or auth error
)

// classifyProviderError categorises a provider error and returns the agent
// final status, a human-readable system message, and a RetryClass.
func classifyProviderError(ctx context.Context, err error) (status, message string, class RetryClass) {
	if ctx.Err() != nil {
		return "CANCELLED", fmt.Sprintf("cancelled: %s", ctx.Err().Error()), RetryClassNone
	}

	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		// Type-based detection takes precedence over StatusCode for rate-limit and overloaded.
		switch apiErr.Type() {
		case sdk.ErrorTypeRateLimitError, sdk.ErrorTypeOverloadedError:
			return "RATE_LIMITED", fmt.Sprintf("rate_limit: %s", apiErr.Error()), RetryClassRateLimit
		}
		switch {
		case apiErr.StatusCode == 429 || apiErr.StatusCode == 529:
			return "RATE_LIMITED", fmt.Sprintf("rate_limit: %s", apiErr.Error()), RetryClassRateLimit
		case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
			return "FAIL", fmt.Sprintf("auth_error: %s", apiErr.Error()), RetryClassError
		case apiErr.StatusCode >= 500 && apiErr.StatusCode < 600:
			return "FAIL", fmt.Sprintf("provider_error: %s", apiErr.Error()), RetryClassError
		default:
			return "FAIL", fmt.Sprintf("provider_error: %s", apiErr.Error()), RetryClassError
		}
	}

	var jsonErr *json.SyntaxError
	if errors.As(err, &jsonErr) {
		return "FAIL", fmt.Sprintf("provider_protocol_error: %s", err.Error()), RetryClassError
	}
	var unmarshalErr *json.UnmarshalTypeError
	if errors.As(err, &unmarshalErr) {
		return "FAIL", fmt.Sprintf("provider_protocol_error: %s", err.Error()), RetryClassError
	}

	return "FAIL", fmt.Sprintf("provider_error: %s", err.Error()), RetryClassNone
}
