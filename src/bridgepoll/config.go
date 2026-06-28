package bridgepoll

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	defaultPollIntervalMSNotAtCapacity = 2000
	defaultPollIntervalMSAtCapacity    = 600000
	defaultReclaimOlderThanMS          = 5000
	defaultSessionKeepaliveIntervalMS  = 120000
)

const (
	keyPollIntervalMSNotAtCapacity             = "poll_interval_ms_not_at_capacity"
	keyPollIntervalMSAtCapacity                = "poll_interval_ms_at_capacity"
	keyNonExclusiveHeartbeatIntervalMS         = "non_exclusive_heartbeat_interval_ms"
	keyMultisessionPollIntervalMSNotAtCapacity = "multisession_poll_interval_ms_not_at_capacity"
	keyMultisessionPollIntervalMSPartialCap    = "multisession_poll_interval_ms_partial_capacity"
	keyMultisessionPollIntervalMSAtCapacity    = "multisession_poll_interval_ms_at_capacity"
	keyReclaimOlderThanMS                      = "reclaim_older_than_ms"
	keySessionKeepaliveIntervalV2MS            = "session_keepalive_interval_v2_ms"
)

// Config defines the bridge poll timing knobs in milliseconds.
type Config struct {
	PollIntervalMSNotAtCapacity               int `json:"poll_interval_ms_not_at_capacity"`
	PollIntervalMSAtCapacity                  int `json:"poll_interval_ms_at_capacity"`
	NonExclusiveHeartbeatIntervalMS           int `json:"non_exclusive_heartbeat_interval_ms"`
	MultisessionPollIntervalMSNotAtCapacity   int `json:"multisession_poll_interval_ms_not_at_capacity"`
	MultisessionPollIntervalMSPartialCapacity int `json:"multisession_poll_interval_ms_partial_capacity"`
	MultisessionPollIntervalMSAtCapacity      int `json:"multisession_poll_interval_ms_at_capacity"`
	ReclaimOlderThanMS                        int `json:"reclaim_older_than_ms"`
	SessionKeepaliveIntervalV2MS              int `json:"session_keepalive_interval_v2_ms"`
}

// DefaultConfig returns the bridge poll defaults used when external config is absent or invalid.
func DefaultConfig() Config {
	return Config{
		PollIntervalMSNotAtCapacity:               defaultPollIntervalMSNotAtCapacity,
		PollIntervalMSAtCapacity:                  defaultPollIntervalMSAtCapacity,
		NonExclusiveHeartbeatIntervalMS:           0,
		MultisessionPollIntervalMSNotAtCapacity:   defaultPollIntervalMSNotAtCapacity,
		MultisessionPollIntervalMSPartialCapacity: defaultPollIntervalMSNotAtCapacity,
		MultisessionPollIntervalMSAtCapacity:      defaultPollIntervalMSAtCapacity,
		ReclaimOlderThanMS:                        defaultReclaimOlderThanMS,
		SessionKeepaliveIntervalV2MS:              defaultSessionKeepaliveIntervalMS,
	}
}

// Parse overlays raw external config on top of defaults, then validates the whole result.
// Unknown fields are ignored. If any known field is malformed or invalid, the entire
// external config is rejected and defaults are returned with ok=false.
func Parse(raw map[string]any) (cfg Config, ok bool) {
	cfg = DefaultConfig()
	if !applyInteger(raw, keyPollIntervalMSNotAtCapacity, &cfg.PollIntervalMSNotAtCapacity) ||
		!applyInteger(raw, keyPollIntervalMSAtCapacity, &cfg.PollIntervalMSAtCapacity) ||
		!applyInteger(raw, keyNonExclusiveHeartbeatIntervalMS, &cfg.NonExclusiveHeartbeatIntervalMS) ||
		!applyInteger(raw, keyMultisessionPollIntervalMSNotAtCapacity, &cfg.MultisessionPollIntervalMSNotAtCapacity) ||
		!applyInteger(raw, keyMultisessionPollIntervalMSPartialCap, &cfg.MultisessionPollIntervalMSPartialCapacity) ||
		!applyInteger(raw, keyMultisessionPollIntervalMSAtCapacity, &cfg.MultisessionPollIntervalMSAtCapacity) ||
		!applyInteger(raw, keyReclaimOlderThanMS, &cfg.ReclaimOlderThanMS) ||
		!applyInteger(raw, keySessionKeepaliveIntervalV2MS, &cfg.SessionKeepaliveIntervalV2MS) {
		return DefaultConfig(), false
	}

	if err := Validate(cfg); err != nil {
		return DefaultConfig(), false
	}
	return cfg, true
}

// Validate checks the bridge poll timing constraints without fetching external config.
func Validate(cfg Config) error {
	if cfg.PollIntervalMSNotAtCapacity < 100 {
		return fmt.Errorf("%s must be at least 100ms", keyPollIntervalMSNotAtCapacity)
	}
	if err := validateZeroOrAtLeast100(keyPollIntervalMSAtCapacity, cfg.PollIntervalMSAtCapacity); err != nil {
		return err
	}
	if cfg.NonExclusiveHeartbeatIntervalMS < 0 {
		return fmt.Errorf("%s must be at least 0ms", keyNonExclusiveHeartbeatIntervalMS)
	}
	if cfg.MultisessionPollIntervalMSNotAtCapacity < 100 {
		return fmt.Errorf("%s must be at least 100ms", keyMultisessionPollIntervalMSNotAtCapacity)
	}
	if cfg.MultisessionPollIntervalMSPartialCapacity < 100 {
		return fmt.Errorf("%s must be at least 100ms", keyMultisessionPollIntervalMSPartialCap)
	}
	if err := validateZeroOrAtLeast100(keyMultisessionPollIntervalMSAtCapacity, cfg.MultisessionPollIntervalMSAtCapacity); err != nil {
		return err
	}
	if cfg.ReclaimOlderThanMS < 1 {
		return fmt.Errorf("%s must be at least 1ms", keyReclaimOlderThanMS)
	}
	if cfg.SessionKeepaliveIntervalV2MS < 0 {
		return fmt.Errorf("%s must be at least 0ms", keySessionKeepaliveIntervalV2MS)
	}
	if cfg.NonExclusiveHeartbeatIntervalMS <= 0 && cfg.PollIntervalMSAtCapacity <= 0 {
		return fmt.Errorf("at-capacity liveness requires %s > 0 or %s > 0", keyNonExclusiveHeartbeatIntervalMS, keyPollIntervalMSAtCapacity)
	}
	if cfg.NonExclusiveHeartbeatIntervalMS <= 0 && cfg.MultisessionPollIntervalMSAtCapacity <= 0 {
		return fmt.Errorf("at-capacity liveness requires %s > 0 or %s > 0", keyNonExclusiveHeartbeatIntervalMS, keyMultisessionPollIntervalMSAtCapacity)
	}
	return nil
}

func applyInteger(raw map[string]any, key string, target *int) bool {
	rawValue, exists := raw[key]
	if !exists {
		return true
	}
	value, ok := integer(rawValue)
	if !ok {
		return false
	}
	*target = value
	return true
}

func validateZeroOrAtLeast100(name string, value int) error {
	if value == 0 || value >= 100 {
		return nil
	}
	return fmt.Errorf("%s must be 0 or at least 100ms", name)
}

func integer(value any) (int, bool) {
	const maxInt = int64(^uint(0) >> 1)
	const minInt = -maxInt - 1

	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		if v < minInt || v > maxInt {
			return 0, false
		}
		return int(v), true
	case uint:
		if uint64(v) > uint64(maxInt) {
			return 0, false
		}
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		if uint64(v) > uint64(maxInt) {
			return 0, false
		}
		return int(v), true
	case uint64:
		if v > uint64(maxInt) {
			return 0, false
		}
		return int(v), true
	case float32:
		return integerFloat(float64(v), minInt, maxInt)
	case float64:
		return integerFloat(v, minInt, maxInt)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			if i < minInt || i > maxInt {
				return 0, false
			}
			return int(i), true
		}
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return integerFloat(f, minInt, maxInt)
	default:
		return 0, false
	}
}

func integerFloat(value float64, minInt, maxInt int64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < float64(minInt) || value > float64(maxInt) {
		return 0, false
	}
	return int(value), true
}
