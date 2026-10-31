package service

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeDB is a small in-memory seam for service tests. It only implements the
// methods used by generated repository queries.
type fakeDB struct {
	queryRowFn func(context.Context, string, ...any) pgx.Row
	queryFn    func(context.Context, string, ...any) (pgx.Rows, error)
	execFn     func(context.Context, string, ...any) (pgconn.CommandTag, error)
	beginErr   error
	commitErr  error
	tx         *fakeTx
}

func (f *fakeDB) Begin(context.Context) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	f.tx = &fakeTx{db: f}
	return f.tx, nil
}

func (f *fakeDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFn == nil {
		return pgconn.CommandTag{}, nil
	}
	return f.execFn(ctx, sql, args...)
}

func (f *fakeDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.queryFn == nil {
		return &fakeRows{}, nil
	}
	return f.queryFn(ctx, sql, args...)
}

func (f *fakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFn == nil {
		return fakeRow{err: pgx.ErrNoRows}
	}
	return f.queryRowFn(ctx, sql, args...)
}

type fakeTx struct {
	pgx.Tx     // embedded for unused methods
	db         *fakeDB
	committed  bool
	rolledBack bool
}

func (t *fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.db.Exec(ctx, sql, args...)
}

func (t *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return t.db.Query(ctx, sql, args...)
}

func (t *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return t.db.QueryRow(ctx, sql, args...)
}

func (t *fakeTx) Commit(context.Context) error {
	if t.db.commitErr != nil {
		return t.db.commitErr
	}
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assignRow(dest, r.values)
}

type fakeRows struct {
	pgx.Rows // embedded for unused methods
	data     [][]any
	index    int
	err      error
}

func (r *fakeRows) Next() bool {
	if r.index >= len(r.data) {
		return false
	}
	r.index++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.index == 0 || r.index > len(r.data) {
		return fmt.Errorf("scan called without next")
	}
	return assignRow(dest, r.data[r.index-1])
}

func (r *fakeRows) Err() error { return r.err }
func (r *fakeRows) Close()     {}

func assignRow(dest []any, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("scan destination count %d != values %d", len(dest), len(values))
	}
	for i := range dest {
		dst := reflect.ValueOf(dest[i])
		if dst.Kind() != reflect.Pointer || dst.IsNil() {
			return fmt.Errorf("destination %d is not a pointer", i)
		}
		if values[i] == nil {
			dst.Elem().SetZero()
			continue
		}
		src := reflect.ValueOf(values[i])
		if !src.Type().AssignableTo(dst.Elem().Type()) {
			return fmt.Errorf("destination %d type %s != value %s", i, dst.Elem().Type(), src.Type())
		}
		dst.Elem().Set(src)
	}
	return nil
}
