<!-- Banner -->
<p align="center">
  <img src="assets/banner.svg" alt="go-eloquent banner" width="100%" />
</p>

# go-eloquent 🚀

Eloquent-like MongoDB ORM for Go with a fluent, Laravel-style API.

<p align="left">
  <a href="https://pkg.go.dev/github.com/venomous-maker/go-eloquent"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/venomous-maker/go-eloquent.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-00b894.svg"></a>
  <a href="https://github.com/venomous-maker/go-eloquent/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/venomous-maker/go-eloquent/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <img alt="Go Version" src="https://img.shields.io/badge/Go-%E2%89%A51.24-007d9c.svg">
  <img alt="MongoDB" src="https://img.shields.io/badge/MongoDB-Driver-13aa52.svg">
</p>

Why go-eloquent
- ⚡ Fluent query builder: Where, OrWhere, WhereIn, OrderBy, Limit, Skip, Select
- 🧹 Soft deletes with Active, WithTrashed, OnlyTrashed scopes
- 🔗 Eager loading and counts via aggregation (With, WithCount)
- 🧠 Read caching with per-collection TTL and automatic invalidation on writes
- 🪝 Lifecycle hooks: Before/After Save, Update, Delete, Fetch
- 🧪 Simple transactions wrapper
- 🧭 Gin controller helpers for fast CRUD routes

Table of contents
- Getting started
- Soft deletes
- Relationships
- Caching
- Controller helper
- Development

Getting started

Install

```bash
go get github.com/venomous-maker/go-eloquent
```

Model and service

```go
package main

import (
  "context"
  base "github.com/venomous-maker/go-eloquent/Engine/Mongo/Base"
  BaseModels "github.com/venomous-maker/go-eloquent/Models/Base"
  "go.mongodb.org/mongo-driver/mongo"
  "go.mongodb.org/mongo-driver/mongo/options"
)

type User struct {
  *BaseModels.BaseModel
  Name string `json:"name" bson:"name"`
}

func main() {
  ctx := context.Background()
  client, _ := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
  db := client.Database("app")

  svc := base.NewEloquentService[*User](ctx, db, func() *User {
    return &User{BaseModel: &BaseModels.BaseModel{}}
  })

  // Create
  saved, _ := svc.Save(&User{BaseModel: &BaseModels.BaseModel{}, Name: "Ada"})

  // Find
  _ = saved
  found, _ := svc.Find(saved.GetID().Hex())

  // Query
  list, _ := svc.Query().Where("status", "active").OrderBy("created_at", "desc").Take(10).Get()
  first, _ := svc.Query().Where("name", "Ada").First()
  total, _ := svc.Query().Count()
  _, _, _ = list, first, total
  _ = found
}
```

Soft deletes

```go
active, _ := svc.Query().Get()
all, _ := svc.Query().WithTrashed().Get()
deleted, _ := svc.Query().OnlyTrashed().Get()
_ = active; _ = all; _ = deleted
```

Relationships

```go
// Example: Posts belong to a User (posts.user_id -> users._id)
posts, _ := postSvc.Query().BelongsTo("users", "user_id", "_id").Get()

// Example: User has many Posts (posts.user_id -> users._id)
usersWithPosts, _ := userSvc.Query().HasMany("posts", "user_id", "_id").With("posts").Get()

// Example: User belongs to many Roles via pivot table role_user
// role_user.user_id -> users._id, role_user.role_id -> roles._id
usersWithRoles, _ := userSvc.Query().BelongsToMany("roles", "role_user", "user_id", "role_id").With("roles").Get()

_ = posts; _ = usersWithPosts; _ = usersWithRoles
```

Caching

> 💡 Read caching is opt-in via a default TTL at the service level, or per-query.
> Any write (Create/Save/Update/Delete/Restore/ForceDelete) invalidates the collection cache.

```go
// default TTL for this service
svc.SetDefaultCacheTTL(5 * time.Minute)

// per-query override
fast, _ := svc.Query().Where("status", "active").Cache(30 * time.Second).Get()
count, _ := svc.Query().Cache(1 * time.Minute).Count()
_ = fast; _ = count
```

Controller helper

```go
// type UserController struct { BaseControllers.BaseController[*User] }
// ctrl.RegisterRoutes(routerGroup) exposes GET/POST/PUT/DELETE and restore routes.
```

Notes
- Cache covers Get(), Get() with relations (aggregate), and Count()
- Collection cache invalidates on Create/Save/Update/Delete/Restore/ForceDelete

Development

```bash
# Build
go build ./...

# Test
go test ./...

# Vet
go vet ./...

# Lint (optional)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.0
golangci-lint run ./...
```

License
- MIT (see LICENSE)
