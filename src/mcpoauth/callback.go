package mcpoauth

import (
	"net/url"
	"strings"
)

type CallbackParams struct {
	Code             string
	State            string
	Error            string
	ErrorDescription string
	ErrorURI         string
}

type CallbackResult struct {
	Code  string
	State string
}

func ParseCallbackValues(values url.Values) CallbackParams {
	return CallbackParams{
		Code:             strings.TrimSpace(values.Get("code")),
		State:            strings.TrimSpace(values.Get("state")),
		Error:            strings.TrimSpace(values.Get("error")),
		ErrorDescription: strings.TrimSpace(values.Get("error_description")),
		ErrorURI:         strings.TrimSpace(values.Get("error_uri")),
	}
}

func ValidateCallback(session AuthSession, callback CallbackParams) (CallbackResult, error) {
	if callback.Error != "" {
		return CallbackResult{}, CallbackError{
			Code:        callback.Error,
			Description: callback.ErrorDescription,
			URI:         callback.ErrorURI,
		}
	}
	if callback.State == "" {
		return CallbackResult{}, ErrMissingCallbackState
	}
	if callback.State != session.State {
		return CallbackResult{}, ErrStateMismatch
	}
	if callback.Code == "" {
		return CallbackResult{}, ErrMissingAuthorizationCode
	}
	return CallbackResult{
		Code:  callback.Code,
		State: callback.State,
	}, nil
}
