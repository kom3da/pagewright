package site

import (
	"errors"
	"math"
	"testing"
)

func TestContrast(t *testing.T) {
	if got := Contrast("#000000", "#ffffff"); math.Abs(got-21) > 0.01 {
		t.Errorf("black on white = %.2f, want 21", got)
	}
	if got := Contrast("#0a4f8f", "#ffffff"); got < MinContrast {
		t.Errorf("default theme color contrast %.2f < %.1f", got, MinContrast)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		s         Settings
		wantField string
	}{
		{"defaults", Settings{}, ""},
		{"dark colors", Settings{PrimaryColor: "#0a4f8f", FooterColor: "#1d2733", Font: "mincho", TimeZone: "Asia/Tokyo", ContactEmail: "info@example.jp"}, ""},
		{"light primary", Settings{PrimaryColor: "#7fb3ff"}, "primaryColor"},
		{"not hex", Settings{FooterColor: "red"}, "footerColor"},
		{"font", Settings{Font: "comic"}, "font"},
		{"zone", Settings{TimeZone: "Mars/Olympus"}, "timeZone"},
		{"email", Settings{ContactEmail: "Info <info@example.jp>"}, "contactEmail"},
	}
	for _, tt := range tests {
		err := tt.s.Validate()
		var fe *FieldError
		got := ""
		if errors.As(err, &fe) {
			got = fe.Field
		}
		if got != tt.wantField {
			t.Errorf("%s: field %q (err %v), want %q", tt.name, got, err, tt.wantField)
		}
	}
}
