package mcpoauth

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidMetadata          = errors.New("invalid oauth server metadata")
	ErrInvalidClient            = errors.New("invalid oauth client")
	ErrInvalidAuthSession       = errors.New("invalid oauth auth session")
	ErrStateMismatch            = errors.New("oauth state mismatch")
	ErrMissingAuthorizationCode = errors.New("oauth callback missing authorization code")
	ErrMissingCallbackState     = errors.New("oauth callback missing state")
	ErrSessionAlreadyExists     = errors.New("oauth auth session already exists")
	ErrSessionExpired           = errors.New("oauth auth session expired")
)

type CallbackError struct {
	Code        string
	Description string
	URI         string
}

func (e CallbackError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("oauth callback error %q: %s", e.Code, e.Description)
	}
	return fmt.Sprintf("oauth callback error %q", e.Code)
}

func wrapInvalid(base error, msg string) error {
	return fmt.Errorf("%w: %s", base, msg)
}
