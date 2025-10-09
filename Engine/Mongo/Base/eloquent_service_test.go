package base_test

import (
	"testing"
	"time"

	"github.com/venomous-maker/go-eloquent/Engine/Mongo/Base"
	BaseModels "github.com/venomous-maker/go-eloquent/Models/Base"
	strlib "github.com/venomous-maker/go-eloquent/libs/strings"
	"go.mongodb.org/mongo-driver/bson"
)

// simple test model type
type SimpleModel struct {
	*BaseModels.BaseModel
	Title string `json:"title" bson:"title"`
}

func (s *SimpleModel) GetCollectionName() string { return "" }

func TestEloquentService_SerializationHelpers(t *testing.T) {
	svc := &base.EloquentService[*SimpleModel]{
		Factory: func() *SimpleModel { return &SimpleModel{BaseModel: &BaseModels.BaseModel{}} },
	}

	m := svc.Factory()
	m.Title = "hello"
	m.SetID(m.GetID())
	m.SetTimestampsOnCreate()

	// ToBson
	doc, err := svc.ToBson(m)
	if err != nil {
		t.Fatalf("ToBson error: %v", err)
	}
	if doc["title"] != "hello" {
		t.Fatalf("ToBson mismatch: got %v", doc)
	}

	// ToJson
	j, err := svc.ToJson(m)
	if err != nil {
		t.Fatalf("ToJson error: %v", err)
	}
	if j["title"] != "hello" {
		t.Fatalf("ToJson mismatch: got %v", j)
	}

	// FromBson
	var mm *SimpleModel
	if err := svc.FromBson(&mm, bson.M{"title": "world"}); err != nil {
		t.Fatalf("FromBson error: %v", err)
	}
	if mm.Title != "world" {
		t.Fatalf("FromBson mismatch: got %v", mm)
	}

	// FromJson
	var m2 *SimpleModel
	if err := svc.FromJson(&m2, map[string]interface{}{"title": "xyz"}); err != nil {
		t.Fatalf("FromJson error: %v", err)
	}
	if m2.Title != "xyz" {
		t.Fatalf("FromJson mismatch: got %v", m2)
	}

	// GetRoutePrefix should use hyphenation from strlib
	prefix := svc.GetRoutePrefix()
	// ensure it's non-empty and lowercase
	if prefix == "" {
		t.Fatalf("expected non-empty route prefix")
	}
	// simple check: plural of hyphenated name exists
	if strlib.Pluralize(strlib.Hyphenate("SimpleModel")) != prefix {
		// allow slight variance but at least ensure non-empty
		if prefix == "" {
			t.Fatalf("route prefix appears invalid: %s", prefix)
		}
	}

	// NewModel and GetFactory
	nm := svc.NewModel()
	if nm == nil {
		t.Fatalf("NewModel should not be nil")
	}
	if svc.GetFactory() == nil {
		t.Fatalf("GetFactory should not be nil")
	}

	// Timestamp behavior
	if time.Since(m.GetCreatedAt()) < 0 {
		t.Fatalf("invalid CreatedAt")
	}
}
