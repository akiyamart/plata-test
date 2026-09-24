package domain

import "errors"

var (
	ErrInvalidQuote  = errors.New("invalid quote")
	ErrQuoteNotFound = errors.New("not found")
)
