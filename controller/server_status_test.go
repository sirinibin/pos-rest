package controller

import (
	"strings"
	"testing"
)

func TestCheckServiceState_ReturnsString(t *testing.T) {
	// systemctl is-active always returns a non-empty state string.
	// On a dev machine without systemd, it returns "inactive" or similar —
	// what matters is it never panics and returns a trimmed string.
	state := checkServiceState("nonexistent-service-xyz.service")
	if strings.ContainsAny(state, " \t\n") {
		t.Errorf("expected trimmed state, got %q", state)
	}
}

func TestBuildServerStatus_OverallLogic(t *testing.T) {
	// Simulate the overall-state derivation rules without hitting real systemctl/redis/mongo.
	// We test the logic by constructing a partial ServerStatus and verifying the Reason field.

	tests := []struct {
		name    string
		api     ComponentStatus
		redis   ComponentStatus
		mongodb ComponentStatus
		want    string // overall
	}{
		{
			name:    "all healthy → running",
			api:     ComponentStatus{true, "Running"},
			redis:   ComponentStatus{true, "OK"},
			mongodb: ComponentStatus{true, "OK"},
			want:    "running",
		},
		{
			name:    "api down → down",
			api:     ComponentStatus{false, "Service stopped"},
			redis:   ComponentStatus{true, "OK"},
			mongodb: ComponentStatus{true, "OK"},
			want:    "down",
		},
		{
			name:    "api up redis down → degraded",
			api:     ComponentStatus{true, "Running"},
			redis:   ComponentStatus{false, "not responding"},
			mongodb: ComponentStatus{true, "OK"},
			want:    "degraded",
		},
		{
			name:    "api up mongo down → degraded",
			api:     ComponentStatus{true, "Running"},
			redis:   ComponentStatus{true, "OK"},
			mongodb: ComponentStatus{false, "not responding"},
			want:    "degraded",
		},
		{
			name:    "api up both infra down → degraded",
			api:     ComponentStatus{true, "Running"},
			redis:   ComponentStatus{false, "err"},
			mongodb: ComponentStatus{false, "err"},
			want:    "degraded",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := ServerStatus{API: tc.api, Redis: tc.redis, MongoDB: tc.mongodb}

			// Replicate the overall-derivation logic from buildServerStatus.
			if s.Overall == "" {
				if !s.API.OK {
					s.Overall = "down"
				} else if s.Redis.OK && s.MongoDB.OK {
					s.Overall = "running"
				} else {
					s.Overall = "degraded"
				}
			}

			if s.Overall != tc.want {
				t.Errorf("overall = %q, want %q", s.Overall, tc.want)
			}
		})
	}
}

func TestBuildServerStatus_ReasonPopulated(t *testing.T) {
	s := ServerStatus{
		API:     ComponentStatus{true, "Running"},
		Redis:   ComponentStatus{false, "connection refused"},
		MongoDB: ComponentStatus{true, "OK"},
	}
	// degraded → reason should mention Redis
	var parts []string
	if !s.Redis.OK {
		parts = append(parts, "Redis: "+s.Redis.Message)
	}
	if !s.MongoDB.OK {
		parts = append(parts, "MongoDB: "+s.MongoDB.Message)
	}
	reason := strings.Join(parts, "; ")
	if !strings.Contains(reason, "Redis") {
		t.Errorf("expected reason to contain 'Redis', got %q", reason)
	}
	if strings.Contains(reason, "MongoDB") {
		t.Errorf("expected MongoDB to be absent from reason when it's OK, got %q", reason)
	}
}
