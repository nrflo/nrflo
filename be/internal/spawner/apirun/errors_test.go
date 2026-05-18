package apirun

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// makeSDKErr constructs a *sdk.Error with the given HTTP status code. When
// errType is non-empty, the error type field is populated via JSON unmarshal
// (sdk.Error.Type() will return errType). Request and Response are populated
// so that sdk.Error.Error() does not panic.
func makeSDKErr(statusCode int, errType sdk.ErrorType) *sdk.Error {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	resp := &http.Response{StatusCode: statusCode}
	apiErr := &sdk.Error{
		StatusCode: statusCode,
		Request:    req,
		Response:   resp,
	}
	if errType != "" {
		body := `{"error":{"type":"` + string(errType) + `","message":"test error"}}`
		_ = json.Unmarshal([]byte(body), apiErr)
	}
	return apiErr
}

func TestClassifyProviderError(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name          string
		ctx           context.Context
		err           error
		wantStatus    string
		wantClass     RetryClass
		wantMsgSubstr string
	}{
		// Type()-based rate-limit detection: 200 OK response with rate_limit_error type.
		// If Type() detection is broken, StatusCode=200 falls to FAIL (not RATE_LIMITED).
		{
			name:          "sdk_rate_limit_type_200",
			ctx:           context.Background(),
			err:           makeSDKErr(200, sdk.ErrorTypeRateLimitError),
			wantStatus:    "RATE_LIMITED",
			wantClass:     RetryClassRateLimit,
			wantMsgSubstr: "rate_limit",
		},
		// Type()-based overloaded detection on non-429 status.
		{
			name:          "sdk_overloaded_type_200",
			ctx:           context.Background(),
			err:           makeSDKErr(200, sdk.ErrorTypeOverloadedError),
			wantStatus:    "RATE_LIMITED",
			wantClass:     RetryClassRateLimit,
			wantMsgSubstr: "rate_limit",
		},
		// StatusCode fallback: 429 without an explicit error type.
		{
			name:          "http_429_no_type",
			ctx:           context.Background(),
			err:           makeSDKErr(429, ""),
			wantStatus:    "RATE_LIMITED",
			wantClass:     RetryClassRateLimit,
			wantMsgSubstr: "rate_limit",
		},
		// StatusCode fallback: 529 (Anthropic overloaded variant).
		{
			name:          "http_529_no_type",
			ctx:           context.Background(),
			err:           makeSDKErr(529, ""),
			wantStatus:    "RATE_LIMITED",
			wantClass:     RetryClassRateLimit,
			wantMsgSubstr: "rate_limit",
		},
		{
			name:          "http_401",
			ctx:           context.Background(),
			err:           makeSDKErr(401, ""),
			wantStatus:    "FAIL",
			wantClass:     RetryClassError,
			wantMsgSubstr: "auth_error",
		},
		{
			name:          "http_403",
			ctx:           context.Background(),
			err:           makeSDKErr(403, ""),
			wantStatus:    "FAIL",
			wantClass:     RetryClassError,
			wantMsgSubstr: "auth_error",
		},
		{
			name:          "http_500",
			ctx:           context.Background(),
			err:           makeSDKErr(500, ""),
			wantStatus:    "FAIL",
			wantClass:     RetryClassError,
			wantMsgSubstr: "provider_error",
		},
		// json.SyntaxError maps to protocol error.
		{
			name:          "json_syntax_error",
			ctx:           context.Background(),
			err:           &json.SyntaxError{},
			wantStatus:    "FAIL",
			wantClass:     RetryClassError,
			wantMsgSubstr: "provider_protocol_error",
		},
		// json.UnmarshalTypeError also maps to protocol error.
		{
			name:          "json_unmarshal_type_error",
			ctx:           context.Background(),
			err:           &json.UnmarshalTypeError{Value: "string", Type: reflect.TypeOf(0)},
			wantStatus:    "FAIL",
			wantClass:     RetryClassError,
			wantMsgSubstr: "provider_protocol_error",
		},
		// Generic error: FAIL with RetryClassNone.
		{
			name:          "generic_error",
			ctx:           context.Background(),
			err:           errors.New("boom"),
			wantStatus:    "FAIL",
			wantClass:     RetryClassNone,
			wantMsgSubstr: "provider_error",
		},
		// Cancelled context: takes priority over the error.
		{
			name:          "cancelled_ctx",
			ctx:           cancelledCtx,
			err:           errors.New("irrelevant"),
			wantStatus:    "CANCELLED",
			wantClass:     RetryClassNone,
			wantMsgSubstr: "cancelled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg, class := classifyProviderError(tc.ctx, tc.err)
			if status != tc.wantStatus {
				t.Errorf("status = %q, want %q", status, tc.wantStatus)
			}
			if class != tc.wantClass {
				t.Errorf("class = %v, want %v", class, tc.wantClass)
			}
			if tc.wantMsgSubstr != "" && !strings.Contains(msg, tc.wantMsgSubstr) {
				t.Errorf("msg = %q, want to contain %q", msg, tc.wantMsgSubstr)
			}
		})
	}
}
