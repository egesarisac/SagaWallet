package repository

import "testing"

func TestIsAllowedTransition(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		{"PENDING", "DEBITED", true},
		{"PENDING", "FAILED", true},
		{"DEBITED", "COMPLETED", true},
		{"DEBITED", "REFUNDING", true},
		{"REFUNDING", "FAILED", true},
		{"REFUNDING", "MANUAL_REVIEW", true},
		{"COMPLETED", "FAILED", false},
		{"FAILED", "DEBITED", false},
		{"PENDING", "COMPLETED", false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			if got := IsAllowedTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("IsAllowedTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
