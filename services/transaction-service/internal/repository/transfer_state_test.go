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

func TestClassifyTransition(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		expected string
		target   string
		want     TransitionOutcome
	}{
		{"credit success before debit success", "PENDING", "DEBITED", "COMPLETED", TransitionDeferred},
		{"refund success before refunding", "DEBITED", "REFUNDING", "FAILED", TransitionDeferred},
		{"late failure after completion", "COMPLETED", "PENDING", "FAILED", TransitionIgnored},
		{"credit success after refund started", "REFUNDING", "DEBITED", "COMPLETED", TransitionIgnored},
		{"duplicate target state", "FAILED", "REFUNDING", "FAILED", TransitionIgnored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTransition(tt.current, tt.expected, tt.target); got != tt.want {
				t.Fatalf("classifyTransition(%q, %q, %q) = %q, want %q", tt.current, tt.expected, tt.target, got, tt.want)
			}
		})
	}
}
