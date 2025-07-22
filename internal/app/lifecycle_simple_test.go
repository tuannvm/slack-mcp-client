package app

import (
	"testing"
	"time"
)

func TestValidateReloadInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantErr  bool
	}{
		{
			name:     "valid interval - 30m",
			interval: "30m",
			wantErr:  false,
		},
		{
			name:     "valid interval - 1h",
			interval: "1h",
			wantErr:  false,
		},
		{
			name:     "valid interval - minimum 1m",
			interval: "1m",
			wantErr:  false,
		},
		{
			name:     "short interval - 30s (allowed)",
			interval: "30s",
			wantErr:  false, // We allow short intervals, just log warning
		},
		{
			name:     "invalid format",
			interval: "invalid",
			wantErr:  true,
		},
		{
			name:     "empty interval",
			interval: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReloadInterval(tt.interval)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateReloadInterval() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReloadTriggerTypes(t *testing.T) {
	tests := []struct {
		name     string
		trigger  ReloadTrigger
		expected string
	}{
		{
			name:     "signal trigger",
			trigger:  ReloadTrigger{Type: "signal"},
			expected: "signal",
		},
		{
			name:     "periodic trigger",
			trigger:  ReloadTrigger{Type: "periodic"},
			expected: "periodic",
		},
		{
			name:     "shutdown trigger",
			trigger:  ReloadTrigger{Type: "shutdown"},
			expected: "shutdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.trigger.Type != tt.expected {
				t.Errorf("ReloadTrigger.Type = %v, expected %v", tt.trigger.Type, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are properly set
	if maxBackoffDelay != 5*time.Minute {
		t.Errorf("maxBackoffDelay = %v, expected 5m", maxBackoffDelay)
	}
	
	if backoffMultiplier != 2.0 {
		t.Errorf("backoffMultiplier = %v, expected 2.0", backoffMultiplier)
	}
	
	if minReloadInterval != 1*time.Minute {
		t.Errorf("minReloadInterval = %v, expected 1m", minReloadInterval)
	}
}