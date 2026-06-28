package mcpelicitation

import "fmt"

type RetryController struct {
	MaxRetries int
}

type RetryDecision struct {
	ShouldRetry       bool   `json:"shouldRetry"`
	DisplayError      string `json:"displayError,omitempty"`
	AttemptsRemaining int    `json:"attemptsRemaining"`
	ResponseAction    Action `json:"responseAction"`
}

func NewRetryController(maxRetries int) RetryController {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return RetryController{MaxRetries: maxRetries}
}

func (c RetryController) AfterElicitation(req Request, response Response, retriesUsed int) RetryDecision {
	if retriesUsed < 0 {
		retriesUsed = 0
	}
	if actionAllowsRetry(response.Action) {
		if retriesUsed >= c.MaxRetries {
			return RetryDecision{
				ResponseAction: response.Action,
				DisplayError: fmt.Sprintf(
					"URL elicitation retry limit reached for MCP tool %q on server %q after %d retries: %s",
					req.ToolName,
					req.ServerName,
					c.MaxRetries,
					req.SafeURL(),
				),
			}
		}
		return RetryDecision{
			ShouldRetry:       true,
			AttemptsRemaining: c.MaxRetries - retriesUsed - 1,
			ResponseAction:    response.Action,
		}
	}

	return RetryDecision{
		ResponseAction: response.Action,
		DisplayError: fmt.Sprintf(
			"URL elicitation was %s for MCP tool %q on server %q: %s",
			displayAction(response.Action),
			req.ToolName,
			req.ServerName,
			req.SafeURL(),
		),
	}
}

func displayAction(action Action) string {
	switch action {
	case ActionDecline:
		return "declined"
	case ActionTimeout:
		return "timed out"
	case ActionCancel:
		return "canceled"
	case ActionOpen:
		return "opened but not completed"
	case "":
		return "not completed"
	default:
		return string(action)
	}
}
