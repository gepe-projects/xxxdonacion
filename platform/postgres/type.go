package postgres

import (
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
)

func ToPgText(s string) pgtype.Text {
	return pgtype.Text{
		String: s,
		Valid:  true,
	}
}

func ToPgTextPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{
		String: *s,
		Valid:  *s != "",
	}
}

func FromPgText(v pgtype.Text) string {
	return v.String
}

func FromPgTextPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func ToPgBool(b bool) pgtype.Bool {
	return pgtype.Bool{Bool: b, Valid: true}
}

func ToPgBoolPtr(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func FromPgBool(v pgtype.Bool) bool {
	return v.Bool
}

func FromPgBoolPtr(v pgtype.Bool) *bool {
	if !v.Valid {
		return nil
	}
	return &v.Bool
}

func ToPgInt4(i int32) pgtype.Int4 {
	return pgtype.Int4{Int32: i, Valid: true}
}

func ToPgInt4Ptr(i *int32) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: *i, Valid: true}
}

func FromPgInt4(v pgtype.Int4) int32 {
	return v.Int32
}

func FromPgInt4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

func ToPgInt8(i int64) pgtype.Int8 {
	return pgtype.Int8{Int64: i, Valid: true}
}

func ToPgInt8Ptr(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func FromPgInt8(v pgtype.Int8) int64 {
	return v.Int64
}

func FromPgInt8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func ToPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}
}

func ToPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return ToPgUUID(*id)
}

func FromPgUUID(v pgtype.UUID) uuid.UUID {
	return uuid.UUID(v.Bytes)
}

func FromPgUUIDPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}
