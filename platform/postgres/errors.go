package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func IsUniqueViolation(err error) bool {
	return hasSQLState(err, "23505")
}

func IsForeignKeyViolation(err error) bool {
	return hasSQLState(err, "23503")
}

func IsNotNullViolation(err error) bool {
	return hasSQLState(err, "23502")
}

func IsCheckViolation(err error) bool {
	return hasSQLState(err, "23514")
}

func IsSerializationFailure(err error) bool {
	return hasSQLState(err, "40001")
}

func IsDeadlockDetected(err error) bool {
	return hasSQLState(err, "40P01")
}

func hasSQLState(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
