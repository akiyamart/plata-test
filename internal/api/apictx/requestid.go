package apictx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

const (
	Header = "X-Request-Id"
	MaxLen = 100
)

type contextKey int

const (
	idKey contextKey = iota
	updateIDKey
)

// Valid reports whether id is acceptable as an inbound X-Request-Id.
func Valid(id string) bool {
	if id == "" || len(id) > MaxLen {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

// New mint a random request/correlation id.
func New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, idKey, id)
}

func From(ctx context.Context) string {
	id, _ := ctx.Value(idKey).(string)
	return id
}

func WithUpdateID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, updateIDKey, id)
}

func UpdateID(ctx context.Context) string {
	id, _ := ctx.Value(updateIDKey).(string)
	return id
}
