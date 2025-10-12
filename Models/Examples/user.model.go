package examples

import (
	BaseModels "github.com/venomous-maker/go-eloquent/Models/Base"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User is a minimal example model demonstrating model-level relationships and default eager loads.
// It shows:
//   - GetRelationships: declare relations once at the model level (agnostic to the engine)
//   - GetDefaultWith:   relations that should be eager-loaded by default for every query
//
// Relationships:
//   - profile: hasOne to profiles, FK=profiles.user_id -> users._id, aliased as "main_profile"
//   - roles:   belongsToMany via role_user (user_id, role_id)
//
// Default eager loads:
//   - "profile as main_profile"
//   - "roles"
//
// With the Mongo engine, the following work out-of-the-box:
//
//	userSvc.Query().Get()                           // auto-loads profile as main_profile and roles
//	userSvc.Query().With("main_profile").Get()      // alias reference works
//	userSvc.Query().HasOne("profile as main_profile", "", "").With("main_profile").Get()
//	userSvc.Query().HasOne("profile", "", "").With("profile as main_profile").Get()
//
// Note: This file is a compile-time example and isn't used in tests. Adapt to your project models.
type User struct {
	*BaseModels.BaseModel
	Name string `json:"name" bson:"name"`
}

// GetRelationships defines model-level relationships for User.
func (u *User) GetRelationships() []BaseModels.RelationDef {
	return []BaseModels.RelationDef{
		{
			Name:       "profile",
			Type:       BaseModels.RelHasOne,
			Related:    "profiles", // explicit collection; if omitted, derived from Name
			ForeignKey: "user_id",  // profiles.user_id -> users._id
			LocalKey:   "_id",
			As:         "main_profile", // alias field in the result document
		},
		{
			Name:    "roles",
			Type:    BaseModels.RelBelongsToMany,
			Related: "roles",
			Pivot: &BaseModels.PivotDef{
				Table:      "role_user",
				ForeignKey: "user_id",
				RelatedKey: "role_id",
			},
		},
	}
}

// GetDefaultWith lists relations to eager-load by default for User queries.
func (u *User) GetDefaultWith() []string {
	return []string{
		"profile as main_profile",
		"roles",
	}
}

// Profile is a minimal related model for the example.
type Profile struct {
	*BaseModels.BaseModel
	UserID primitive.ObjectID `json:"user_id" bson:"user_id"`
	Bio    string             `json:"bio" bson:"bio"`
}

// Role is a minimal related model for the example.
type Role struct {
	*BaseModels.BaseModel
	Name string `json:"name" bson:"name"`
}
