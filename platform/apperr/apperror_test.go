package apperr

import (
	"errors"
	"testing"
)

func TestKindConstructors(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		kind Kind
	}{
		{name: "bad request", err: BadRequest("bad"), kind: KindBadRequest},
		{name: "unauthorized", err: Unauthorized("unauthorized"), kind: KindUnauthorized},
		{name: "forbidden", err: Forbidden("forbidden"), kind: KindForbidden},
		{name: "not found", err: NotFound("not found"), kind: KindNotFound},
		{name: "conflict", err: Conflict("conflict"), kind: KindConflict},
		{name: "too many requests", err: TooManyRequests("slow down"), kind: KindTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Kind != tt.kind {
				t.Fatalf("kind = %v, want %v", tt.err.Kind, tt.kind)
			}
		})
	}
}

func TestInternal(t *testing.T) {
	cause := errors.New("db down")
	err := Internal(cause, "list users")

	if err.Kind != KindInternal {
		t.Fatalf("kind = %v, want internal", err.Kind)
	}
	if err.Message != "oops something went wrong" {
		t.Fatalf("message = %q, want generic message", err.Message)
	}
	if !errors.Is(err, cause) {
		t.Fatal("cause is not wrapped")
	}
}
