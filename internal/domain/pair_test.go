package domain

import (
	"errors"
	"testing"
)

func TestParsePair_valid(t *testing.T) {
	cases := []struct {
		in, wantString, wantSlug string
	}{
		{"EUR/MXN", "EUR/MXN", "eur-mxn"},
		{"eur-mxn", "EUR/MXN", "eur-mxn"},
		{"USD/EUR", "USD/EUR", "usd-eur"},
		{"  mxn/usd  ", "MXN/USD", "mxn-usd"},
	}
	for _, tc := range cases {
		p, err := ParsePair(tc.in)
		if err != nil {
			t.Fatalf("ParsePair(%q): %v", tc.in, err)
		}
		if p.String() != tc.wantString {
			t.Errorf("ParsePair(%q).String() = %q, want %q", tc.in, p.String(), tc.wantString)
		}
		if p.Slug() != tc.wantSlug {
			t.Errorf("ParsePair(%q).Slug() = %q, want %q", tc.in, p.Slug(), tc.wantSlug)
		}
	}
}

func TestParsePair_invalid(t *testing.T) {
	cases := []string{"USD/USD", "BTC/USD", "EURMXN", "", "EUR/MXN/USD", "EUR-USD-MXN"}
	for _, in := range cases {
		_, err := ParsePair(in)
		if !errors.Is(err, ErrInvalidPair) {
			t.Errorf("ParsePair(%q) error = %v, want %v", in, err, ErrInvalidPair)
		}
	}
}

func TestNewCurrencyPair(t *testing.T) {
	p, err := NewCurrencyPair("eur", "mxn")
	if err != nil {
		t.Fatal(err)
	}
	if p.Base != "EUR" || p.Quote != "MXN" {
		t.Fatalf("got %+v", p)
	}
	_, err = NewCurrencyPair("EUR", "EUR")
	if !errors.Is(err, ErrInvalidPair) {
		t.Fatalf("same currency: error = %v, want %v", err, ErrInvalidPair)
	}
}
