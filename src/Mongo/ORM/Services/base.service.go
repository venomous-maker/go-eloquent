package MongoServices

import (
	"context"
	"encoding/json"
	"errors"
	GlobalModels "github.com/venomous-maker/go-eloquent/src/Global/Models"
	StringLibs "github.com/venomous-maker/go-eloquent/src/Libs/Strings"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"reflect"
	"strings"
	"time"
)

// QueryBuilder is a struct for query building
// It is a chainable struct
type QueryBuilder[T GlobalModels.ORMModel] struct {
	service    *BaseService[T]
	filter     bson.M
	opts       *options.FindOptions
	projection bson.M
	order      bson.D
	limit      *int64
	skip       *int64
}

// CollectionName returns the name of the MongoDB collection associated with
// the QueryBuilder.
//
// Returns:
//
//	string: The name of the MongoDB collection used by the QueryBuilder.
func (qb *QueryBuilder[T]) CollectionName() string {
	return qb.service.GetCollectionName()
}

// GetCollection returns the MongoDB collection associated with the QueryBuilder.
//
// This method retrieves the collection from the underlying service, allowing
// direct access to the MongoDB collection for advanced operations.
//
// Returns:
//
//	*mongo.Collection: The MongoDB collection used by the QueryBuilder.
func (qb *QueryBuilder[T]) GetCollection() *mongo.Collection {
	return qb.service.GetCollection()
}

// Apply is a method to apply a scope to the query builder.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"age": bson.M{"$gt": 18}})
//	qb = qb.Apply(func(qb *QueryBuilder[User]) *QueryBuilder[User] {
//	    qb = qb.WithTrashed()
//	    return qb
//	})
//
// This is equivalent to:
//
//	qb := db.Users().Where(bson.M{"age": bson.M{"$gt": 18}}).WithTrashed()
func (qb *QueryBuilder[T]) Apply(scope func(*QueryBuilder[T]) *QueryBuilder[T]) *QueryBuilder[T] {
	return scope(qb)
}

// Where adds a condition to the query filter.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"age": bson.M{"$gt": 18}})
//	qb = qb.Where(bson.M{"deleted_at": nil})
//
// This is equivalent to:
//
//	qb := db.Users().Where(bson.M{
//	    "age": bson.M{"$gt": 18},
//	    "deleted_at": nil,
//	})
func (qb *QueryBuilder[T]) Where(condition bson.M) *QueryBuilder[T] {
	for k, v := range condition {
		qb.filter[k] = v
	}
	return qb
}

// WithTrashed includes soft-deleted records in the query result.
//
// By default, queries exclude soft-deleted documents (where "deleted_at" is not null).
// This method clears the "deleted_at = null" condition to include both active and deleted records.
//
// Example:
//
//	qb := db.Users().WithTrashed()
//	users, _ := qb.Get()
//
// This is equivalent to removing the filter:
//
//	qb := db.Users()
//	qb.IgnoreDeletedAtFilter()
//	users, _ := qb.Get()
func (qb *QueryBuilder[T]) WithTrashed() *QueryBuilder[T] {
	delete(qb.filter, "deleted_at")
	return qb
}

// OnlyTrashed excludes active records from the query result.
//
// This method adds a filter "deleted_at != null" to include only soft-deleted documents.
//
// Example:
//
//	qb := db.Users().OnlyTrashed()
//	users, _ := qb.Get()
//
// This is equivalent to adding the filter:
//
//	qb := db.Users()
//	qb.filter["deleted_at"] = bson.M{"$ne": time.Time{}}
//	users, _ := qb.Get()
func (qb *QueryBuilder[T]) OnlyTrashed() *QueryBuilder[T] {
	qb.filter["deleted_at"] = bson.M{"$ne": time.Time{}}
	return qb
}

// Active excludes soft-deleted records from the query result.
//
// This method adds a filter "deleted_at = null" to include only active documents.
//
// Example:
//
//	qb := db.Users().Active()
//	users, _ := qb.Get()
//
// This is equivalent to adding the filter:
//
//	qb := db.Users()
//	qb.filter["deleted_at"] = time.Time{}
//	users, _ := qb.Get()
func (qb *QueryBuilder[T]) Active() *QueryBuilder[T] {
	qb.filter["deleted_at"] = time.Time{}
	return qb
}

// OrderBy adds a sort order to the query.
//
// The field parameter specifies the field to sort by. The ascending parameter
// specifies whether the sort order is ascending or descending.
//
// The method returns the modified QueryBuilder.
//
// Example:
//
//	qb := db.Users().OrderBy("age", true)
//	users, _ := qb.Get()
//
// This is equivalent to:
//
//	qb := db.Users()
//	qb.order = append(qb.order, bson.E{Key: "age", Value: 1})
//	users, _ := qb.Get()
func (qb *QueryBuilder[T]) OrderBy(field string, ascending bool) *QueryBuilder[T] {
	order := 1
	if !ascending {
		order = -1
	}
	qb.order = append(qb.order, bson.E{Key: field, Value: order})
	return qb
}

// Limit sets the limit for the query result.
//
// The method sets the limit for the number of documents to return.
// If the limit is set to 0, the limit is removed.
//
// Example:
//
//	qb := db.Users().Limit(10)
//	users, _ := qb.Get()
//
// This is equivalent to:
//
//	qb := db.Users()
//	qb.opts.SetLimit(10)
//	users, _ := qb.Get()
func (qb *QueryBuilder[T]) Limit(n int) *QueryBuilder[T] {
	if n <= 0 {
		qb.limit = nil
	} else {
		n64 := int64(n)
		qb.limit = &n64
	}
	return qb
}

// Select specifies the fields to include in the query result.
//
// This method updates the projection of the query to include only the specified fields.
// If no fields are provided, the projection is cleared.
//
// Example:
//
//	qb := db.Users().Select("name", "email")
//	users, _ := qb.Get()
//
// This is equivalent to setting the projection:
//
//	qb := db.Users()
//	qb.projection = bson.M{"name": 1, "email": 1}
//	users, _ := qb.Get()
func (qb *QueryBuilder[T]) Select(fields ...string) *QueryBuilder[T] {
	if len(fields) == 0 {
		qb.projection = nil
		return qb
	}
	proj := bson.M{}
	for _, f := range fields {
		proj[f] = 1
	}
	qb.projection = proj
	return qb
}

// Paginate applies pagination to the query with the given page and limit.
//
// It will use the default page (1) and limit (10) if the provided values are less than 1.
// The total count of documents is also returned.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	users, total, err := qb.Paginate(1, 10)
//
// This is equivalent to:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	qb.skip = 0
//	qb.limit = 10
//	users, total, err := qb.Get()
func (qb *QueryBuilder[T]) Paginate(page, limit int) ([]T, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	skip := int64((page - 1) * limit)
	qb.skip = &skip
	n64 := int64(limit)
	qb.limit = &n64

	return qb.getWithPagination()
}

// getWithPagination executes the query with pagination.
//
// It first counts the total number of documents with the given filter.
// Then it executes the query with the given options and returns the result.
// The result will be paginated according to the `skip` and `limit` fields.
// If the `skip` or `limit` fields are not set, the default values will be used.
// The total count of documents is also returned.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	users, total, err := qb.getWithPagination()
//
// This is equivalent to:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	qb.skip = 0
//	qb.limit = 10
//	users, total, err := qb.getWithPagination()
func (qb *QueryBuilder[T]) getWithPagination() ([]T, int64, error) {
	total, err := qb.service.Collection.CountDocuments(qb.service.Ctx, qb.filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find()
	if qb.limit != nil {
		opts.SetLimit(*qb.limit)
	}
	if qb.skip != nil {
		opts.SetSkip(*qb.skip)
	}
	if len(qb.order) > 0 {
		opts.SetSort(qb.order)
	}
	if qb.projection != nil {
		opts.SetProjection(qb.projection)
	}

	cursor, err := qb.service.Collection.Find(qb.service.Ctx, qb.filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {
			return
		}
	}(cursor, qb.service.Ctx)

	var results []T
	for cursor.Next(qb.service.Ctx) {
		obj := qb.service.Factory()
		if qb.service.BeforeFetch != nil {
			if err := qb.service.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			results = append(results, obj)
		}
		if qb.service.AfterFetch != nil {
			if err := qb.service.AfterFetch(obj); err != nil {
				continue
			}
		}
	}
	return results, total, nil
}

// Get executes the query and returns the results.
//
// It will use the default query options if none are provided.
// The results will be paginated according to the `skip` and `limit` fields.
// If the `skip` or `limit` fields are not set, the default values will be used.
// The total count of documents is also returned.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	users, err := qb.Get()
//
// This is equivalent to:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	qb.skip = 0
//	qb.limit = 10
//	users, err := qb.Get()
func (qb *QueryBuilder[T]) Get() ([]T, error) {
	opts := options.Find()
	if qb.limit != nil {
		opts.SetLimit(*qb.limit)
	}
	if qb.skip != nil {
		opts.SetSkip(*qb.skip)
	}
	if len(qb.order) > 0 {
		opts.SetSort(qb.order)
	}
	if qb.projection != nil {
		opts.SetProjection(qb.projection)
	}

	cursor, err := qb.service.Collection.Find(qb.service.Ctx, qb.filter, opts)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {
			return
		}
	}(cursor, qb.service.Ctx)

	var results []T
	for cursor.Next(qb.service.Ctx) {
		obj := qb.service.Factory()
		if qb.service.BeforeFetch != nil {
			if err := qb.service.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			results = append(results, obj)
		}
		if qb.service.AfterFetch != nil {
			if err := qb.service.AfterFetch(obj); err != nil {
				continue
			}
		}
	}
	return results, nil
}

// First executes the query and returns the first document.
//
// It will use the default query options if none are provided.
// The results will be paginated according to the `skip` and `limit` fields.
// If the `skip` or `limit` fields are not set, the default values will be used.
// The total count of documents is also returned.
//
// If no documents are found, the zero value of T will be returned, and an error of `mongo.ErrNoDocuments` will be returned.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	user, err := qb.First()
//
// This is equivalent to:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	opts := options.FindOne()
//	user, err := qb.service.Collection.FindOne(qb.service.Ctx, qb.filter, opts).Decode(&user)
func (qb *QueryBuilder[T]) First() (T, error) {
	opts := options.FindOne()
	if qb.projection != nil {
		opts.SetProjection(qb.projection)
	}
	var zero T
	obj := qb.service.Factory()
	if qb.service.BeforeFetch != nil {
		if err := qb.service.BeforeFetch(obj); err != nil {
			return zero, err
		}
	}
	err := qb.service.Collection.FindOne(qb.service.Ctx, qb.filter, opts).Decode(obj)
	if err != nil {
		return zero, err
	}
	if qb.service.AfterFetch != nil {
		if err := qb.service.AfterFetch(obj); err != nil {
			return zero, err
		}
	}
	return obj, nil
}

// Each calls the provided function for each document in the result set.
//
// It will use the default query options if none are provided.
// The results will be paginated according to the `skip` and `limit` fields.
// If the `skip` or `limit` fields are not set, the default values will be used.
//
// The provided function will be called once for each document in the result set.
// If the function returns an error, the iteration will stop and the error will be returned.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	err := qb.Each(func(user models.User) error {
//	    // do something with the user
//	    return nil
//	})
func (qb *QueryBuilder[T]) Each(fn func(T) error) error {
	cursor, err := qb.service.Collection.Find(qb.service.Ctx, qb.filter, qb.opts)
	if err != nil {
		return err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {
			return
		}
	}(cursor, qb.service.Ctx)

	for cursor.Next(qb.service.Ctx) {
		obj := qb.service.Factory()
		if qb.service.BeforeFetch != nil {
			if err := qb.service.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			if qb.service.AfterFetch != nil {
				if err := qb.service.AfterFetch(obj); err != nil {
					continue
				}
			}
			if err := fn(obj); err != nil {
				return err
			}
		}
	}
	return nil
}

// Chunk executes the query and passes chunks of the result set to the
// provided function. The chunks are of the given size.
//
// It will use the default query options if none are provided.
// The results will be paginated according to the `skip` and `limit` fields.
// If the `skip` or `limit` fields are not set, the default values will be used.
//
// The provided function will be called once for each chunk of the result set.
// If the function returns an error, the iteration will stop and the error will be returned.
//
// Example:
//
//	qb := db.Users().Where(bson.M{"deleted_at": nil})
//	err := qb.Chunk(10, func(users []models.User) error {
//	    // do something with the users
//	    return nil
//	})
func (qb *QueryBuilder[T]) Chunk(chunkSize int, fn func([]T) error) error {
	if chunkSize <= 0 {
		return errors.New("chunk size must be > 0")
	}

	var page = 1
	for {
		results, total, err := qb.Paginate(page, chunkSize)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			break
		}
		if err := fn(results); err != nil {
			return err
		}
		if int64(page*chunkSize) >= total {
			break
		}
		page++
	}
	return nil
}

// Find retrieves a single document by its ID.
//
// The ID can be provided as a string or a primitive.ObjectID. If the ID is a
// string, it will be converted to a primitive.ObjectID. If the ID is invalid
// or of an unsupported type, an error will be returned.
//
// The method sets the query filter to match the document with the specified ID
// and then retrieves the first matching document using the First() method.
//
// Returns the document of type T and an error if any occurs during the operation.
func (qb *QueryBuilder[T]) Find(id any) (T, error) {
	var objID primitive.ObjectID
	var err error

	switch v := id.(type) {
	case string:
		objID, err = primitive.ObjectIDFromHex(v)
		if err != nil {
			return qb.service.Factory(), errors.New("invalid ID string format")
		}
	case primitive.ObjectID:
		if v.IsZero() {
			return qb.service.Factory(), errors.New("invalid ID: zero value")
		}
		objID = v
	default:
		return qb.service.Factory(), errors.New("unsupported ID type")
	}

	qb.filter["_id"] = objID
	return qb.First()
}

// Insert inserts a new document into the collection
func (qb *QueryBuilder[T]) Insert(model T) error {
	model.SetTimestampsOnCreate()
	model.SetID(primitive.NewObjectID())
	_, err := qb.service.Collection.InsertOne(qb.service.Ctx, model)
	return err
}

// Update updates documents matching the current filter
func (qb *QueryBuilder[T]) Update(data bson.M) error {
	data["updated_at"] = time.Now()
	update := bson.M{"$set": data}
	_, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, update)
	return err
}

// UpdateByID updates a document by its ID
func (qb *QueryBuilder[T]) UpdateByID(id string, data any) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("invalid ID")
	}
	update := bson.M{"$set": data}
	_, err = qb.service.Collection.UpdateByID(qb.service.Ctx, objID, update)
	return err
}

// Delete performs a soft delete by setting deleted_at timestamp
func (qb *QueryBuilder[T]) Delete() error {
	now := time.Now()
	update := bson.M{"$set": bson.M{"deleted_at": now}}
	_, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, update)
	return err
}

// Restore clears the deleted_at field on soft-deleted documents
func (qb *QueryBuilder[T]) Restore() error {
	update := bson.M{"$set": bson.M{"deleted_at": time.Time{}}}
	_, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, update)
	return err
}

// ForceDelete permanently deletes documents matching the current filter
func (qb *QueryBuilder[T]) ForceDelete() error {
	_, err := qb.service.Collection.DeleteMany(qb.service.Ctx, qb.filter)
	return err
}

// ========================
// BaseService
// ========================

type BaseService[T GlobalModels.ORMModel] struct {
	Ctx        context.Context
	DB         *mongo.Database
	Collection *mongo.Collection
	Factory    func() T

	// Collection based
	CollectionName string

	// Lifecycle hooks
	BeforeSave   func(model T) error
	AfterCreate  func(model T) error
	AfterUpdate  func(model T) error
	BeforeDelete func(id string) error
	AfterDelete  func(id string) error
	BeforeFetch  func(model T) error
	AfterFetch   func(model T) error
}

// BaseServiceInterface defines the main service operations for a model T.
type BaseServiceInterface[T GlobalModels.ORMModel] interface {
	// Context and Factory
	GetCtx() context.Context
	GetFactory() func() T

	// CRUD operations
	Find(id string) (T, error)
	FindOrFail(id string) (T, error)
	Save(model T) (T, error)
	Update(id string, updates bson.M) error
	Delete(id string) error
	Restore(id string) error
	ForceDelete(id string) error

	// Query builder and advanced queries
	Query() *QueryBuilder[T]

	// Additional helpers
	UpdateOrCreate(id string, updates bson.M) (T, error)
	CreateOrUpdate(model T) (T, error)
	FirstOrCreate(filter bson.M, newData T) (T, error)
	Reload(model T) (T, error)

	// Internal / helpers exposed for interface use
	NewInstance() BaseService[T]
	GetCollection() *mongo.Collection
	GetModel() T
	NewModel() T

	// Get Collection
	GetCollectionName() string

	// Get Route
	GetRoutePrefix() string

	// Serialization / Deserialization
	ToBson(model T) (bson.M, error)
	ToJson(model T) (map[string]interface{}, error)
	FromBson(model *T, data bson.M) error
	FromJson(model *T, data map[string]interface{}) error
}

// NewBaseService returns a new BaseService instance.
//
// The provided context will be used for all operations.
// The provided mongo database will be used for all operations.
// The provided factory function will be used to create new instances of the model.
//
// The collection name will be determined by calling the `GetCollectionName` method
// on the model returned by the factory function.
func NewBaseService[T GlobalModels.ORMModel](ctx context.Context, db *mongo.Database, factory func() T) *BaseService[T] {
	model := factory()

	// Define a local function to get the collection name with custom logic
	getCollectionName := func(model T) string {
		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return StringLibs.Pluralize(StringLibs.ConvertToSnakeCase(t.Name()))
	}

	baseService := &BaseService[T]{
		Ctx:        ctx,
		DB:         db,
		Collection: db.Collection(getCollectionName(model)),
		Factory:    factory,
	}

	// Define a local function to get the collection name with custom logic
	baseService.CollectionName = getCollectionName(model)

	_, _ = baseService.Collection.UpdateMany(ctx, bson.M{
		"$or": []bson.M{
			{"deleted_at": bson.M{"$exists": false}},
			{"deleted_at": nil},
		},
	}, bson.M{
		"$set": bson.M{"deleted_at": time.Time{}},
	})

	return baseService
}

// Query returns a QueryBuilder instance for this service.
//
// The QueryBuilder will have a default filter of "deleted_at = null" and no
// additional options.
//
// This is a convenience method for creating a QueryBuilder instance with a
// default filter and options.
//
// Example:
//
//	qb := db.Users().Query()
//	users, _ := qb.Get()
func (s *BaseService[T]) Query() *QueryBuilder[T] {
	return &QueryBuilder[T]{
		service: s,
		filter: bson.M{
			"deleted_at": time.Time{},
		},
		opts: options.Find(),
	}
}

// Find retrieves a document by its ID.
//
// It converts the provided string ID to a primitive.ObjectID and searches for a document
// with the matching "_id" and a "deleted_at" field set to the zero time value, indicating
// that the document is active.
//
// Returns the document if found, or an error if the ID is invalid or the document is not found.
func (s *BaseService[T]) Find(id string) (T, error) {
	var zero T
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return zero, err
	}
	return s.findOne(bson.M{
		"_id":        objID,
		"deleted_at": time.Time{},
	})
}

// FindOrFail by ID
func (s *BaseService[T]) FindOrFail(id string) (T, error) {
	result, err := s.Find(id)
	if err != nil {
		return result, errors.New("record with id " + id + " not found")
	}
	return result, nil
}

// Save creates a new record with BeforeSave and AfterCreate hooks
func (s *BaseService[T]) Save(model T) (T, error) {
	if s.BeforeSave != nil {
		if err := s.BeforeSave(model); err != nil {
			return model, err
		}
	}
	model.SetTimestampsOnCreate()
	res, err := s.Collection.InsertOne(s.Ctx, model)
	if err != nil {
		return model, err
	}
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		model.SetID(oid)
	}
	if s.AfterCreate != nil {
		_ = s.AfterCreate(model)
	}
	return model, nil
}

// Update by ID with AfterUpdate hook
func (s *BaseService[T]) Update(id string, updates bson.M) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	updates["updated_at"] = time.Now()
	currentModel, err := s.Find(id)
	if err != nil {
		return err
	}
	updates["created_at"] = currentModel.GetCreatedAt()
	updates["deleted_at"] = currentModel.GetDeletedAt()

	model := s.Factory()
	err = s.FromBson(&model, updates)
	if err != nil {
		return err
	}

	updates, err = s.ToBson(model)
	if err != nil {
		return err
	}

	if s.BeforeSave != nil {
		// Optional: fetch model, apply updates and call BeforeSave here
		err = s.BeforeSave(model)
		if err != nil {
			return err
		}
	}
	_, err = s.Collection.UpdateOne(s.Ctx, bson.M{"_id": objID}, bson.M{"$set": updates})
	if err == nil && s.AfterUpdate != nil {
		model, findErr := s.Find(id)
		if findErr == nil {
			_ = s.AfterUpdate(model)
		}
	}
	return err
}

// Delete (soft delete) with lifecycle hooks
func (s *BaseService[T]) Delete(id string) error {
	if s.BeforeDelete != nil {
		if err := s.BeforeDelete(id); err != nil {
			return err
		}
	}
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.Collection.UpdateOne(s.Ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"deleted_at": time.Now()}})
	if err == nil && s.AfterDelete != nil {
		_ = s.AfterDelete(id)
	}
	return err
}

// Restore a soft deleted document
func (s *BaseService[T]) Restore(id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.Collection.UpdateOne(s.Ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"deleted_at": time.Time{}}})
	return err
}

// ForceDelete hard deletes a document by ID
func (s *BaseService[T]) ForceDelete(id string) error {
	if s.BeforeDelete != nil {
		if err := s.BeforeDelete(id); err != nil {
			return err
		}
	}
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.Collection.DeleteOne(s.Ctx, bson.M{"_id": objID})
	if err == nil && s.AfterDelete != nil {
		_ = s.AfterDelete(id)
	}
	return err
}

// UpdateOrCreate updates by id or creates if not found
func (s *BaseService[T]) UpdateOrCreate(id string, updates bson.M) (T, error) {
	var model T
	model = s.Factory()

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		// Invalid ID — treat as create
		_ = s.FromBson(&model, updates)
		return s.Save(model)
	}

	// Try update
	updates["updated_at"] = time.Now()
	result, err := s.Collection.UpdateOne(s.Ctx, bson.M{"_id": objID}, bson.M{"$set": updates})
	if err != nil {
		return model, err
	}

	if result.MatchedCount == 0 {
		// No document found — create new
		_ = s.FromBson(&model, updates)
		model.SetID(objID) // if you have this method
		return s.Save(model)
	}

	// Updated — fetch latest
	return s.Find(id)
}

// CreateOrUpdate creates or updates a model
func (s *BaseService[T]) CreateOrUpdate(model T) (T, error) {
	doc, err := s.ToBson(model)
	if err != nil {
		return model, err
	}
	return s.UpdateOrCreate(model.GetID().Hex(), doc)
}

// FirstOrCreate finds or creates a record
func (s *BaseService[T]) FirstOrCreate(filter bson.M, newData T) (T, error) {
	obj := s.Factory()
	if s.BeforeFetch != nil {
		if err := s.BeforeFetch(obj); err != nil {
			return obj, err
		}
	}
	err := s.Collection.FindOne(s.Ctx, filter).Decode(obj)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return s.Save(newData)
	} else if err != nil {
		var zero T
		return zero, err
	}
	if s.AfterFetch != nil {
		if err := s.AfterFetch(obj); err != nil {
			return obj, err
		}
	}
	return obj, nil
}

// Reload reloads model from DB
func (s *BaseService[T]) Reload(model T) (T, error) {
	return s.Find(model.GetID().Hex())
}

// findOne helper
func (s *BaseService[T]) findOne(filter bson.M) (T, error) {
	var zero T
	obj := s.Factory()
	if s.BeforeFetch != nil {
		if err := s.BeforeFetch(obj); err != nil {
			return zero, err
		}
	}
	err := s.Collection.FindOne(s.Ctx, filter).Decode(obj)

	if s.AfterFetch != nil {
		if err := s.AfterFetch(obj); err != nil {
			return zero, err
		}
	}
	if err != nil {
		return zero, err
	}
	return obj, nil
}

// findWithFilter helper
func (s *BaseService[T]) findWithFilter(filter bson.M) ([]T, error) {
	cursor, err := s.Collection.Find(s.Ctx, filter)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {
			return
		}
	}(cursor, s.Ctx)

	var results []T
	for cursor.Next(s.Ctx) {
		obj := s.Factory()
		if s.BeforeFetch != nil {
			if err := s.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			if s.AfterFetch != nil {
				if err := s.AfterFetch(obj); err != nil {
					continue
				}
			}
			results = append(results, obj)
		}
	}
	return results, nil
}

// paginate helper used by QueryBuilder.Paginate
func (s *BaseService[T]) paginate(filter bson.M, page, limit int) ([]T, int64, error) {
	skip := (page - 1) * limit
	total, err := s.Collection.CountDocuments(s.Ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	cursor, err := s.Collection.Find(s.Ctx, filter, options.Find().SetSkip(int64(skip)).SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {
			return
		}
	}(cursor, s.Ctx)

	var results []T
	for cursor.Next(s.Ctx) {
		obj := s.Factory()
		if s.BeforeFetch != nil {
			if err := s.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			if s.AfterFetch != nil {
				if err := s.AfterFetch(obj); err != nil {
					continue
				}
			}
			results = append(results, obj)
		}
	}
	return results, total, nil
}

// NewInstance returns a new instance of the service
func (s *BaseService[T]) NewInstance() BaseService[T] {
	return *s
}

// GetCollection returns the collection
func (s *BaseService[T]) GetCollection() *mongo.Collection {
	return s.Collection
}

// GetModel returns the model factory
func (s *BaseService[T]) GetModel() T {
	return s.Factory()
}

// NewModel returns a new instance of the model
func (s *BaseService[T]) NewModel() T {
	return s.Factory()
}

// GetFactory returns the model factory
func (s *BaseService[T]) GetFactory() func() T {
	return s.Factory
}

func (s *BaseService[T]) GetCtx() context.Context {
	return s.Ctx
}

func (s *BaseService[T]) GetCollectionName() string {
	return s.CollectionName
}

func (s *BaseService[T]) GetRoutePrefix() string {
	t := reflect.TypeOf(s.GetFactory()())
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	hyphenated := StringLibs.Hyphenate(t.Name())
	return StringLibs.Pluralize(strings.ToLower(hyphenated))
}

// ToBson converts a model instance to bson.M by marshalling/unmarshalling inside the service.
func (s *BaseService[T]) ToBson(model T) (bson.M, error) {
	data, err := bson.Marshal(model)
	if err != nil {
		return nil, err
	}
	var doc bson.M
	err = bson.Unmarshal(data, &doc)
	return doc, err
}

// ToJson converts a model instance to map[string]interface{} by marshalling/unmarshalling inside the service.
func (s *BaseService[T]) ToJson(model T) (map[string]interface{}, error) {
	bytes, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(bytes, &result)
	return result, err
}

// FromBson converts a bson.M document to the model instance
func (s *BaseService[T]) FromBson(model *T, data bson.M) error {
	bytes, err := bson.Marshal(data)
	if err != nil {
		return err
	}
	return bson.Unmarshal(bytes, model)
}

// FromJson converts a JSON-like map to the model instance
// FromJson converts a JSON-like map to the model instance (must be pointer)
func (s *BaseService[T]) FromJson(model *T, data map[string]interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, model)
}
