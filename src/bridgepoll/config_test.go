package bridgepoll

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if err := Validate(cfg); err != nil {
		t.Fatalf("DefaultConfig did not validate: %v", err)
	}
	if cfg.PollIntervalMSNotAtCapacity != 2000 {
		t.Fatalf("expected not-at-capacity default 2000, got %d", cfg.PollIntervalMSNotAtCapacity)
	}
	if cfg.PollIntervalMSAtCapacity != 600000 {
		t.Fatalf("expected at-capacity default 600000, got %d", cfg.PollIntervalMSAtCapacity)
	}
	if cfg.NonExclusiveHeartbeatIntervalMS != 0 {
		t.Fatalf("expected heartbeat default 0, got %d", cfg.NonExclusiveHeartbeatIntervalMS)
	}
	if cfg.MultisessionPollIntervalMSNotAtCapacity != cfg.PollIntervalMSNotAtCapacity {
		t.Fatalf("expected multisession not-at-capacity to inherit default, got %d", cfg.MultisessionPollIntervalMSNotAtCapacity)
	}
	if cfg.MultisessionPollIntervalMSPartialCapacity != cfg.PollIntervalMSNotAtCapacity {
		t.Fatalf("expected multisession partial-capacity to inherit default, got %d", cfg.MultisessionPollIntervalMSPartialCapacity)
	}
	if cfg.MultisessionPollIntervalMSAtCapacity != cfg.PollIntervalMSAtCapacity {
		t.Fatalf("expected multisession at-capacity to inherit default, got %d", cfg.MultisessionPollIntervalMSAtCapacity)
	}
	if cfg.ReclaimOlderThanMS != 5000 {
		t.Fatalf("expected reclaim default 5000, got %d", cfg.ReclaimOlderThanMS)
	}
	if cfg.SessionKeepaliveIntervalV2MS != 120000 {
		t.Fatalf("expected keepalive default 120000, got %d", cfg.SessionKeepaliveIntervalV2MS)
	}
}

func TestParsePartialConfigUsesDefaultsForMissingOptionalFields(t *testing.T) {
	cfg, ok := Parse(map[string]any{
		"poll_interval_ms_not_at_capacity":    3500,
		"poll_interval_ms_at_capacity":        0,
		"non_exclusive_heartbeat_interval_ms": 30000,
		"ignored_future_field":                42,
	})
	if !ok {
		t.Fatalf("expected partial config to parse")
	}

	defaults := DefaultConfig()
	if cfg.PollIntervalMSNotAtCapacity != 3500 {
		t.Fatalf("expected configured not-at-capacity interval, got %d", cfg.PollIntervalMSNotAtCapacity)
	}
	if cfg.PollIntervalMSAtCapacity != 0 {
		t.Fatalf("expected configured at-capacity interval, got %d", cfg.PollIntervalMSAtCapacity)
	}
	if cfg.NonExclusiveHeartbeatIntervalMS != 30000 {
		t.Fatalf("expected configured heartbeat interval, got %d", cfg.NonExclusiveHeartbeatIntervalMS)
	}
	if cfg.MultisessionPollIntervalMSNotAtCapacity != defaults.MultisessionPollIntervalMSNotAtCapacity {
		t.Fatalf("expected default multisession not-at-capacity interval, got %d", cfg.MultisessionPollIntervalMSNotAtCapacity)
	}
	if cfg.MultisessionPollIntervalMSPartialCapacity != defaults.MultisessionPollIntervalMSPartialCapacity {
		t.Fatalf("expected default multisession partial-capacity interval, got %d", cfg.MultisessionPollIntervalMSPartialCapacity)
	}
	if cfg.MultisessionPollIntervalMSAtCapacity != defaults.MultisessionPollIntervalMSAtCapacity {
		t.Fatalf("expected default multisession at-capacity interval, got %d", cfg.MultisessionPollIntervalMSAtCapacity)
	}
	if cfg.ReclaimOlderThanMS != defaults.ReclaimOlderThanMS {
		t.Fatalf("expected default reclaim interval, got %d", cfg.ReclaimOlderThanMS)
	}
	if cfg.SessionKeepaliveIntervalV2MS != defaults.SessionKeepaliveIntervalV2MS {
		t.Fatalf("expected default keepalive interval, got %d", cfg.SessionKeepaliveIntervalV2MS)
	}
}

func TestParseRejectsBadFieldAndFallsBackToDefault(t *testing.T) {
	cfg, ok := Parse(map[string]any{
		"poll_interval_ms_not_at_capacity":    3500,
		"poll_interval_ms_at_capacity":        600000,
		"non_exclusive_heartbeat_interval_ms": 30000,
		"reclaim_older_than_ms":               0,
	})
	if ok {
		t.Fatalf("expected config with bad reclaim field to fail")
	}
	if !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatalf("expected whole config to fall back to defaults, got %+v", cfg)
	}
}

func TestParseRejectsNonIntegerValues(t *testing.T) {
	for _, rawValue := range []any{1.5, json.Number("100.5"), "100"} {
		cfg, ok := Parse(map[string]any{
			"poll_interval_ms_not_at_capacity": rawValue,
		})
		if ok {
			t.Fatalf("expected non-integer value %v to fail", rawValue)
		}
		if !reflect.DeepEqual(cfg, DefaultConfig()) {
			t.Fatalf("expected defaults after non-integer value %v, got %+v", rawValue, cfg)
		}
	}
}

func TestParseAcceptsJSONIntegerNumbers(t *testing.T) {
	cfg, ok := Parse(map[string]any{
		"poll_interval_ms_not_at_capacity": json.Number("2500"),
		"poll_interval_ms_at_capacity":     float64(700000),
	})
	if !ok {
		t.Fatalf("expected integer JSON-shaped values to parse")
	}
	if cfg.PollIntervalMSNotAtCapacity != 2500 {
		t.Fatalf("expected json.Number integer to parse, got %d", cfg.PollIntervalMSNotAtCapacity)
	}
	if cfg.PollIntervalMSAtCapacity != 700000 {
		t.Fatalf("expected float64 integer to parse, got %d", cfg.PollIntervalMSAtCapacity)
	}
}

func TestLivenessAllowsHeartbeatOnlyAtCapacity(t *testing.T) {
	cfg, ok := Parse(map[string]any{
		"poll_interval_ms_at_capacity":              0,
		"multisession_poll_interval_ms_at_capacity": 0,
		"non_exclusive_heartbeat_interval_ms":       1,
	})
	if !ok {
		t.Fatalf("expected heartbeat to satisfy at-capacity liveness")
	}
	if cfg.PollIntervalMSAtCapacity != 0 || cfg.MultisessionPollIntervalMSAtCapacity != 0 {
		t.Fatalf("expected both at-capacity poll intervals disabled, got %+v", cfg)
	}
}

func TestLivenessRequiresHeartbeatOrAtCapacityPolling(t *testing.T) {
	cfg, ok := Parse(map[string]any{
		"poll_interval_ms_at_capacity":              0,
		"multisession_poll_interval_ms_at_capacity": 0,
		"non_exclusive_heartbeat_interval_ms":       0,
	})
	if ok {
		t.Fatalf("expected missing at-capacity liveness to fail")
	}
	if !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatalf("expected liveness failure to fall back to defaults, got %+v", cfg)
	}
}

func TestAtCapacityIntervalsAllowZeroOrAtLeast100(t *testing.T) {
	valid := []int{0, 100, 600000}
	for _, value := range valid {
		cfg := DefaultConfig()
		cfg.PollIntervalMSAtCapacity = value
		cfg.MultisessionPollIntervalMSAtCapacity = value
		cfg.NonExclusiveHeartbeatIntervalMS = 30000
		if err := Validate(cfg); err != nil {
			t.Fatalf("expected at-capacity value %d to validate: %v", value, err)
		}
	}
}

func TestAtCapacityIntervalsRejectOneThroughNinetyNine(t *testing.T) {
	for _, field := range []string{
		"poll_interval_ms_at_capacity",
		"multisession_poll_interval_ms_at_capacity",
	} {
		for _, value := range []int{1, 99} {
			cfg, ok := Parse(map[string]any{
				field:                                 value,
				"non_exclusive_heartbeat_interval_ms": 30000,
			})
			if ok {
				t.Fatalf("expected %s=%d to fail, got %+v", field, value, cfg)
			}
			if !reflect.DeepEqual(cfg, DefaultConfig()) {
				t.Fatalf("expected %s=%d to fall back to defaults, got %+v", field, value, cfg)
			}
		}
	}
}

func TestNotAtCapacityIntervalsRequireAtLeast100(t *testing.T) {
	for _, field := range []string{
		"poll_interval_ms_not_at_capacity",
		"multisession_poll_interval_ms_not_at_capacity",
		"multisession_poll_interval_ms_partial_capacity",
	} {
		cfg, ok := Parse(map[string]any{
			field: 99,
		})
		if ok {
			t.Fatalf("expected %s=99 to fail, got %+v", field, cfg)
		}
		if !reflect.DeepEqual(cfg, DefaultConfig()) {
			t.Fatalf("expected %s=99 to fall back to defaults, got %+v", field, cfg)
		}
	}
}
