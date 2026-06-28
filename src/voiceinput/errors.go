package voiceinput

import "fmt"

type ErrorKind string

const (
	ErrorKindTranscript      ErrorKind = "transcript_error"
	ErrorKindServer          ErrorKind = "server_error"
	ErrorKindUpgradeRejected ErrorKind = "upgrade_rejected"
	ErrorKindWebSocketClose  ErrorKind = "ws_close"
	ErrorKindConnection      ErrorKind = "connection_error"
	ErrorKindUnknown         ErrorKind = "unknown"
	normalClosureCode                  = 1000
	noStatusReceivedCode               = 1005
)

type ErrorEvent struct {
	Kind       ErrorKind
	Message    string
	Code       string
	HTTPStatus int
	CloseCode  int
	Finalizing bool
}

type ClassifiedError struct {
	Kind    ErrorKind
	Message string
	Fatal   bool
	Ignored bool
}

func ClassifyError(event ErrorEvent) ClassifiedError {
	kind := event.Kind
	if kind == "" {
		kind = ErrorKindUnknown
	}

	err := ClassifiedError{
		Kind:    kind,
		Message: errorMessage(event),
	}

	if event.Finalizing {
		err.Ignored = true
		return err
	}

	switch kind {
	case ErrorKindUpgradeRejected:
		err.Fatal = event.HTTPStatus >= 400 && event.HTTPStatus < 500
	case ErrorKindWebSocketClose:
		err.Ignored = event.CloseCode == normalClosureCode || event.CloseCode == noStatusReceivedCode
	}

	return err
}

func errorMessage(event ErrorEvent) string {
	if event.Message != "" {
		return event.Message
	}
	if event.Code != "" {
		return event.Code
	}
	switch event.Kind {
	case ErrorKindUpgradeRejected:
		if event.HTTPStatus > 0 {
			return fmt.Sprintf("WebSocket upgrade rejected with HTTP %d", event.HTTPStatus)
		}
		return "WebSocket upgrade rejected"
	case ErrorKindWebSocketClose:
		if event.CloseCode > 0 {
			return fmt.Sprintf("Connection closed: code %d", event.CloseCode)
		}
		return "Connection closed"
	case ErrorKindTranscript:
		return "unknown transcription error"
	case ErrorKindServer:
		return "server error"
	case ErrorKindConnection:
		return "voice stream connection error"
	default:
		return "voice input error"
	}
}
