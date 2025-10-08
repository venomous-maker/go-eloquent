package GoEloquentModels

import (
	"reflect"
	"time"

	StringLibs "github.com/venomous-maker/go-eloquent/src/Libs/Strings"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BaseModel is a base model for all models in the application.
// It implements the MongoModel interface.
// It provides a set of zero-value lifecycle hooks and an empty map of global scopes.
// It also has a context, a MongoDB database, a factory function, and a collection
// name.
type BaseModel struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time          `json:"updated_at" bson:"updated_at"`
	DeletedAt time.Time          `json:"deleted_at" bson:"deleted_at"`
	Status    string             `json:"status" bson:"status"`
}

type MongoModel interface {
	GetID() primitive.ObjectID
	SetID(id primitive.ObjectID)
	SetTimestampsOnCreate()
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() time.Time
	GetStatus() string
	GetCollectionName() string
	GetTableName() string
}

func (b *BaseModel) GetID() primitive.ObjectID {
	return b.ID
}

func (b *BaseModel) SetID(id primitive.ObjectID) {
	b.ID = id
}

// SetTimestampsOnCreate sets created_at and updated_at timestamps.
//
// It is a part of the MongoModel interface.
func (b *BaseModel) SetTimestampsOnCreate() {
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	b.DeletedAt = time.Time{}
}

// GetCreatedAt returns the created_at timestamp of the model.
//
// It is a part of the MongoModel interface.
//
// Parameters: None
//
// Returns:
// The created_at timestamp of the model.
func (b *BaseModel) GetCreatedAt() time.Time {
	return b.CreatedAt
}

func (b *BaseModel) GetUpdatedAt() time.Time {
	return b.UpdatedAt
}

func (b *BaseModel) GetDeletedAt() time.Time {
	return b.DeletedAt
}

func (b *BaseModel) GetStatus() string {
	return b.Status
}

func (b *BaseModel) GetCollectionName() string {
	// Use reflection to get the name of the struct that embeds BaseModel
	t := reflect.TypeOf(b)

	// If it's a pointer, get the element
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// If the BaseModel is embedded, move one level up
	if t.Kind() == reflect.Struct {
		// Attempt to find the parent struct (the one embedding BaseModel)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.Anonymous && field.Type == reflect.TypeOf(BaseModel{}) {
				// Found the embedded BaseModel
				return StringLibs.Pluralize(StringLibs.ConvertToSnakeCase(t.Name()))
			}
		}
	}

	// Fallback
	return StringLibs.Pluralize(StringLibs.ConvertToSnakeCase(t.Name()))
}

// GetTableName returns the name of the MongoDB collection associated with the model.
//
// It is a part of the MongoModel interface.
//
// It simply calls GetCollectionName() and returns the result.
//
// Parameters: None
//
// Returns:
// The name of the MongoDB collection associated with the model.
func (b *BaseModel) GetTableName() string {
	return b.GetCollectionName()
}
