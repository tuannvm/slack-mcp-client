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
			name:     "valid interval - minimum 10s",
			interval: "10s",
			wantErr:  false,
		},
		{
			name:     "valid interval - 30s",
			interval: "30s",
			wantErr:  false,
		},
		{
			name:     "short interval - 5s (below minimum)",
			interval: "5s",
			wantErr:  true,
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
	if minReloadInterval != 10*time.Second {
		t.Errorf("minReloadInterval = %v, expected 10s", minReloadInterval)
	}
	
	if defaultShutdownTimeout != 10*time.Second {
		t.Errorf("defaultShutdownTimeout = %v, expected 10s", defaultShutdownTimeout)
	}
}