# Go Eloquent ORM

A Laravel-inspired ORM for Go, supporting both MySQL and MongoDB backends. This project provides Eloquent-style model interfaces, query builders, and service layers for rapid and expressive database access in Go applications.

## Features
- Eloquent-style model interface and base model
- Query builder with familiar Laravel-like methods (where, order, join, etc.)
- Support for soft deletes, timestamps, and global scopes
- Works with MySQL and MongoDB
- Easy to extend and customize

## Directory Structure
```
src/
  Global/
    Models/         # BaseModel and ORMModel interfaces
    Controllers/    # Example controllers
    Routes/         # Route helpers
  Libs/             # Utility libraries
  MySQL/ORM/Services/   # MySQL Eloquent service implementation
  Mongo/ORM/Services/   # MongoDB Eloquent service implementation
```

## Getting Started
1. Clone the repository:
   ```sh
   git clone <repo-url>
   cd go-eloquent
   ```
2. Install dependencies:
   ```sh
   go mod tidy
   ```
3. Explore the `src/` directory for usage examples and service implementations.

## Usage Example
```go
import (
    MySQLServices "path/to/src/MySQL/ORM/Services"
    GlobalModels "path/to/src/Global/Models"
)

type User struct {
    GlobalModels.BaseModel
    Name string `json:"name"`
    Email string `json:"email"`
}

// Optionally override EloquentModel methods for custom behavior

// Usage
service := MySQLServices.NewEloquentService[User](ctx, db, func() User { return User{} })
users, err := service.All()
```

## License
See [LICENSE](LICENSE) for details.

## Contributing
Pull requests and issues are welcome!

