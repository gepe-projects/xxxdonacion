package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"uuid"
)

const (
	DefaultLimit = 50
	MaxLimit     = 100
)

var (
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrInvalidLimit  = errors.New("invalid limit")
)

// Page adalah response standar untuk cursor pagination.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// ParseLimit membaca query `limit`, memakai default kalau kosong, dan
// membatasi nilai maksimum.
func ParseLimit(raw string) (int, error) {
	if raw == "" {
		return DefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, ErrInvalidLimit
	}
	if limit > MaxLimit {
		return MaxLimit, nil
	}
	return limit, nil
}

// EncodeInt64 membuat cursor opaque untuk primary key BIGINT.
func EncodeInt64(id int64) string {
	return encode("i:", strconv.FormatInt(id, 10))
}

// DecodeInt64 membaca cursor yang dibuat EncodeInt64.
func DecodeInt64(raw string) (int64, error) {
	value, err := decode(raw, "i:")
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	return id, nil
}

// EncodeUUID membuat cursor opaque untuk primary key UUID.
func EncodeUUID(id uuid.UUID) string {
	return encode("u:", id.String())
}

// DecodeUUID membaca cursor yang dibuat EncodeUUID.
func DecodeUUID(raw string) (uuid.UUID, error) {
	value, err := decode(raw, "u:")
	if err != nil {
		return uuid.Nil(), err
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	return id, nil
}

// NewPage memotong items menjadi limit dan mengisi NextCursor dari item
// terakhir. Pemanggil harus mengambil limit+1 baris dari database.
func NewPage[T any](items []T, limit int, cursorOf func(T) string) Page[T] {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	if items == nil {
		items = make([]T, 0)
	}

	page := Page[T]{
		Items:   items,
		HasMore: hasMore,
	}
	if hasMore && len(items) > 0 {
		page.NextCursor = cursorOf(items[len(items)-1])
	}
	return page
}

func encode(prefix, value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(prefix + value))
}

func decode(raw, prefix string) (string, error) {
	if raw == "" {
		return "", ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	value := string(decoded)
	if !strings.HasPrefix(value, prefix) {
		return "", ErrInvalidCursor
	}
	return strings.TrimPrefix(value, prefix), nil
}
