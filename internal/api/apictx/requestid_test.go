package apictx_test

import (
	"context"
	"testing"

	"quoteservice/internal/api/apictx"
)

func TestValid(t *testing.T) {
	if apictx.Valid("") || apictx.Valid("bad\nid") {
		t.Fatal("expected invalid")
	}
	if !apictx.Valid("abc-123") {
		t.Fatal("expected valid")
	}
	id := apictx.New()
	if !apictx.Valid(id) || len(id) != 32 {
		t.Fatalf("New: %q", id)
	}
}

func TestContext(t *testing.T) {
	ctx := apictx.With(context.Background(), "rid")
	ctx = apictx.WithUpdateID(ctx, "uid")
	if apictx.From(ctx) != "rid" || apictx.UpdateID(ctx) != "uid" {
		t.Fatalf("got %q %q", apictx.From(ctx), apictx.UpdateID(ctx))
	}
}
