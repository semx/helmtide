package monitor

import (
	"errors"
	"testing"

	"github.com/semx/helmtide/pkg/monitor/prometheus"
)

// newTestConfig returns a monitor config populated with defaults and a valid
// sub-config so that Validate reaches the field under test.
func newTestConfig() *config {
	c := &config{NameF: "test"}
	c.setDefaults()

	sub := prometheus.NewConfig()
	sub.URL = "http://localhost:9090"
	sub.Expr = "up"
	c.subConfig = sub

	return c
}

func TestValidateValidConfig(t *testing.T) {
	c := newTestConfig()

	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid config to pass, got: %v", err)
	}
}

func TestValidateNegativeInterval(t *testing.T) {
	c := newTestConfig()
	c.Interval = -1

	if err := c.Validate(); !errors.Is(err, ErrLowInterval) {
		t.Fatalf("expected ErrLowInterval for negative interval, got: %v", err)
	}
}

func TestValidateZeroInterval(t *testing.T) {
	c := newTestConfig()
	c.Interval = 0

	if err := c.Validate(); !errors.Is(err, ErrLowInterval) {
		t.Fatalf("expected ErrLowInterval for zero interval, got: %v", err)
	}
}

func TestValidateZeroSuccessThreshold(t *testing.T) {
	c := newTestConfig()
	c.SuccessThreshold = 0

	if err := c.Validate(); !errors.Is(err, ErrLowSuccessThreshold) {
		t.Fatalf("expected ErrLowSuccessThreshold for zero success threshold, got: %v", err)
	}
}

func TestValidateZeroFailureThreshold(t *testing.T) {
	c := newTestConfig()
	c.FailureThreshold = 0

	if err := c.Validate(); !errors.Is(err, ErrLowFailureThreshold) {
		t.Fatalf("expected ErrLowFailureThreshold for zero failure threshold, got: %v", err)
	}
}
