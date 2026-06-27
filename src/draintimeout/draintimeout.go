package draintimeout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	ServiceRestartExitCode = 75
	DefaultTimeout         = 60 * time.Second
)

const (
	sourceDefault = "default"
	sourceRaw     = "raw"
	sourceEnv     = "env"
	sourceConfig  = "config"
)

type Resolution struct {
	Timeout     time.Duration
	Source      string
	UsedDefault bool
	Warning     string
}

func Parse(raw string, defaultTimeout time.Duration) Resolution {
	return parse(raw, defaultTimeout, sourceRaw)
}

func Resolve(envValue, configValue string, defaultTimeout time.Duration) Resolution {
	if strings.TrimSpace(envValue) != "" {
		return parse(envValue, defaultTimeout, sourceEnv)
	}
	if strings.TrimSpace(configValue) != "" {
		return parse(configValue, defaultTimeout, sourceConfig)
	}
	return defaultResolution(defaultTimeout, "")
}

func parse(raw string, defaultTimeout time.Duration, source string) Resolution {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultResolution(defaultTimeout, "")
	}

	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return defaultResolution(defaultTimeout, fmt.Sprintf("invalid restart drain timeout %q; using default", raw))
	}
	if seconds < 0 {
		return Resolution{
			Timeout: time.Duration(0),
			Source:  source,
			Warning: fmt.Sprintf("negative restart drain timeout %q; clamped to 0", raw),
		}
	}

	return Resolution{
		Timeout: secondsToDuration(seconds),
		Source:  source,
	}
}

func defaultResolution(defaultTimeout time.Duration, warning string) Resolution {
	return Resolution{
		Timeout:     defaultTimeout,
		Source:      sourceDefault,
		UsedDefault: true,
		Warning:     warning,
	}
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
