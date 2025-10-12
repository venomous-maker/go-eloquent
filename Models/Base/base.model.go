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

// RelationshipType represents the type of relationship a model can define at the model level.
type RelationshipType string

const (
	RelBelongsTo     RelationshipType = "belongsTo"
	RelHasOne        RelationshipType = "hasOne"
	RelHasMany       RelationshipType = "hasMany"
	RelBelongsToMany RelationshipType = "belongsToMany"
)

// PivotDef represents pivot (junction) configuration for many-to-many relations.
type PivotDef struct {
	Table      string   `json:"table" bson:"table"`
	ForeignKey string   `json:"foreign_key" bson:"foreign_key"`
	RelatedKey string   `json:"related_key" bson:"related_key"`
	Fields     []string `json:"fields,omitempty" bson:"fields,omitempty"`
}

// RelationDef is a model-level, engine-agnostic relationship descriptor.
// It lets models describe their relations without importing the query engine to avoid cycles.
type RelationDef struct {
	// Logical relation name, e.g. "profile" or "roles"
	Name string `json:"name" bson:"name"`
	// Type is one of RelationshipType values
	Type RelationshipType `json:"type" bson:"type"`
	// Optional explicit related collection/table; if empty, engine derives from Name
	Related string `json:"related,omitempty" bson:"related,omitempty"`
	// Foreign/local keys
	ForeignKey string `json:"foreign_key,omitempty" bson:"foreign_key,omitempty"`
	LocalKey   string `json:"local_key,omitempty" bson:"local_key,omitempty"`
	// Optional alias: result field name. If empty, engine uses Name
	As string `json:"as,omitempty" bson:"as,omitempty"`
	// Optional conditions map (engine interprets)
	Conditions map[string]interface{} `json:"conditions,omitempty" bson:"conditions,omitempty"`
	// Many-to-many pivot configuration
	Pivot *PivotDef `json:"pivot,omitempty" bson:"pivot,omitempty"`
	// If true, relation acts like an inner join (require at least one related row)
	Required bool `json:"required,omitempty" bson:"required,omitempty"`
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
	// Model-level relationship definitions (optional). BaseModel provides empty defaults.
	GetRelationships() []RelationDef
	// Default relations to eager load by name or "name as alias" tokens. BaseModel provides empty default.
	GetDefaultWith() []string
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

// GetRelationships returns model-defined relations. Default: none.
func (b *BaseModel) GetRelationships() []RelationDef {
	return nil
}

// GetDefaultWith returns relation tokens that should be eager-loaded by default. Default: none.
func (b *BaseModel) GetDefaultWith() []string {
	return nil
}
