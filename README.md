# Go Eloquent ORM

A robust, Laravel-inspired Object-Relational Mapping (ORM) framework for Go applications, providing seamless integration with both MySQL and MongoDB databases. This framework implements the Active Record pattern and offers an expressive, fluent interface for database operations.

## Table of Contents
- [Installation](#installation)
- [Features](#features)
- [Configuration](#configuration)
- [Basic Usage](#basic-usage)
- [Advanced Usage](#advanced-usage)
- [Query Building](#query-building)
- [Model Relations](#model-relations)
- [Database Transactions](#database-transactions)
- [Lifecycle Events](#lifecycle-events)
- [Error Handling](#error-handling)
- [Best Practices](#best-practices)
- [Contributing](#contributing)
- [License](#license)

## Installation

```bash
go get github.com/venomous-maker/go-eloquent
```

Ensure your Go version is 1.18 or higher for generics support.

## Features

### Core Features
- Type-safe generic models and queries
- Automatic timestamps management
- Soft delete capability
- Global and local query scopes
- Event hooks and lifecycle callbacks
- Extensible architecture
- Connection pooling and management
- Comprehensive error handling

### Database Support
- **MySQL**
  - Full CRUD operations
  - Complex JOIN operations
  - Transaction support
  - Query optimization
  
- **MongoDB**
  - Document-oriented operations
  - Aggregation pipelines
  - Index management
  - Atomic operations

## Configuration

### MySQL Configuration
```go
type MySQLConfig struct {
    Host     string
    Port     int
    Database string
    Username string
    Password string
    Options  map[string]string
}

config := MySQLConfig{
    Host:     "localhost",
    Port:     3306,
    Database: "myapp",
    Username: "user",
    Password: "password",
    Options: map[string]string{
        "charset":   "utf8mb4",
        "parseTime": "True",
    },
}
```

### MongoDB Configuration
```go
type MongoConfig struct {
    URI      string
    Database string
    Options  *options.ClientOptions
}

config := MongoConfig{
    URI:      "mongodb://localhost:27017",
    Database: "myapp",
}
```

## Basic Usage

### MySQL Example
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

### MongoDB Example
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

## Advanced Usage

### MySQL Advanced Usage & Chained Queries
```go
// Find users with a specific email, order by name, and limit results
users, err := service.Where("email = ?", "foo@bar.com").OrderBy("name").Limit(10).Get()

// Create a new user
user := User{Name: "Alice", Email: "alice@example.com"}
created, err := service.Create(&user)

// Update a user
user.Name = "Alice Smith"
err = service.Update(&user)

// Soft delete a user
err = service.Delete(&user)

// Restore a soft-deleted user
err = service.Restore(&user)
```

### MongoDB Advanced Usage & Chained Queries
```go
// Find users with a specific name, sort, and limit
users, err := service.Where("name", "==", "Bob").OrderBy("email").Limit(5).Get()

// Create a new user
user := User{Name: "Bob", Email: "bob@example.com"}
created, err := service.Create(&user)

// Update a user
user.Email = "bob@newmail.com"
err = service.Update(&user)

// Soft delete a user
err = service.Delete(&user)

// Restore a soft-deleted user
err = service.Restore(&user)
```

## Query Building

### MySQL Query Builder
- **Select**: `service.Select("id, name").Get()`
- **Where**: `service.Where("age > ?", 18).Get()`
- **OrderBy**: `service.OrderBy("name DESC").Get()`
- **GroupBy**: `service.GroupBy("department").Select("department, COUNT(*)").Get()`
- **Having**: `service.Having("COUNT(*) > ?", 1).Get()`

### MongoDB Query Builder
- **Find**: `service.Find(bson.M{"status": "active"}).All()`
- **Select**: `service.Select(bson.M{"name": 1, "email": 1}).All()`
- **Sort**: `service.Sort(bson.M{"name": 1}).All()`
- **Limit**: `service.Limit(10).All()`
- **Skip**: `service.Skip(5).All()`

## Model Relations

### Defining Relations
```go
type User struct {
    GlobalModels.BaseModel
    Name     string    `json:"name" bson:"name"`
    Email    string    `json:"email" bson:"email"`
    Posts    []Post    `json:"posts" bson:"posts" relation:"hasMany"`
    Profile  Profile   `json:"profile" bson:"profile" relation:"hasOne"`
    Roles    []Role    `json:"roles" bson:"roles" relation:"belongsToMany"`
}
```

### Using Relations
```go
// Eager loading
users, err := service.With("Posts", "Profile", "Roles").Get()

// Lazy loading
user, err := service.Find(1)
posts, err := user.Posts().Where("status", "published").Get()
```

## Lifecycle Events
```go
service.BeforeCreate(func(model *User) error {
    model.CreatedAt = time.Now()
    return nil
})

service.AfterCreate(func(model *User) error {
    // Send notifications, update cache, etc.
    return nil
})

// Available hooks:
// - BeforeCreate/AfterCreate
// - BeforeUpdate/AfterUpdate
// - BeforeDelete/AfterDelete
// - BeforeSave/AfterSave
// - BeforeRestore/AfterRestore
```

## Error Handling
```go
type ValidationError struct {
    Field   string
    Message string
}

// Custom error handling
service.OnError(func(err error) error {
    if mysqlErr, ok := err.(*mysql.MySQLError); ok {
        switch mysqlErr.Number {
        case 1062: // Duplicate entry
            return &ValidationError{
                Field:   "email",
                Message: "Email already exists",
            }
        }
    }
    return err
})
```

## Best Practices

### Query Optimization
- Use appropriate indexes
- Implement pagination for large datasets
- Select only needed columns
- Use eager loading to avoid N+1 queries
- Implement caching strategies

### Model Design
- Keep models focused and cohesive
- Use appropriate data types
- Implement validation at the model level
- Use proper naming conventions
- Document model relationships

### Security Considerations
- Implement proper input validation
- Use prepared statements (automatic with this ORM)
- Implement role-based access control
- Sanitize user inputs
- Use secure connection strings

## Performance Monitoring

The ORM includes built-in query logging and performance monitoring:

```go
service.EnableQueryLogging()
service.SetQueryTimeout(5 * time.Second)
service.OnSlowQuery(func(query string, duration time.Duration) {
    log.Printf("Slow query detected: %s (%.2fs)", query, duration.Seconds())
})
```

## Contributing
We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
