package base_test

import (
	"testing"
	"time"

	"github.com/venomous-maker/go-eloquent/Models/Base"
)

func TestSetTimestampsOnCreate(t *testing.T) {
	b := &base.BaseModel{}
	if !b.GetCreatedAt().IsZero() || !b.GetUpdatedAt().IsZero() {
		t.Fatalf("expected zero timestamps before SetTimestampsOnCreate")
	}
	b.SetTimestampsOnCreate()
	if b.GetCreatedAt().IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
	if b.GetUpdatedAt().IsZero() {
		t.Fatalf("expected UpdatedAt to be set")
	}
	if !b.GetDeletedAt().IsZero() {
		t.Fatalf("expected DeletedAt to be zero time after creation")
	}
	// Ensure timestamps are recent
	if time.Since(b.GetCreatedAt()) > time.Minute {
		t.Fatalf("CreatedAt seems too old")
	}
}

func TestGetTableNameNotEmpty(t *testing.T) {
	b := &base.BaseModel{}
	name := b.GetTableName()
	if name == "" {
		t.Fatalf("expected non-empty table name")
	}
}
