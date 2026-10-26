package postgres

import (
	"testing"
	"uuid"
)

func TestTextConversions(t *testing.T) {
	empty := ""
	if v := ToPgTextPtr(&empty); v.Valid {
		t.Fatal("empty string should map to NULL")
	}
	if v := ToPgTextPtr(nil); v.Valid {
		t.Fatal("nil should map to NULL")
	}

	value := "hello"
	pgValue := ToPgTextPtr(&value)
	if !pgValue.Valid || pgValue.String != value {
		t.Fatalf("value = %+v, want valid hello", pgValue)
	}
	got := FromPgTextPtr(pgValue)
	if got == nil || *got != value {
		t.Fatalf("value = %v, want %q", got, value)
	}

	if FromPgTextPtr(ToPgTextPtr(nil)) != nil {
		t.Fatal("NULL should map to nil")
	}
}

func TestBoolConversions(t *testing.T) {
	v := ToPgBool(true)
	if !v.Valid || !v.Bool {
		t.Fatalf("value = %+v, want valid true", v)
	}
	if got := FromPgBoolPtr(v); got == nil || !*got {
		t.Fatalf("value = %v, want true", got)
	}
	if FromPgBoolPtr(ToPgBoolPtr(nil)) != nil {
		t.Fatal("NULL should map to nil")
	}
}

func TestIntConversions(t *testing.T) {
	if got := FromPgInt4(ToPgInt4(42)); got != 42 {
		t.Fatalf("int4 = %d, want 42", got)
	}
	if got := FromPgInt8(ToPgInt8(42)); got != 42 {
		t.Fatalf("int8 = %d, want 42", got)
	}
	if FromPgInt8Ptr(ToPgInt8Ptr(nil)) != nil {
		t.Fatal("NULL should map to nil")
	}
}

func TestUUIDConversions(t *testing.T) {
	id := uuid.NewV7()
	v := ToPgUUID(id)
	if !v.Valid || FromPgUUID(v) != id {
		t.Fatalf("uuid = %s, want %s", FromPgUUID(v), id)
	}
	if got := FromPgUUIDPtr(v); got == nil || *got != id {
		t.Fatalf("uuid = %v, want %s", got, id)
	}
	if FromPgUUIDPtr(ToPgUUIDPtr(nil)) != nil {
		t.Fatal("NULL should map to nil")
	}
}
