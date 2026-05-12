package model

import "testing"

func TestTokenRelayRequiresPoolSubscriptionCheck(t *testing.T) {
	freePool := &Pool{MonthlyPriceCny: 0}
	paidPool := &Pool{MonthlyPriceCny: 10}

	if TokenRelayRequiresPoolSubscriptionCheck(nil, true) {
		t.Fatal("nil pool")
	}
	if TokenRelayRequiresPoolSubscriptionCheck(freePool, true) {
		t.Fatal("free pool + require should be false")
	}
	if TokenRelayRequiresPoolSubscriptionCheck(paidPool, false) {
		t.Fatal("paid pool + no token opt-in should be false")
	}
	if !TokenRelayRequiresPoolSubscriptionCheck(paidPool, true) {
		t.Fatal("paid pool + token opt-in should be true")
	}
}
