package domain

import "testing"

func TestStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name    string
		from    Status
		to      Status
		allowed bool
	}{
		{"placed to confirmed is allowed", StatusPlaced, StatusConfirmed, true},
		{"placed to cancelled is allowed", StatusPlaced, StatusCancelled, true},
		{"placed to preparing is NOT allowed (skips confirmed)", StatusPlaced, StatusPreparing, false},
		{"placed to delivered is NOT allowed", StatusPlaced, StatusDelivered, false},

		{"confirmed to preparing is allowed", StatusConfirmed, StatusPreparing, true},
		{"confirmed to cancelled is allowed", StatusConfirmed, StatusCancelled, true},
		{"confirmed back to placed is NOT allowed (no going backwards)", StatusConfirmed, StatusPlaced, false},

		{"preparing to ready_for_pickup is allowed", StatusPreparing, StatusReadyForPickup, true},
		{"preparing to cancelled is allowed (last point cancellation is possible)", StatusPreparing, StatusCancelled, true},

		{"ready_for_pickup to out_for_delivery is allowed", StatusReadyForPickup, StatusOutForDelivery, true},
		{"ready_for_pickup to cancelled is NOT allowed (food is ready, cancellation window has closed)", StatusReadyForPickup, StatusCancelled, false},

		{"out_for_delivery to delivered is allowed", StatusOutForDelivery, StatusDelivered, true},
		{"out_for_delivery to cancelled is NOT allowed", StatusOutForDelivery, StatusCancelled, false},

		{"delivered has no valid transitions (terminal)", StatusDelivered, StatusCancelled, false},
		{"cancelled has no valid transitions (terminal)", StatusCancelled, StatusConfirmed, false},

		{"a status cannot transition to itself", StatusConfirmed, StatusConfirmed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.allowed {
				t.Errorf("Status(%q).CanTransitionTo(%q) = %v, want %v", tt.from, tt.to, got, tt.allowed)
			}
		})
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	terminal := []Status{StatusDelivered, StatusCancelled}
	nonTerminal := []Status{StatusPlaced, StatusConfirmed, StatusPreparing, StatusReadyForPickup, StatusOutForDelivery}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("Status(%q).IsTerminal() = false, want true", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("Status(%q).IsTerminal() = true, want false", s)
		}
	}
}

// TestStatus_EveryNonTerminalStatusHasAtLeastOneTransition guards
// against a future edit accidentally leaving a non-terminal status with
// zero valid transitions, which would silently strand every order that
// reaches it (no API call would ever be able to move it forward again).
func TestStatus_EveryNonTerminalStatusHasAtLeastOneTransition(t *testing.T) {
	nonTerminal := []Status{StatusPlaced, StatusConfirmed, StatusPreparing, StatusReadyForPickup, StatusOutForDelivery}
	allStatuses := []Status{StatusPlaced, StatusConfirmed, StatusPreparing, StatusReadyForPickup, StatusOutForDelivery, StatusDelivered, StatusCancelled}

	for _, from := range nonTerminal {
		hasTransition := false
		for _, to := range allStatuses {
			if from.CanTransitionTo(to) {
				hasTransition = true
				break
			}
		}
		if !hasTransition {
			t.Errorf("Status(%q) is non-terminal but has zero valid outgoing transitions — orders reaching this status would be permanently stuck", from)
		}
	}
}
