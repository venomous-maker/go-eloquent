# Go Eloquent ORM

A Laravel-inspired ORM for Go, supporting both MySQL and MongoDB backends. This project provides Eloquent-style model interfaces, query builders, and service layers for rapid and expressive database access in Go applications.

## Features
- Eloquent-style model interface and base model
- Query builder with familiar Laravel-like methods (where, order, join, etc.)
- Support for soft deletes, timestamps, and global scopes
- Works with both MySQL and MongoDB
- Easy to extend and customize

## Directory Structure
```
src/
  Global/
    Models/         # BaseModel and ORMModel interfaces
    Controllers/    # Example controllers
    Routes/         # Route helpers
  Libs/             # Utility libraries
  MySQL/ORM/Services/   # MySQL Eloquent service & query builder
  Mongo/ORM/Services/   # MongoDB Eloquent service & query builder
```

## MySQL Usage Example
```go
package main
import (
    MySQLServices "github.com/venomous-maker/go-eloquent/src/MySQL/ORM/Services"
    GlobalModels "github.com/venomous-maker/go-eloquent/src/Global/Models"
)

type User struct {
    GlobalModels.BaseModel
    Name  string `json:"name"`
    Email string `json:"email"`
}

service := MySQLServices.NewEloquentService[User](ctx, db, func() User { return User{} })
users, err := service.All()
```

## MongoDB Usage Example
```go
package main
import (
    MongoServices "github.com/venomous-maker/go-eloquent/src/Mongo/ORM/Services"
    GlobalModels "github.com/venomous-maker/go-eloquent/src/Global/Models"
)

type User struct {
    GlobalModels.BaseModel
    Name  string `bson:"name"`
    Email string `bson:"email"`
}

service := MongoServices.NewEloquentService[User](ctx, mongoDB, func() User { return User{} })
users, err := service.All()
```

## Key Concepts
- **EloquentModel**: Interface for model metadata (table/collection, fillable, guarded, etc.)
- **BaseEloquentService**: Generic service for CRUD, hooks, and query building
- **QueryBuilder**: Chainable query API for both MySQL and MongoDB
- **Soft Deletes & Timestamps**: Supported in both backends

## License
See [LICENSE](LICENSE) for details.

## Contributing
Pull requests and issues are welcome!
