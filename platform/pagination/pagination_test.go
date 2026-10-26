package pagination

import (
	"errors"
	"testing"
	"uuid"
)

func TestInt64Cursor(t *testing.T) {
	id := int64(9223372036854775807)
	cursor := EncodeInt64(id)
	if cursor == "" {
		t.Fatal("cursor is empty")
	}

	got, err := DecodeInt64(cursor)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != id {
		t.Fatalf("id = %d, want %d", got, id)
	}
}

func TestUUIDCursor(t *testing.T) {
	id := uuid.New()
	cursor := EncodeUUID(id)
	if cursor == "" {
		t.Fatal("cursor is empty")
	}

	got, err := DecodeUUID(cursor)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != id {
		t.Fatalf("id = %s, want %s", got, id)
	}
}

func TestCursorKindMismatch(t *testing.T) {
	if _, err := DecodeInt64(EncodeUUID(uuid.New())); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("error = %v, want ErrInvalidCursor", err)
	}
	if _, err := DecodeUUID(EncodeInt64(1)); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("error = %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeInvalidCursor(t *testing.T) {
	for _, raw := range []string{"", "not-base64!", "aTo"} {
		if _, err := DecodeInt64(raw); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("raw %q: error = %v, want ErrInvalidCursor", raw, err)
		}
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "", want: DefaultLimit},
		{raw: "10", want: 10},
		{raw: "1000", want: MaxLimit},
		{raw: "0", wantErr: true},
		{raw: "-1", wantErr: true},
		{raw: "abc", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ParseLimit(tt.raw)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalidLimit) {
				t.Fatalf("raw %q: error = %v, want ErrInvalidLimit", tt.raw, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("raw %q: %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("raw %q: limit = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestNewPage(t *testing.T) {
	items := []int{3, 2, 1}
	page := NewPage(items, 2, func(v int) string { return EncodeInt64(int64(v)) })
	if len(page.Items) != 2 || page.Items[0] != 3 || page.Items[1] != 2 {
		t.Fatalf("items = %v, want [3 2]", page.Items)
	}
	if !page.HasMore {
		t.Fatal("HasMore = false, want true")
	}
	if page.NextCursor != EncodeInt64(2) {
		t.Fatalf("next cursor = %q, want %q", page.NextCursor, EncodeInt64(2))
	}

	empty := NewPage([]int(nil), 2, func(v int) string { return EncodeInt64(int64(v)) })
	if empty.Items == nil {
		t.Fatal("items is nil, want empty slice")
	}
	if empty.HasMore || empty.NextCursor != "" {
		t.Fatalf("empty page = %+v, want no more", empty)
	}
}
