package MongoBuilders

import (
	"context"
	"errors"
	GlobalModels "github.com/venomous-maker/go-eloquent/src/Global/Models"
	MongoServices "github.com/venomous-maker/go-eloquent/src/Mongo/ORM/Services"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

// QueryBuilder is a struct for query building
// It is a chainable struct
type QueryBuilder[T GlobalModels.ORMModel] struct {
	service    *MongoServices.BaseService[T]
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
