package controller

import "testing"

func TestGetTokenExpSecondsDefault(t *testing.T) {
	t.Setenv("TOKEN_EXP_SECONDS", "")
	m := &secretMaterializer{}

	if got := m.getTokenExpSeconds(); got != defaultTokenExpSeconds {
		t.Fatalf("expected defaultTokenExpSeconds %d, got %d", defaultTokenExpSeconds, got)
	}
}

func TestGetTokenExpSecondsClampsToMinimum(t *testing.T) {
	t.Setenv("TOKEN_EXP_SECONDS", "300")
	m := &secretMaterializer{}

	if got := m.getTokenExpSeconds(); got != minTokenExpSeconds {
		t.Fatalf("expected minimum %d when value below minimum provided, got %d", minTokenExpSeconds, got)
	}
}

func TestGetTokenExpSecondsRespectsHigherValue(t *testing.T) {
	t.Setenv("TOKEN_EXP_SECONDS", "1200")
	m := &secretMaterializer{}

	if got := m.getTokenExpSeconds(); got != 1200 {
		t.Fatalf("expected 1200 from env override, got %d", got)
	}
}

