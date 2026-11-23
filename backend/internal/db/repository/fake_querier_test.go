package repository

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeQuerier is a hand-written db.Querier (no database in unit tests, see
// AGENTS.md Testing): it records the last statement and replays canned
// rows/tags/errors, so repository tests cover SQL arguments, scan order and
// the pgx.ErrNoRows / RowsAffected → NotFoundError mapping.
type fakeQuerier struct {
	// Recorded calls, in order.
	execSQL  []string
	execArgs [][]any
	querySQL string
	args     []any // args of the last Query/QueryRow

	// Canned replies.
	execTag    pgconn.CommandTag
	execErr    error
	execErrAt  int // 1-based index of the Exec call that fails; 0 = every call when execErr != nil
	rowValues  []any
	rowErr     error
	lastRow    *fakeRow // the row handed out by the last QueryRow
	queryErr   error
	rowsValues [][]any
	rowsErrAt  int // 1-based row index whose Scan fails (0 = none)
	rowsErr    error
}

func (fake *fakeQuerier) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	fake.execSQL = append(fake.execSQL, sql)
	fake.execArgs = append(fake.execArgs, args)
	if fake.execErr != nil && (fake.execErrAt == 0 || fake.execErrAt == len(fake.execSQL)) {
		return pgconn.CommandTag{}, fake.execErr
	}
	return fake.execTag, nil
}

func (fake *fakeQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	fake.querySQL, fake.args = sql, args
	if fake.queryErr != nil {
		return nil, fake.queryErr
	}
	return &fakeRows{fake: fake}, nil
}

func (fake *fakeQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	fake.querySQL, fake.args = sql, args
	fake.lastRow = &fakeRow{values: fake.rowValues, err: fake.rowErr}
	return fake.lastRow
}

// scanInto assigns values[i] to the pointer dest[i]; a nil value zeroes it.
func scanInto(dest, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("scan: %d destinations, %d values", len(dest), len(values))
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i]).Elem()
		if values[i] == nil {
			target.SetZero()
			continue
		}
		value := reflect.ValueOf(values[i])
		if !value.Type().AssignableTo(target.Type()) {
			return fmt.Errorf("scan: column %d: %s not assignable to %s", i, value.Type(), target.Type())
		}
		target.Set(value)
	}
	return nil
}

type fakeRow struct {
	values []any
	err    error
	// scanned is the destination count of the last Scan.
	scanned int
}

func (row *fakeRow) Scan(dest ...any) error {
	row.scanned = len(dest)
	if row.err != nil {
		return row.err
	}
	return scanInto(dest, row.values)
}

type fakeRows struct {
	fake *fakeQuerier
	next int
}

func (rows *fakeRows) Next() bool {
	rows.next++
	return rows.next <= len(rows.fake.rowsValues)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.fake.rowsErrAt == rows.next {
		return fmt.Errorf("scan failure at row %d", rows.next)
	}
	return scanInto(dest, rows.fake.rowsValues[rows.next-1])
}

func (rows *fakeRows) Err() error                                   { return rows.fake.rowsErr }
func (rows *fakeRows) Close()                                       {}
func (rows *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (rows *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (rows *fakeRows) Values() ([]any, error)                       { return nil, nil }
func (rows *fakeRows) RawValues() [][]byte                          { return nil }
func (rows *fakeRows) Conn() *pgx.Conn                              { return nil }
