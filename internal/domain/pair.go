package domain

import (
	"fmt"
	"strings"
)

var allowedCurrencies = map[string]struct{}{
	"USD": {},
	"EUR": {},
	"MXN": {},
}

// CurrencyPair is a validated base/quote pair from the currency whitelist.
type CurrencyPair struct {
	Base  string
	Quote string
}

func ParsePair(s string) (CurrencyPair, error) {
	s = strings.TrimSpace(s)
	sep := ""
	switch {
	case strings.Contains(s, "/"):
		sep = "/"
	case strings.Contains(s, "-"):
		sep = "-"
	default:
		return CurrencyPair{}, fmt.Errorf("%w: %q", ErrInvalidPair, s)
	}
	parts := strings.Split(s, sep)
	if len(parts) != 2 {
		return CurrencyPair{}, fmt.Errorf("%w: %q", ErrInvalidPair, s)
	}
	return NewCurrencyPair(parts[0], parts[1])
}

func NewCurrencyPair(base, quote string) (CurrencyPair, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if _, ok := allowedCurrencies[base]; !ok {
		return CurrencyPair{}, fmt.Errorf("%w: unknown currency %q", ErrInvalidPair, base)
	}
	if _, ok := allowedCurrencies[quote]; !ok {
		return CurrencyPair{}, fmt.Errorf("%w: unknown currency %q", ErrInvalidPair, quote)
	}
	if base == quote {
		return CurrencyPair{}, fmt.Errorf("%w: base and quote must differ", ErrInvalidPair)
	}
	return CurrencyPair{Base: base, Quote: quote}, nil
}

func (p CurrencyPair) String() string {
	return p.Base + "/" + p.Quote
}

func (p CurrencyPair) Slug() string {
	return strings.ToLower(p.Base) + "-" + strings.ToLower(p.Quote)
}
