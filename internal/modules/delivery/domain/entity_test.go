package domain

import "testing"

func TestAssignmentStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name    string
		from    AssignmentStatus
		to      AssignmentStatus
		allowed bool
	}{
		{"offered to accepted is allowed", AssignmentOffered, AssignmentAccepted, true},
		{"offered to rejected is allowed", AssignmentOffered, AssignmentRejected, true},
		{"offered to cancelled is allowed", AssignmentOffered, AssignmentCancelled, true},
		{"offered to picked_up is NOT allowed (must be accepted first)", AssignmentOffered, AssignmentPickedUp, false},
		{"offered to delivered is NOT allowed", AssignmentOffered, AssignmentDelivered, false},

		{"accepted to picked_up is allowed", AssignmentAccepted, AssignmentPickedUp, true},
		{"accepted to cancelled is allowed", AssignmentAccepted, AssignmentCancelled, true},
		{"accepted to rejected is NOT allowed (already accepted, can't reject)", AssignmentAccepted, AssignmentRejected, false},
		{"accepted back to offered is NOT allowed", AssignmentAccepted, AssignmentOffered, false},

		{"picked_up to delivered is allowed", AssignmentPickedUp, AssignmentDelivered, true},
		{"picked_up to cancelled is allowed", AssignmentPickedUp, AssignmentCancelled, true},
		{"picked_up to accepted is NOT allowed", AssignmentPickedUp, AssignmentAccepted, false},

		{"delivered is terminal", AssignmentDelivered, AssignmentCancelled, false},
		{"rejected is terminal", AssignmentRejected, AssignmentOffered, false},
		{"cancelled is terminal", AssignmentCancelled, AssignmentOffered, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.allowed {
				t.Errorf("AssignmentStatus(%q).CanTransitionTo(%q) = %v, want %v", tt.from, tt.to, got, tt.allowed)
			}
		})
	}
}

func TestAssignmentStatus_IsTerminal(t *testing.T) {
	terminal := []AssignmentStatus{AssignmentDelivered, AssignmentRejected, AssignmentCancelled}
	nonTerminal := []AssignmentStatus{AssignmentOffered, AssignmentAccepted, AssignmentPickedUp}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("AssignmentStatus(%q).IsTerminal() = false, want true", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("AssignmentStatus(%q).IsTerminal() = true, want false", s)
		}
	}
}

func TestPartner_IsAvailable(t *testing.T) {
	tests := []struct {
		name      string
		partner   Partner
		maxActive int
		want      bool
	}{
		{
			name:      "verified, online, under capacity is available",
			partner:   Partner{KYCStatus: KYCVerified, IsOnline: true, ActiveAssignmentCount: 1},
			maxActive: 3,
			want:      true,
		},
		{
			name:      "unverified is never available regardless of online/capacity",
			partner:   Partner{KYCStatus: KYCPending, IsOnline: true, ActiveAssignmentCount: 0},
			maxActive: 3,
			want:      false,
		},
		{
			name:      "offline is never available even if verified",
			partner:   Partner{KYCStatus: KYCVerified, IsOnline: false, ActiveAssignmentCount: 0},
			maxActive: 3,
			want:      false,
		},
		{
			name:      "at capacity is not available",
			partner:   Partner{KYCStatus: KYCVerified, IsOnline: true, ActiveAssignmentCount: 3},
			maxActive: 3,
			want:      false,
		},
		{
			name:      "one under capacity is available",
			partner:   Partner{KYCStatus: KYCVerified, IsOnline: true, ActiveAssignmentCount: 2},
			maxActive: 3,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.partner.IsAvailable(tt.maxActive)
			if got != tt.want {
				t.Errorf("Partner.IsAvailable(%d) = %v, want %v", tt.maxActive, got, tt.want)
			}
		})
	}
}
