package providererrors

import "testing"

func TestClassifyHTTPStatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    ClassificationInput
		want     Reason
		retry    bool
		compress bool
		rotate   bool
		fallback bool
	}{
		{
			name:     "401 auth",
			input:    ClassificationInput{StatusCode: 401, Message: "invalid api key"},
			want:     ReasonAuth,
			retry:    false,
			rotate:   true,
			fallback: true,
		},
		{
			name:     "403 auth",
			input:    ClassificationInput{StatusCode: 403, Message: "forbidden"},
			want:     ReasonAuth,
			retry:    false,
			rotate:   true,
			fallback: true,
		},
		{
			name:     "402 billing",
			input:    ClassificationInput{StatusCode: 402, Message: "insufficient credits"},
			want:     ReasonBilling,
			retry:    false,
			rotate:   true,
			fallback: true,
		},
		{
			name:     "404 model not found",
			input:    ClassificationInput{StatusCode: 404, Message: "model not found"},
			want:     ReasonModelNotFound,
			retry:    false,
			fallback: true,
		},
		{
			name:     "413 payload too large",
			input:    ClassificationInput{StatusCode: 413, Message: "payload too large"},
			want:     ReasonPayloadTooLarge,
			retry:    true,
			compress: true,
		},
		{
			name:     "429 rate limit",
			input:    ClassificationInput{StatusCode: 429, Message: "rate limit exceeded"},
			want:     ReasonRateLimit,
			retry:    true,
			rotate:   true,
			fallback: true,
		},
		{
			name:  "500 server error",
			input: ClassificationInput{StatusCode: 500, Message: "internal server error"},
			want:  ReasonServerError,
			retry: true,
		},
		{
			name:  "503 overloaded",
			input: ClassificationInput{StatusCode: 503, Message: "service unavailable"},
			want:  ReasonOverloaded,
			retry: true,
		},
		{
			name:  "529 overloaded",
			input: ClassificationInput{StatusCode: 529, Message: "overloaded"},
			want:  ReasonOverloaded,
			retry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.input)
			assertClassification(t, got, tt.want, tt.retry, tt.compress, tt.rotate, tt.fallback)
		})
	}
}

func TestClassifyOpenRouterPrivacyPolicyBlock(t *testing.T) {
	got := Classify(ClassificationInput{
		StatusCode: 404,
		Provider:   "openrouter",
		Body:       `{"error":{"message":"No endpoints available matching your guardrail restrictions and data policy. Configure: https://openrouter.ai/settings/privacy"}}`,
	})

	assertClassification(t, got, ReasonProviderPolicyBlocked, false, false, false, false)
	if got.Message == "" {
		t.Fatal("expected message extracted from body")
	}
}

func TestClassifyAnthropicThinkingSignature(t *testing.T) {
	got := Classify(ClassificationInput{
		StatusCode: 400,
		Provider:   "anthropic",
		Message:    "The thinking block signature is invalid.",
	})

	assertClassification(t, got, ReasonThinkingSignature, true, false, false, false)
}

func TestClassifyAnthropicLongContextTier(t *testing.T) {
	got := Classify(ClassificationInput{
		StatusCode: 429,
		Provider:   "anthropic",
		Message:    "This request requires extra usage for long context.",
	})

	assertClassification(t, got, ReasonLongContextTier, true, true, false, false)
}

func TestClassifyContextOverflowMessages(t *testing.T) {
	tests := []ClassificationInput{
		{
			StatusCode: 400,
			Message:    "This model's maximum context length is 128000 tokens.",
		},
		{
			Body: `{"error":{"message":"Prompt length 150000 exceeds the max_model_len 131072","code":"bad_request"}}`,
		},
		{
			Message: "上下文长度超过最大长度",
		},
	}

	for _, input := range tests {
		got := Classify(input)
		assertClassification(t, got, ReasonContextOverflow, true, true, false, false)
	}
}

func TestClassifyImageTooLargeBadRequest(t *testing.T) {
	got := Classify(ClassificationInput{
		StatusCode: 400,
		Message:    "messages.0.content.1.image.source.base64: image exceeds 5 MB maximum",
	})

	assertClassification(t, got, ReasonImageTooLarge, true, false, false, false)
}

func TestClassifySSLAlertDiffersFromLargeContextDisconnect(t *testing.T) {
	sslAlert := Classify(ClassificationInput{
		ErrorType:     "SSLError",
		Message:       "[SSL: BAD_RECORD_MAC] SSL alert bad record mac",
		ApproxTokens:  180000,
		ContextLength: 200000,
		MessageCount:  300,
	})
	assertClassification(t, sslAlert, ReasonTimeout, true, false, false, false)

	disconnect := Classify(ClassificationInput{
		ErrorType:     "RemoteProtocolError",
		Message:       "server disconnected without sending a response",
		ApproxTokens:  180000,
		ContextLength: 200000,
		MessageCount:  300,
	})
	assertClassification(t, disconnect, ReasonContextOverflow, true, true, false, false)
}

func TestClassifyErrorCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    ClassificationInput
		want     Reason
		retry    bool
		compress bool
		rotate   bool
		fallback bool
	}{
		{
			name:     "explicit resource exhausted",
			input:    ClassificationInput{ErrorCode: "resource_exhausted"},
			want:     ReasonRateLimit,
			retry:    true,
			rotate:   true,
			fallback: true,
		},
		{
			name:     "explicit context length exceeded",
			input:    ClassificationInput{StatusCode: 400, ErrorCode: "context_length_exceeded"},
			want:     ReasonContextOverflow,
			retry:    true,
			compress: true,
		},
		{
			name:     "body insufficient quota",
			input:    ClassificationInput{Body: `{"error":{"message":"quota exhausted","code":"insufficient_quota"}}`},
			want:     ReasonBilling,
			retry:    false,
			rotate:   true,
			fallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.input)
			assertClassification(t, got, tt.want, tt.retry, tt.compress, tt.rotate, tt.fallback)
		})
	}
}

func TestClassifyRateLimitErrorType(t *testing.T) {
	got := Classify(ClassificationInput{
		ErrorType: "RateLimitError",
		Message:   "provider wrapped rate limit",
	})

	assertClassification(t, got, ReasonRateLimit, true, false, true, true)
}

func TestClassifyTransientUsageLimitAsRateLimit(t *testing.T) {
	got := Classify(ClassificationInput{
		StatusCode: 402,
		Message:    "Usage limit reached, try again in 5 minutes.",
	})

	assertClassification(t, got, ReasonRateLimit, true, false, true, true)
}

func TestClassifyWrappedOpenRouterProviderError(t *testing.T) {
	got := Classify(ClassificationInput{
		Provider: "openrouter",
		Body: `{
			"error": {
				"message": "Provider returned error",
				"metadata": {
					"raw": "{\"error\":{\"message\":\"context length exceeded\",\"code\":\"context_length_exceeded\"}}"
				}
			}
		}`,
	})

	assertClassification(t, got, ReasonContextOverflow, true, true, false, false)
}

func TestClassifyTransportType(t *testing.T) {
	got := Classify(ClassificationInput{
		ErrorType: "APITimeoutError",
		Message:   "request timed out",
	})

	assertClassification(t, got, ReasonTimeout, true, false, false, false)
}

func assertClassification(t *testing.T, got ClassifiedError, reason Reason, retry bool, compress bool, rotate bool, fallback bool) {
	t.Helper()

	if got.Reason != reason {
		t.Fatalf("expected reason %q, got %q with metadata %+v", reason, got.Reason, got.Metadata)
	}
	if got.Retryable != retry {
		t.Fatalf("expected retryable %v, got %v for %+v", retry, got.Retryable, got)
	}
	if got.ShouldCompress != compress {
		t.Fatalf("expected should_compress %v, got %v for %+v", compress, got.ShouldCompress, got)
	}
	if got.ShouldRotateCredential != rotate {
		t.Fatalf("expected should_rotate_credential %v, got %v for %+v", rotate, got.ShouldRotateCredential, got)
	}
	if got.ShouldFallback != fallback {
		t.Fatalf("expected should_fallback %v, got %v for %+v", fallback, got.ShouldFallback, got)
	}
	if got.Metadata == nil || got.Metadata["matched_by"] == "" {
		t.Fatalf("expected matched_by metadata, got %+v", got.Metadata)
	}
}
