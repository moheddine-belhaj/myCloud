package database

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestMigratorIsNotReadyBeforeUp(t *testing.T) {
	m := NewMigrator(nil, fstest.MapFS{})
	if err := m.Check(context.Background()); err == nil {
		t.Fatal("Check succeeded before any migration ran")
	}
	if m.Name() != "migrations" {
		t.Errorf("Name = %q, want migrations", m.Name())
	}
}
