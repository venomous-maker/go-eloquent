package GlobalModels

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type BaseModel struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time          `json:"updated_at" bson:"updated_at"`
	DeletedAt time.Time          `json:"deleted_at" bson:"deleted_at"`
	Status    string             `json:"status" bson:"status"`
}

type ORMModel interface {
	GetID() primitive.ObjectID
	SetID(id primitive.ObjectID)
	SetTimestampsOnCreate()
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() time.Time
	GetStatus() string

	GetTable() string
	GetFillable() []string
	GetGuarded() []string
	GetHidden() []string
	GetCasts() map[string]string
	GetDates() []string
	IsTimestamps() bool
	IsSoftDeletes() bool
	GetCreatedAtColumn() string
	GetUpdatedAtColumn() string
	GetDeletedAtColumn() string

	GetCollection() string
}

// GetID returns the ObjectID of the model.
func (b *BaseModel) GetID() primitive.ObjectID {
	return b.ID
}

// SetID sets the ObjectID of the model.
func (b *BaseModel) SetID(id primitive.ObjectID) {
	b.ID = id
}

// SetTimestampsOnCreate sets created_at and updated_at timestamps.
func (b *BaseModel) SetTimestampsOnCreate() {
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	b.DeletedAt = time.Time{}
}

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

// EloquentModel implementation for BaseModel (default values, override in child models as needed)
func (b *BaseModel) GetTable() string {
	return ""
}

func (b *BaseModel) GetFillable() []string {
	return []string{"status"}
}

func (b *BaseModel) GetGuarded() []string {
	return []string{"id"}
}

func (b *BaseModel) GetHidden() []string {
	return []string{}
}

func (b *BaseModel) GetCasts() map[string]string {
	return map[string]string{
		"created_at": "time.Time",
		"updated_at": "time.Time",
		"deleted_at": "time.Time",
	}
}

func (b *BaseModel) GetDates() []string {
	return []string{"created_at", "updated_at", "deleted_at"}
}

func (b *BaseModel) IsTimestamps() bool {
	return true
}

func (b *BaseModel) IsSoftDeletes() bool {
	return true
}

func (b *BaseModel) GetCreatedAtColumn() string {
	return "created_at"
}

func (b *BaseModel) GetUpdatedAtColumn() string {
	return "updated_at"
}

func (b *BaseModel) GetDeletedAtColumn() string {
	return "deleted_at"
}

func (b *BaseModel) GetCollection() string {
	return b.GetTable()
}
