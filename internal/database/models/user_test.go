package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type dryRunConnPool struct{}

func (dryRunConnPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("unexpected PrepareContext call")
}

func (dryRunConnPool) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return driver.RowsAffected(1), nil
}

func (dryRunConnPool) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("unexpected QueryContext call")
}

func (dryRunConnPool) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return nil
}

func TestShowLastSeenCreateDefaultAndExplicitValues(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:             dryRunConnPool{},
		WithoutReturning: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run GORM database: %v", err)
	}

	boolPointer := func(value bool) *bool {
		return &value
	}

	tests := []struct {
		name string
		set  *bool
		want bool
	}{
		{name: "unset defaults to true", want: true},
		{name: "explicit false remains false", set: boolPointer(false), want: false},
		{name: "explicit true remains true", set: boolPointer(true), want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := User{ShowLastSeen: test.set}
			result := db.Session(&gorm.Session{DryRun: true}).Create(&user)
			if result.Error != nil {
				t.Fatalf("create user in dry-run mode: %v", result.Error)
			}

			if user.ShowLastSeen == nil {
				t.Fatal("GORM did not populate the default show-last-seen value")
			}
			if got := *user.ShowLastSeen; got != test.want {
				t.Fatalf("ShowLastSeen after create = %t, want %t", got, test.want)
			}
		})
	}
}
