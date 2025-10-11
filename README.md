# go-eloquent

Eloquent-like MongoDB ORM and helpers for Go.

Highlights
- Expressive, Laravel-style query builder (Where/OrWhere/WhereIn/OrderBy/Limit/Skip/Select)
- Soft deletes with convenient scopes (Active, WithTrashed, OnlyTrashed)
- Eager loading and counts (With, WithCount) via aggregation lookups
- Lightweight, in-memory per-collection TTL cache for read queries (Get/Aggregate/Count)
- Cache invalidation on all write operations (Create/Save/Update/Delete/Restore/ForceDelete)
- Lifecycle hooks (BeforeSave, AfterCreate, AfterUpdate, BeforeDelete, AfterDelete, BeforeFetch, AfterFetch)
- Simple transactions wrapper
- Gin controller helper with standard CRUD routes and extensible extra routes
- String utilities (pluralize, snake-case, hyphenate) in libs/strings

Install
- Minimum Go: 1.24
- Add to your module
  go get github.com/venomous-maker/go-eloquent

Quick start
1) Define a model (embed BaseModels.BaseModel to get timestamps/soft-delete fields)

   type User struct {
     *BaseModels.BaseModel
     Name string `json:"name" bson:"name"`
   }

2) Create a service for your model

   ctx := context.Background()
   client, _ := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
   db := client.Database("app")

   userService := base.NewEloquentService[*User](ctx, db, func() *User {
     return &User{BaseModel: &BaseModels.BaseModel{}}
   })

3) Basic CRUD and queries

   // Create
   u := &User{BaseModel: &BaseModels.BaseModel{}, Name: "Ada"}
   saved, _ := userService.Save(u)

   // Find one
   found, _ := userService.Find(saved.GetID().Hex())

   // Query builder
   list, _ := userService.Query().Where("status", "active").OrderBy("created_at", "desc").Take(10).Get()

   // Get first or count
   first, _ := userService.Query().Where("name", "Ada").First()
   total, _ := userService.Query().Count()

Soft deletes
- By default queries return only active documents (deleted_at == zero time)
- WithTrashed() includes soft-deleted, OnlyTrashed() limits to soft-deleted

   active, _ := userService.Query().Get()
   all, _ := userService.Query().WithTrashed().Get()
   deleted, _ := userService.Query().OnlyTrashed().Get()

Relationships and counts
- Eager load related collections using With() and WithCount()

   users, _ := userService.Query().With("posts", "profile").WithCount("posts").Get()

Read-query caching
- Enable a default cache TTL for all read queries on the service

   userService.SetDefaultCacheTTL(5 * time.Minute)

- Override per-query using Cache(ttl)

   fast, _ := userService.Query().Where("status", "active").Cache(30 * time.Second).Get()

Notes
- Cache covers Get(), Get() with relations (aggregate), and Count()
- Any write (Create/Save/Update/Delete/Restore/ForceDelete) invalidates cached entries for the service collection

Gin controller helper
- Quickly expose CRUD routes using the provided base controller

   type UserController struct{ BaseControllers.BaseController[*User] }

   func NewUserController(s BaseServices.EloquentServiceInterface[*User]) *UserController {
     return &UserController{BaseControllers.BaseController[*User]{Service: s}}
   }

   r := gin.Default()
   api := r.Group("/api")
   ctrl := NewUserController(userService)
   ctrl.RegisterRoutes(api) // GET/POST/PUT/DELETE + restore + extra routes

Dev scripts
- Build: go build ./...
- Test:  go test ./...
- Vet:   go vet ./...
- Lint (optional):
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.0
  golangci-lint run ./...

Tips
- libs/strings provides strlib.ConvertToSnakeCase, strlib.Pluralize, strlib.Hyphenate
- The service infers the collection name from the model type unless you override GetCollectionName()
