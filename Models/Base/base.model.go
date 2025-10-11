package base

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BaseModel is a base model for all models in the application.
// It implements the MongoModel interface.
// It provides a set of zero-value lifecycle hooks and an empty map of global scopes.
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

// GetCollectionName returns the name of the MongoDB collection associated with the model.
//
// IMPORTANT:
//   - When called via an embedded BaseModel on a concrete struct, Go will bind the receiver to the
//     embedded field (*BaseModel), so reflection here cannot reliably discover the outer concrete type.
//   - To ensure correct collection resolution for concrete models, we deliberately return an empty
//     string here. EloquentService will then fall back to deriving the collection name from the
//     concrete model type (snake_case + plural).
//   - If you need a custom collection name, override GetCollectionName on your concrete model type.
func (b *BaseModel) GetCollectionName() string {
	return ""
}

// GetTableName returns the model's table/collection name.
// Mirrors GetCollectionName behavior. Override on concrete models if you need a custom name.
func (b *BaseModel) GetTableName() string {
	return b.GetCollectionName()
}
