package MongoServices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// =============================================================================
// BaseEloquentService
// =============================================================================

// BaseEloquentService is a base implementation of the EloquentService interface.
// It provides a set of zero-value lifecycle hooks and an empty map of global scopes.
// It also has a context, a MongoDB database, a factory function, and a collection
// name.
type BaseEloquentService[T GlobalModels.ORMModel] struct {
	Ctx            context.Context
	DB             *mongo.Database
	Collection     *mongo.Collection
	Factory        func() T
	CollectionName string

	// Laravel-style lifecycle hooks
	Creating  func(model T) error
	Created   func(model T) error
	Updating  func(model T) error
	Updated   func(model T) error
	Saving    func(model T) error
	Saved     func(model T) error
	Deleting  func(model T) error
	Deleted   func(model T) error
	Restoring func(model T) error
	Restored  func(model T) error
	Retrieved func(model T) error

	// Laravel-style global scopes
	GlobalScopes map[string]func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]
}

// NewEloquentService creates a new MongoDB-based Eloquent service.
//
// It takes a context, a MongoDB database, and a factory function that returns a
// model instance. The service is created with a collection name derived from the
// model name; if the model has a custom table name set, that will be used instead.
//
// The service is returned with a set of zero-value lifecycle hooks and an empty
// map of global scopes.
func NewEloquentService[T GlobalModels.ORMModel](ctx context.Context, db *mongo.Database, factory func() T) *BaseEloquentService[T] {
	model := factory()
	collectionName := model.GetTable()
	if collectionName == "" {
		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		collectionName = StringLibs.Pluralize(StringLibs.ConvertToSnakeCase(t.Name()))
	}

	collection := db.Collection(collectionName)

	return &BaseEloquentService[T]{
		Ctx:            ctx,
		DB:             db,
		Collection:     collection,
		Factory:        factory,
		CollectionName: collectionName,
		GlobalScopes:   make(map[string]func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]),
	}
}

// MongoQueryBuilder is a query builder for MongoDB.
// It implements the EloquentQueryBuilder interface.
// It is a generic type that can be used with any model that implements the
// ORMModel interface.
// It is a chainable query builder, meaning you can call methods in a chain
// to build up the query.
type MongoQueryBuilder[T GlobalModels.ORMModel] struct {
	service     *BaseEloquentService[T]
	filter      bson.M
	sort        bson.D
	limit       int64
	skip        int64
	projection  bson.M
	scopes      []string
	withTrashed bool
	onlyTrashed bool
	forceDelete bool
}

// NewQuery initializes a new MongoQueryBuilder instance for constructing
// MongoDB queries. It applies any defined global scopes to the query builder
// and sets up a filter to exclude soft-deleted records by default if the
// model supports soft deletes. The returned MongoQueryBuilder can be used to
// further build and execute queries on the associated MongoDB collection.
func (s *BaseEloquentService[T]) NewQuery() *MongoQueryBuilder[T] {
	qb := &MongoQueryBuilder[T]{
		service:    s,
		filter:     bson.M{},
		sort:       bson.D{},
		limit:      0,
		skip:       0,
		projection: bson.M{},
		scopes:     []string{},
	}

	// Apply global scopes
	for name, scope := range s.GlobalScopes {
		qb.scopes = append(qb.scopes, name)
		qb = scope(qb)
	}

	// Apply soft delete scope by default
	model := s.Factory()
	if model.IsSoftDeletes() {
		deletedAtCol := model.GetDeletedAtColumn()
		qb.filter[deletedAtCol] = bson.M{"$exists": false}
	}

	return qb
}

// Query initializes a new MongoQueryBuilder instance for constructing
// MongoDB queries. It automatically applies any defined global scopes
// and sets up the builder to exclude soft-deleted records by default
// if the model supports soft deletes. The returned MongoQueryBuilder
// can be used to further refine and execute queries on the associated
// MongoDB collection.
func (s *BaseEloquentService[T]) Query() *MongoQueryBuilder[T] {
	return s.NewQuery()
}

// Where adds a filter to the query builder. The operator parameter defaults to "="
// if not provided. The value parameter may be a string, int, float64, bool, or
// any other type that can be converted to a valid MongoDB query value.
//
// Supported operators:
//
//	"=", "!=", "<>", ">", ">=", "<", "<=", "like", "not like"
//
// "like" and "not like" operators are converted to MongoDB regex queries.
// The value argument should contain SQL LIKE syntax with "%" as the wildcard.
// The query will be case-insensitive.
//
// All other operators are converted directly to MongoDB query syntax.
func (qb *MongoQueryBuilder[T]) Where(column string, args ...interface{}) *MongoQueryBuilder[T] {
	var operator string = "="
	var value interface{}

	switch len(args) {
	case 1:
		value = args[0]
	case 2:
		operator = args[0].(string)
		value = args[1]
	}

	switch operator {
	case "=":
		qb.filter[column] = value
	case "!=", "<>":
		qb.filter[column] = bson.M{"$ne": value}
	case ">":
		qb.filter[column] = bson.M{"$gt": value}
	case ">=":
		qb.filter[column] = bson.M{"$gte": value}
	case "<":
		qb.filter[column] = bson.M{"$lt": value}
	case "<=":
		qb.filter[column] = bson.M{"$lte": value}
	case "like":
		// Convert SQL LIKE to MongoDB regex
		pattern := strings.ReplaceAll(value.(string), "%", ".*")
		qb.filter[column] = bson.M{"$regex": pattern, "$options": "i"}
	case "not like":
		pattern := strings.ReplaceAll(value.(string), "%", ".*")
		qb.filter[column] = bson.M{"$not": bson.M{"$regex": pattern, "$options": "i"}}
	default:
		qb.filter[column] = value
	}

	return qb
}

// OrWhere adds a logical OR condition to the query builder. It accepts one or
// more MongoDB filter documents as arguments. If no arguments are provided,
// the query builder is returned unchanged.
//
// If the query builder already has an existing $or clause, the provided
// conditions are appended to the existing clause. If the query builder has
// existing conditions but no $or clause, the existing conditions are
// automatically wrapped in a new $or clause.
func (qb *MongoQueryBuilder[T]) OrWhere(conditions ...bson.M) *MongoQueryBuilder[T] {
	if len(conditions) == 0 {
		return qb
	}

	if existingOr, exists := qb.filter["$or"]; exists {
		// Add to existing $or
		orArray := existingOr.([]bson.M)
		qb.filter["$or"] = append(orArray, conditions...)
	} else {
		// Check if we need to wrap existing conditions
		if len(qb.filter) > 0 {
			existingConditions := bson.M{}
			for k, v := range qb.filter {
				existingConditions[k] = v
			}
			qb.filter = bson.M{
				"$or": append([]bson.M{existingConditions}, conditions...),
			}
		} else {
			qb.filter["$or"] = conditions
		}
	}

	return qb
}

// WhereIn adds a filter to the query builder that matches documents where the
// value of the given column is in the given list of values.
//
// Example:
//
// qb.WhereIn("age", []int{18, 25, 42})
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//		"$where": "return [18, 25, 42].includes(this.age)"
//	}
func (qb *MongoQueryBuilder[T]) WhereIn(column string, values []interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$in": values}
	return qb
}

// WhereNotIn adds a filter to the query builder that matches documents where the
// value of the given column is NOT in the given list of values.
//
// Example:
//
// qb.WhereNotIn("age", []int{18, 25, 42})
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//		"$where": "return ![18, 25, 42].includes(this.age)"
//	}
func (qb *MongoQueryBuilder[T]) WhereNotIn(column string, values []interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$nin": values}
	return qb
}

// WhereBetween adds a filter to the query builder that matches documents where
// the value of the given column is within the given range of values.
//
// Example:
//
// qb.WhereBetween("age", []int{18, 42})
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//		"age": {
//			"$gte": 18,
//			"$lte": 42
//		}
//	}
func (qb *MongoQueryBuilder[T]) WhereBetween(column string, values [2]interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{
		"$gte": values[0],
		"$lte": values[1],
	}
	return qb
}

// WhereNotBetween adds a filter to the query builder that matches documents
// where the value of the given column is not within the specified range.
//
// Example:
//
// qb.WhereNotBetween("age", [2]interface{}{18, 42})
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "age": {
//	    "$not": {
//	      "$gte": 18,
//	      "$lte": 42
//	    }
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereNotBetween(column string, values [2]interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{
		"$not": bson.M{
			"$gte": values[0],
			"$lte": values[1],
		},
	}
	return qb
}

// whereNull adds a filter to the query builder that matches documents
// where the given column does not exist.
//
// Example:
//
// qb.WhereNull("age")
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "age": {
//	    "$exists": false
//	  }
//	}
func (qb *MongoQueryBuilder[T]) whereNull(column string) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$exists": false}
	return qb
}

// WhereNull adds a filter to the query builder that matches documents
// where the given column does not exist or is set to null.
//
// Example:
//
// qb.WhereNull("age")
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "age": {
//	    "$exists": false
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereNull(column string) *MongoQueryBuilder[T] {
	return qb.whereNull(column)
}

// WhereNotNull adds a filter to the query builder that matches documents
// where the given column exists.
//
// Example:
//
// qb.WhereNotNull("age")
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "age": {
//	    "$exists": true
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereNotNull(column string) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$exists": true}
	return qb
}

// WhereDate adds a filter to the query builder that matches documents where the
// date value of the given column satisfies the specified condition. The operator
// parameter determines the type of comparison (e.g., "=", ">", ">=", "<", "<="),
// and the value parameter can be a string in the "YYYY-MM-DD" format or a time.Time
// object. The function handles date comparisons by normalizing the date and
// constructing the appropriate MongoDB query filter.
//
// Example:
//
// qb.WhereDate("created_at", "=", "2023-10-10")
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "created_at": {
//	    "$gte": ISODate("2023-10-10T00:00:00Z"),
//	    "$lte": ISODate("2023-10-10T23:59:59.999999999Z")
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereDate(column string, operator string, value interface{}) *MongoQueryBuilder[T] {
	// For MongoDB, we need to handle date comparisons differently
	var startDate, endDate time.Time

	switch v := value.(type) {
	case string:
		if parsed, err := time.Parse("2006-01-02", v); err == nil {
			startDate = parsed
			endDate = parsed.Add(24 * time.Hour).Add(-time.Nanosecond)
		}
	case time.Time:
		startDate = time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, v.Location())
		endDate = startDate.Add(24 * time.Hour).Add(-time.Nanosecond)
	}

	switch operator {
	case "=":
		qb.filter[column] = bson.M{"$gte": startDate, "$lte": endDate}
	case ">":
		qb.filter[column] = bson.M{"$gt": endDate}
	case ">=":
		qb.filter[column] = bson.M{"$gte": startDate}
	case "<":
		qb.filter[column] = bson.M{"$lt": startDate}
	case "<=":
		qb.filter[column] = bson.M{"$lte": endDate}
	}

	return qb
}

// WhereTime adds a filter to the query builder that matches documents where the
// time value of the given column satisfies the specified condition. The operator
// parameter determines the type of comparison (e.g., "=", ">", ">=", "<", "<="),
// and the value parameter can be a time.Time object or a string representing the time.
//
// Note: This function currently performs a basic comparison and does not
// extract the time part using MongoDB aggregation.
//
// Example:
//
// qb.WhereTime("created_at", "=", "15:30:00")
//
// This is equivalent to a basic MongoDB query filter using the specified operator.
func (qb *MongoQueryBuilder[T]) WhereTime(column string, operator string, value interface{}) *MongoQueryBuilder[T] {
	// Extract time part using MongoDB aggregation
	// This is more complex in MongoDB - for now, we'll do a basic comparison
	return qb.Where(column, operator, value)
}

// WhereYear adds a filter to the query builder that matches documents where the
// year value of the given column satisfies the specified condition. The operator
// parameter determines the type of comparison (e.g., "=", ">", ">=", "<", "<="),
// and the value parameter must be an integer representing the year.
//
// Example:
//
// qb.WhereYear("created_at", "=", 2023)
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "created_at": {
//	    "$gte": ISODate("2023-01-01T00:00:00Z"),
//	    "$lt": ISODate("2024-01-01T00:00:00Z")
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereYear(column string, operator string, value interface{}) *MongoQueryBuilder[T] {
	year, ok := value.(int)
	if !ok {
		return qb
	}

	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)

	switch operator {
	case "=":
		qb.filter[column] = bson.M{"$gte": startDate, "$lt": endDate}
	case ">":
		qb.filter[column] = bson.M{"$gte": endDate}
	case ">=":
		qb.filter[column] = bson.M{"$gte": startDate}
	case "<":
		qb.filter[column] = bson.M{"$lt": startDate}
	case "<=":
		qb.filter[column] = bson.M{"$lt": endDate}
	}

	return qb
}

// WhereMonth adds a filter to the query builder that matches documents where the
// month value of the given column satisfies the specified condition. The operator
// parameter determines the type of comparison (e.g., "=", ">", ">=", "<", "<="),
// and the value parameter must be an integer in the range 1-12 representing the
// month.
//
// This is simplified - in production, you might want to handle years too.
//
// Example:
//
// qb.WhereMonth("created_at", "=", 10)
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "created_at": {
//	    "$expr": {
//	      "$eq": [{ "$month": "$created_at" }, 10]
//	    }
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereMonth(column string, operator string, value interface{}) *MongoQueryBuilder[T] {
	month, ok := value.(int)
	if !ok || month < 1 || month > 12 {
		return qb
	}

	// This is simplified - in production, you might want to handle years too
	qb.filter[column] = bson.M{
		"$expr": bson.M{
			"$eq": []interface{}{
				bson.M{"$month": "$" + column},
				month,
			},
		},
	}

	return qb
}

// WhereDay adds a filter to the query builder that matches documents where the
// day of the month of the given column satisfies the specified condition. The
// operator parameter is ignored in the current implementation, and the value
// parameter must be an integer representing the day of the month (1-31).
//
// Example:
//
// qb.WhereDay("created_at", "=", 15)
//
// This is equivalent to the following MongoDB query filter:
//
//	{
//	  "created_at": {
//	    "$expr": {
//	      "$eq": [{ "$dayOfMonth": "$created_at" }, 15]
//	    }
//	  }
//	}
func (qb *MongoQueryBuilder[T]) WhereDay(column string, operator string, value interface{}) *MongoQueryBuilder[T] {
	day, ok := value.(int)
	if !ok || day < 1 || day > 31 {
		return qb
	}

	qb.filter[column] = bson.M{
		"$expr": bson.M{
			"$eq": []interface{}{
				bson.M{"$dayOfMonth": "$" + column},
				day,
			},
		},
	}

	return qb
}

// WhereExists adds a filter to the query builder that matches documents where
// the given subquery returns any results. Note that MongoDB does not have a
// direct EXISTS operator, so this method is not implemented yet. If you need
// to emulate the behavior of EXISTS, you will need to use a different
// approach, such as a subquery with $lookup or other aggregation operators.
func (qb *MongoQueryBuilder[T]) WhereExists(subquery func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	// MongoDB doesn't have direct EXISTS - this would need to be implemented
	// based on specific use case using $lookup or other aggregation operators
	// TODO Implement this
	return qb
}

// WhereRaw adds a raw MongoDB filter to the query builder. The filter parameter
// is a bson.M map that represents the raw MongoDB query filter to be applied.
// This method allows the use of complex or custom filters that are not directly
// supported by the query builder's other methods. It merges the provided filter
// with the existing query conditions.
//
// Example:
//
// qb.WhereRaw(bson.M{"status": "active", "age": bson.M{"$gte": 18}})
func (qb *MongoQueryBuilder[T]) WhereRaw(filter bson.M) *MongoQueryBuilder[T] {
	for k, v := range filter {
		qb.filter[k] = v
	}
	return qb
}

// ========================
// Ordering (Laravel-style)
// ========================

// OrderBy adds a sorting clause to the query builder, specifying the column
// and direction of the sort. The direction parameter is optional and defaults
// to ascending order. If a direction is provided and it is "desc", the sort
// order will be descending.
//
// Example:
//
// qb.OrderBy("name")
// qb.OrderBy("age", "desc")
func (qb *MongoQueryBuilder[T]) OrderBy(column string, direction ...string) *MongoQueryBuilder[T] {
	dir := 1 // ascending
	if len(direction) > 0 && strings.ToLower(direction[0]) == "desc" {
		dir = -1
	}

	qb.sort = append(qb.sort, bson.E{Key: column, Value: dir})
	return qb
}

// OrderByDesc is a shortcut for OrderBy with the direction set to "desc". It
// adds a sorting clause to the query builder, specifying the column and
// direction of the sort. The column parameter specifies the column to sort
// by, and the direction parameter is set to "desc" to sort in descending
// order.
//
// Example:
//
// qb.OrderByDesc("name")
func (qb *MongoQueryBuilder[T]) OrderByDesc(column string) *MongoQueryBuilder[T] {
	return qb.OrderBy(column, "desc")
}

// Latest adds a sorting clause to the query builder, sorting by the specified
// column in descending (newest) order. If no column is specified, the default
// column is "created_at".
//
// Example:
//
// qb.Latest()
// qb.Latest("updated_at")
func (qb *MongoQueryBuilder[T]) Latest(column ...string) *MongoQueryBuilder[T] {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return qb.OrderByDesc(col)
}

// Oldest adds a sorting clause to the query builder, sorting by the specified
// column in ascending (oldest) order. If no column is specified, the default
// column is "created_at".
//
// Example:
//
// qb.Oldest()
// qb.Oldest("updated_at")
func (qb *MongoQueryBuilder[T]) Oldest(column ...string) *MongoQueryBuilder[T] {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return qb.OrderBy(col)
}

// InRandomOrder adds a sorting clause to the query builder, sorting the results
// in random order. Note that this is a simplified version of the MongoDB
// equivalent, which uses the $sample aggregation operator. In practice, you'd
// use the aggregation pipeline to achieve this.
func (qb *MongoQueryBuilder[T]) InRandomOrder() *MongoQueryBuilder[T] {
	// MongoDB equivalent of random order using $sample in aggregation
	// This is a simplified version - in practice, you'd use aggregation pipeline
	// TODO Implement this
	return qb
}

// ========================
// Limiting and Offsetting
// ========================

// Take sets the limit for the query, limiting the number of documents returned.
//
// Example:
//
// qb.Take(10) // limits query to 10 documents
func (qb *MongoQueryBuilder[T]) Take(count int) *MongoQueryBuilder[T] {
	qb.limit = int64(count)
	return qb
}

// Limit is an alias for Take, setting the limit for the query, limiting the
// number of documents returned.
//
// Example:
//
// qb.Limit(10) // limits query to 10 documents
func (qb *MongoQueryBuilder[T]) Limit(count int) *MongoQueryBuilder[T] {
	return qb.Take(count)
}

// Skip sets the number of documents to skip before starting to return documents from the query.
// This is useful for pagination, allowing you to skip a specified number of documents.
//
// Example:
//
// qb.Skip(5) // skips the first 5 documents
func (qb *MongoQueryBuilder[T]) Skip(count int) *MongoQueryBuilder[T] {
	qb.skip = int64(count)
	return qb
}

// Offset is an alias for Skip, setting the number of documents to skip before
// starting to return documents from the query. This is useful for pagination,
// allowing you to skip a specified number of documents.
//
// Example:
//
// qb.Offset(5) // skips the first 5 documents
func (qb *MongoQueryBuilder[T]) Offset(count int) *MongoQueryBuilder[T] {
	return qb.Skip(count)
}

// ========================
// Selecting Fields (Projection)
// ========================

// Select sets the fields to include in the query results. The fields parameter
// is a variable number of arguments, each of which specifies a field to include.
// If the "*" field is specified, all fields are included. Otherwise, only the
// specified fields are included. If no fields are specified, all fields are
// included.
//
// Example:
//
// qb.Select("name", "age") // includes only "name" and "age" fields
// qb.Select("*") // includes all fields
func (qb *MongoQueryBuilder[T]) Select(fields ...string) *MongoQueryBuilder[T] {
	qb.projection = bson.M{}
	for _, field := range fields {
		if field != "*" {
			qb.projection[field] = 1
		}
	}
	return qb
}

// AddSelect adds additional fields to the existing projection of the query builder.
// The fields parameter is a variadic argument, allowing you to specify multiple
// field names to include in the projection. If the projection is currently empty,
// it will initialize a new bson.M map. The method ignores the "*" field and sets
// the specified fields to 1 in the projection map, indicating inclusion.
//
// Example:
//
// qb.AddSelect("name", "age") // adds "name" and "age" to the projection
// qb.AddSelect("*") // adds all fields to the projection
func (qb *MongoQueryBuilder[T]) AddSelect(fields ...string) *MongoQueryBuilder[T] {
	if len(qb.projection) == 0 {
		qb.projection = bson.M{}
	}
	for _, field := range fields {
		if field != "*" {
			qb.projection[field] = 1
		}
	}
	return qb
}

// Exclude sets the specified fields to 0 in the projection map, excluding
// them from the query results. The fields parameter is a variadic argument,
// allowing you to specify multiple field names to exclude from the projection.
// If the projection is currently empty, it will initialize a new bson.M map.
// The method ignores the "*" field.
//
// Example:
//
// qb.Exclude("name", "age") // excludes "name" and "age" from the projection
func (qb *MongoQueryBuilder[T]) Exclude(fields ...string) *MongoQueryBuilder[T] {
	if len(qb.projection) == 0 {
		qb.projection = bson.M{}
	}
	for _, field := range fields {
		qb.projection[field] = 0
	}
	return qb
}

// ========================
// Soft Deletes (Laravel-style)
// ========================

// WithTrashed sets the query builder to include soft deleted records in the query results.
// This method is only applicable if the model supports soft deletes. If the model does not
// support soft deletes, this method has no effect.
//
// Example:
//
// qb.WithTrashed() // includes soft deleted records in the query results
func (qb *MongoQueryBuilder[T]) WithTrashed() *MongoQueryBuilder[T] {
	qb.withTrashed = true
	qb.onlyTrashed = false

	model := qb.service.Factory()
	if model.IsSoftDeletes() {
		deletedAtCol := model.GetDeletedAtColumn()
		delete(qb.filter, deletedAtCol)
	}

	return qb
}

// WithoutTrashed sets the query builder to exclude soft deleted records from the query results.
// This method is only applicable if the model supports soft deletes. If the model does not
// support soft deletes, this method has no effect.
//
// Example:
//
// qb.WithoutTrashed() // excludes soft deleted records from the query results
func (qb *MongoQueryBuilder[T]) WithoutTrashed() *MongoQueryBuilder[T] {
	qb.withTrashed = false
	qb.onlyTrashed = false

	model := qb.service.Factory()
	if model.IsSoftDeletes() {
		deletedAtCol := model.GetDeletedAtColumn()
		qb.filter[deletedAtCol] = bson.M{"$exists": false}
	}

	return qb
}

// OnlyTrashed sets the query builder to include only soft deleted records in the query results.
// This method is only applicable if the model supports soft deletes. If the model does not
// support soft deletes, this method has no effect.
//
// Example:
//
// qb.OnlyTrashed() // includes only soft deleted records in the query results
func (qb *MongoQueryBuilder[T]) OnlyTrashed() *MongoQueryBuilder[T] {
	qb.onlyTrashed = true
	qb.withTrashed = false

	model := qb.service.Factory()
	if model.IsSoftDeletes() {
		deletedAtCol := model.GetDeletedAtColumn()
		qb.filter[deletedAtCol] = bson.M{"$exists": true}
	}

	return qb
}

// ========================
// Scopes (Laravel-style)
// ========================

// Scope applies a named scope to the query builder. A scope is a function
// that takes a MongoQueryBuilder and returns a modified MongoQueryBuilder.
// This allows for reusable query logic that can be applied to different
// queries. The scope function is executed with the current query builder
// instance as its argument.
//
// Parameters:
// - name: A string representing the name of the scope.
// - scope: A function that modifies the query builder.
//
// Returns:
// A reference to the modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) Scope(name string, scope func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	return scope(qb)
}

// When applies the given callback if the condition is true.
//
// Parameters:
//   - condition: A boolean indicating whether to apply the callback.
//   - callback: A function that takes a MongoQueryBuilder as an argument and
//     returns a modified MongoQueryBuilder.
//
// Returns:
// A reference to the modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) When(condition bool, callback func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	if condition {
		return callback(qb)
	}
	return qb
}

// Unless applies the given callback if the condition is false.
//
// Parameters:
//   - condition: A boolean indicating whether to apply the callback.
//   - callback: A function that takes a MongoQueryBuilder as an argument and
//     returns a modified MongoQueryBuilder.
//
// Returns:
// A reference to the modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) Unless(condition bool, callback func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	if !condition {
		return callback(qb)
	}
	return qb
}

// ========================
// Result Retrieval (Laravel-style)
// ========================

// Get retrieves the documents from the MongoDB collection based on the
// query builder's filter and options such as sort, limit, skip, and projection.
//
// It executes the query by applying the specified options to the Find operation
// and iterates over the resulting cursor to decode the documents into models.
// The Retrieved hook is executed for each successfully decoded model.
//
// Returns a slice of models of type T and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) Get() ([]T, error) {
	opts := options.Find()

	if len(qb.sort) > 0 {
		opts.SetSort(qb.sort)
	}
	if qb.limit > 0 {
		opts.SetLimit(qb.limit)
	}
	if qb.skip > 0 {
		opts.SetSkip(qb.skip)
	}
	if len(qb.projection) > 0 {
		opts.SetProjection(qb.projection)
	}

	cursor, err := qb.service.Collection.Find(qb.service.Ctx, qb.filter, opts)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, qb.service.Ctx)

	var results []T
	for cursor.Next(qb.service.Ctx) {
		model := qb.service.Factory()

		if err := cursor.Decode(&model); err != nil {
			continue
		}

		// Execute Retrieved hook
		if qb.service.Retrieved != nil {
			if err := qb.service.Retrieved(model); err != nil {
				continue
			}
		}

		results = append(results, model)
	}

	return results, cursor.Err()
}

// First retrieves the first document from the MongoDB collection based on the
// query builder's filter and options such as sort, limit, skip, and projection.
//
// It executes the query by applying the specified options to the FindOne
// operation and decodes the resulting document into a model. The Retrieved
// hook is executed for the successfully decoded model.
//
// If no document is found, First returns a zero value of type T and an error.
//
// Returns a model of type T and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) First() (T, error) {
	var zero T
	results, err := qb.Take(1).Get()
	if err != nil || len(results) == 0 {
		return zero, errors.New("no records found")
	}
	return results[0], nil
}

// FirstOrFail retrieves the first document from the MongoDB collection based on
// the query builder's filter and options such as sort, limit, skip, and
// projection. If no document is found, FirstOrFail returns an error with the
// message "model not found".
//
// It executes the query by applying the specified options to the FindOne
// operation and decodes the resulting document into a model. The Retrieved
// hook is executed for the successfully decoded model.
//
// Returns a model of type T and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) FirstOrFail() (T, error) {
	result, err := qb.First()
	if err != nil {
		return result, errors.New("model not found")
	}
	return result, nil
}

// Find retrieves the document from the MongoDB collection by its _id field.
// It automatically converts string IDs to primitive.ObjectID.
//
// If the ID is not a string or a primitive.ObjectID, it is passed directly
// to the Where method.
//
// Parameters:
// - id: The ID of the document to retrieve.
//
// Returns:
// A model of type T and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) Find(id interface{}) (T, error) {
	// Handle both string and ObjectID
	var objectID primitive.ObjectID
	var err error

	switch v := id.(type) {
	case string:
		objectID, err = primitive.ObjectIDFromHex(v)
		if err != nil {
			return qb.Where("_id", id).First()
		}
	case primitive.ObjectID:
		objectID = v
	default:
		return qb.Where("_id", id).First()
	}

	return qb.Where("_id", objectID).First()
}

// FindOrFail retrieves the document from the MongoDB collection by its _id field
// and returns the model and an error if the operation fails. If the document is
// not found, it returns an error.
//
// Parameters:
// - id: The ID of the document to retrieve.
//
// Returns:
// A model of type T and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) FindOrFail(id interface{}) (T, error) {
	result, err := qb.Find(id)
	if err != nil {
		return result, fmt.Errorf("no query results for model with id: %v", id)
	}
	return result, nil
}

// FindMany retrieves multiple documents from the MongoDB collection by their _id
// field. It is equivalent to calling WhereIn("_id", ids) and then calling Get.
//
// Parameters:
// - ids: The IDs of the documents to retrieve.
//
// Returns:
// A slice of models of type T and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) FindMany(ids []interface{}) ([]T, error) {
	return qb.WhereIn("_id", ids).Get()
}

// ========================
// Aggregates (Laravel-style)
// ========================

// Count returns the number of documents that match the query filter.
//
// Returns:
// An int64 of the number of documents that match the query filter, and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) Count() (int64, error) {
	return qb.service.Collection.CountDocuments(qb.service.Ctx, qb.filter)
}

// Max returns the maximum value of a given field in the documents that match the
// query filter. This method is equivalent to calling Aggregate with a pipeline
// that contains a $match stage with the query filter and a $group stage with a
// $max expression.
//
// Parameters:
// - field: The name of the field in which to find the maximum value.
//
// Returns:
// An interface{} that contains the maximum value of the given field, and an error
// if the operation fails.
func (qb *MongoQueryBuilder[T]) Max(field string) (interface{}, error) {
	pipeline := []bson.M{
		{"$match": qb.filter},
		{"$group": bson.M{
			"_id": nil,
			"max": bson.M{"$max": "$" + field},
		}},
	}

	cursor, err := qb.service.Collection.Aggregate(qb.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, qb.service.Ctx)

	var result struct {
		Max interface{} `bson:"max"`
	}

	if cursor.Next(qb.service.Ctx) {
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
		return result.Max, nil
	}

	return nil, nil
}

// Min returns the minimum value of a given field in the documents that match the
// query filter. This method is equivalent to calling Aggregate with a pipeline
// that contains a $match stage with the query filter and a $group stage with a
// $min expression.
//
// Parameters:
// - field: The name of the field in which to find the minimum value.
//
// Returns:
// An interface{} that contains the minimum value of the given field, and an error
// if the operation fails.
func (qb *MongoQueryBuilder[T]) Min(field string) (interface{}, error) {
	pipeline := []bson.M{
		{"$match": qb.filter},
		{"$group": bson.M{
			"_id": nil,
			"min": bson.M{"$min": "$" + field},
		}},
	}

	cursor, err := qb.service.Collection.Aggregate(qb.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, qb.service.Ctx)

	var result struct {
		Min interface{} `bson:"min"`
	}

	if cursor.Next(qb.service.Ctx) {
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
		return result.Min, nil
	}

	return nil, nil
}

// Sum returns the sum of a given field in the documents that match the query filter.
//
// Parameters:
// - field: The name of the field in which to find the sum.
//
// Returns:
// An interface{} that contains the sum of the given field, and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) Sum(field string) (interface{}, error) {
	pipeline := []bson.M{
		{"$match": qb.filter},
		{"$group": bson.M{
			"_id": nil,
			"sum": bson.M{"$sum": "$" + field},
		}},
	}

	cursor, err := qb.service.Collection.Aggregate(qb.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, qb.service.Ctx)

	var result struct {
		Sum interface{} `bson:"sum"`
	}

	if cursor.Next(qb.service.Ctx) {
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
		return result.Sum, nil
	}

	return 0, nil
}

// Avg returns the average value of a given field in the documents that match the
// query filter. This method is equivalent to calling Aggregate with a pipeline
// that contains a $match stage with the query filter and a $group stage with a
// $avg expression.
//
// Parameters:
// - field: The name of the field in which to find the average value.
//
// Returns:
// An interface{} that contains the average value of the given field, and an error
// if the operation fails.
func (qb *MongoQueryBuilder[T]) Avg(field string) (interface{}, error) {
	pipeline := []bson.M{
		{"$match": qb.filter},
		{"$group": bson.M{
			"_id": nil,
			"avg": bson.M{"$avg": "$" + field},
		}},
	}

	cursor, err := qb.service.Collection.Aggregate(qb.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, qb.service.Ctx)

	var result struct {
		Avg interface{} `bson:"avg"`
	}

	if cursor.Next(qb.service.Ctx) {
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
		return result.Avg, nil
	}

	return 0, nil
}

// Exists returns true if there are any documents that match the query filter,
// and false otherwise.
//
// Returns:
// A boolean indicating whether there are any matching documents, and an error if
// the operation fails.
func (qb *MongoQueryBuilder[T]) Exists() (bool, error) {
	count, err := qb.Count()
	return count > 0, err
}

// DoesntExist returns true if there are no documents that match the query filter,
// and false otherwise. It is equivalent to calling Exists and inverting the
// result.
//
// Returns:
// A boolean indicating whether there are no matching documents, and an error if
// the operation fails.
func (qb *MongoQueryBuilder[T]) DoesntExist() (bool, error) {
	exists, err := qb.Exists()
	return !exists, err
}

// ========================
// Pagination (Laravel-style)
// ========================

// PaginationResult represents the result of a pagination operation.
// It contains the paginated data, the current page, the last page, the number
// of items per page, the total number of items, and the range of items in the
// current page.
// The CurrentPage field is 0-indexed.
// The LastPage field is 0-indexed.
// The PerPage field is 0-indexed.
// The Total field is 0-indexed.
// The From field is 0-indexed.
// The To field is 0-indexed.
type PaginationResult[T GlobalModels.ORMModel] struct {
	Data        []T   `json:"data"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	PerPage     int   `json:"per_page"`
	Total       int64 `json:"total"`
	From        int   `json:"from"`
	To          int   `json:"to"`
}

// Paginate retrieves a subset of documents from the collection based on the specified
// page and perPage parameters, and returns a PaginationResult containing the paginated data.
// This method calculates the total number of documents, the last page, and the range of items
// on the current page. If page or perPage are less than 1, they will default to 1 and 15 respectively.
//
// Parameters:
// - page: The current page number (1-indexed).
// - perPage: The number of items to display per page.
//
// Returns:
// A pointer to a PaginationResult containing the paginated data and metadata, and an error if
// the operation fails.
func (qb *MongoQueryBuilder[T]) Paginate(page, perPage int) (*PaginationResult[T], error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	// Get total count
	total, err := qb.Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination
	skip := (page - 1) * perPage
	lastPage := int((total + int64(perPage) - 1) / int64(perPage))

	// Get paginated results
	results, err := qb.Skip(skip).Take(perPage).Get()
	if err != nil {
		return nil, err
	}

	from := skip + 1
	to := skip + len(results)
	if total == 0 {
		from = 0
	}

	return &PaginationResult[T]{
		Data:        results,
		CurrentPage: page,
		LastPage:    lastPage,
		PerPage:     perPage,
		Total:       total,
		From:        from,
		To:          to,
	}, nil
}

// SimplePaginate retrieves a subset of documents from the collection based on the specified
// page and perPage parameters, and returns the paginated data as a slice of T.
// This method does not return any pagination metadata, and is suitable for use
// cases where only the paginated data is required. If page or perPage are less
// than 1, they will default to 1 and 15 respectively.
//
// Parameters:
// - page: The current page number (1-indexed).
// - perPage: The number of items to display per page.
//
// Returns:
// A slice of T containing the paginated data, and an error if the operation fails.
func (qb *MongoQueryBuilder[T]) SimplePaginate(page, perPage int) ([]T, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	skip := (page - 1) * perPage
	return qb.Skip(skip).Take(perPage + 1).Get() // +1 to check if there are more records
}

// ========================
// Chunking (Laravel-style)
// ========================

// Chunk retrieves subsets of documents from the collection in chunks of a specified size
// and passes each chunk to a callback function for processing. This method continues
// retrieving and processing chunks until there are no more documents to process or
// the callback function returns an error.
//
// Parameters:
//   - size: The number of documents to include in each chunk. It must be greater than 0.
//   - callback: A function that processes each chunk of documents. The function is called
//     with a slice of documents and should return an error if processing fails.
//
// Returns:
// An error if the chunk size is less than or equal to 0, if an error occurs while
// retrieving documents, or if the callback function returns an error.
func (qb *MongoQueryBuilder[T]) Chunk(size int, callback func([]T) error) error {
	if size <= 0 {
		return errors.New("chunk size must be greater than 0")
	}

	skip := 0
	for {
		results, err := qb.Skip(skip).Take(size).Get()
		if err != nil {
			return err
		}

		if len(results) == 0 {
			break
		}

		if err := callback(results); err != nil {
			return err
		}

		if len(results) < size {
			break
		}

		skip += size
	}

	return nil
}

// Each iterates over each document in the collection and applies the given callback function.
// It uses chunking internally to efficiently handle large datasets by processing them in chunks.
//
// Parameters:
//   - callback: A function that processes each document. It is called with a single document
//     and should return an error if processing fails.
//
// Returns:
// An error if an error occurs while retrieving documents or if the callback function returns an error.
func (qb *MongoQueryBuilder[T]) Each(callback func(T) error) error {
	return qb.Chunk(1000, func(models []T) error {
		for _, model := range models {
			if err := callback(model); err != nil {
				return err
			}
		}
		return nil
	})
}

// ========================
// Updates and Deletes (Laravel-style)
// ========================

// Update updates the documents in the collection that match the filter criteria with the given values.
//
// Parameters:
// - values: A map of key-value pairs of fields to update and their values.
//
// Returns:
// The number of documents updated and an error if an error occurs.
func (qb *MongoQueryBuilder[T]) Update(values bson.M) (int64, error) {
	if len(qb.filter) == 0 && !qb.withTrashed && !qb.onlyTrashed {
		return 0, errors.New("update requires filter for safety")
	}

	// Add updated_at timestamp
	model := qb.service.Factory()
	if model.IsTimestamps() {
		values[model.GetUpdatedAtColumn()] = time.Now()
	}

	// Filter attributes
	filteredValues := qb.service.filterAttributes(values)

	updateDoc := bson.M{"$set": filteredValues}
	result, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, updateDoc)
	if err != nil {
		return 0, err
	}

	return result.ModifiedCount, nil
}

// Increment increments a field in the documents that match the filter criteria by the given amount.
//
// Parameters:
// - field: The name of the field to increment.
// - amount: The amount to increment the field by (default: 1).
//
// Returns:
// The number of documents updated and an error if an error occurs.
func (qb *MongoQueryBuilder[T]) Increment(field string, amount ...int) (int64, error) {
	value := 1
	if len(amount) > 0 {
		value = amount[0]
	}

	updateDoc := bson.M{"$inc": bson.M{field: value}}

	// Add updated_at timestamp
	model := qb.service.Factory()
	if model.IsTimestamps() {
		updateDoc["$set"] = bson.M{model.GetUpdatedAtColumn(): time.Now()}
	}

	result, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, updateDoc)
	if err != nil {
		return 0, err
	}

	return result.ModifiedCount, nil
}

// Decrement decrements a field in the documents that match the filter criteria by the given amount.
//
// Parameters:
// - field: The name of the field to decrement.
// - amount: The amount to decrement the field by (default: 1).
//
// Returns:
// The number of documents updated and an error if an error occurs.
func (qb *MongoQueryBuilder[T]) Decrement(field string, amount ...int) (int64, error) {
	value := 1
	if len(amount) > 0 {
		value = amount[0]
	}

	updateDoc := bson.M{"$inc": bson.M{field: -value}}

	// Add updated_at timestamp
	model := qb.service.Factory()
	if model.IsTimestamps() {
		updateDoc["$set"] = bson.M{model.GetUpdatedAtColumn(): time.Now()}
	}

	result, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, updateDoc)
	if err != nil {
		return 0, err
	}

	return result.ModifiedCount, nil
}

// Delete removes documents from the collection based on the filter criteria.
// If the model supports soft deletes and forceDelete is not enabled, it performs
// a soft delete by updating the deleted_at column with the current timestamp.
// Otherwise, it performs a hard delete by removing the documents from the collection.
//
// Returns:
// The number of documents deleted and an error if an error occurs.
func (qb *MongoQueryBuilder[T]) Delete() (int64, error) {
	if len(qb.filter) == 0 && !qb.withTrashed && !qb.onlyTrashed {
		return 0, errors.New("delete requires filter for safety")
	}

	model := qb.service.Factory()

	// Soft delete if enabled
	if model.IsSoftDeletes() && !qb.forceDelete {
		updates := bson.M{
			model.GetDeletedAtColumn(): time.Now(),
		}
		return qb.Update(updates)
	}

	// Hard delete
	result, err := qb.service.Collection.DeleteMany(qb.service.Ctx, qb.filter)
	if err != nil {
		return 0, err
	}

	return result.DeletedCount, nil
}

// ForceDelete performs a hard delete by removing the documents from the collection,
// even if the model supports soft deletes.
//
// This method is useful when you want to permanently delete the documents from the
// database, rather than just marking them as deleted.
//
// Parameters:
// None
//
// Returns:
// The number of documents deleted and an error if an error occurs.
func (qb *MongoQueryBuilder[T]) ForceDelete() (int64, error) {
	qb.forceDelete = true
	return qb.Delete()
}

// Restore performs a soft restore on the documents that match the filter criteria,
// by setting the deleted_at column to null.
//
// Parameters:
// None
//
// Returns:
// The number of documents restored and an error if an error occurs.
func (qb *MongoQueryBuilder[T]) Restore() (int64, error) {
	model := qb.service.Factory()
	if !model.IsSoftDeletes() {
		return 0, errors.New("model does not support soft deletes")
	}

	updateDoc := bson.M{"$unset": bson.M{model.GetDeletedAtColumn(): ""}}
	result, err := qb.service.Collection.UpdateMany(qb.service.Ctx, qb.filter, updateDoc)
	if err != nil {
		return 0, err
	}

	return result.ModifiedCount, nil
}

// ========================
// Eloquent Model Methods (Laravel-style)
// ========================

// All retrieves all records from the database for the given model type.
// If specific fields are provided, only those fields are selected.
//
// Parameters:
// - fields: An optional list of field names to select.
//
// Returns:
// A slice of model instances and an error if an error occurs.
func (s *BaseEloquentService[T]) All(fields ...string) ([]T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.Get()
}

// Find retrieves a document from the MongoDB collection by its _id field.
//
// Parameters:
// - id: The ID of the document to retrieve.
// - fields: An optional list of field names to include in the query result.
//
// Returns:
// A model of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) Find(id interface{}, fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.Find(id)
}

// FindOrFail retrieves a document from the database using the provided ID.
// If the document is not found, it returns an error.
//
// Parameters:
// - id: The ID of the document to retrieve.
// - fields: An optional list of field names to include in the query result.
//
// Returns:
// A model of type T and an error if the operation fails or if the document is not found.
func (s *BaseEloquentService[T]) FindOrFail(id interface{}, fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.FindOrFail(id)
}

// FindMany retrieves multiple documents from the MongoDB collection by their _id field.
//
// Parameters:
// - ids: A slice of identifiers for the documents to retrieve.
// - fields: An optional list of field names to include in the query result.
//
// Returns:
// A slice of models of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) FindMany(ids []interface{}, fields ...string) ([]T, error) {
	qb := s.NewQuery().WhereIn("_id", ids)
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.Get()
}

// First retrieves the first document from the MongoDB collection.
//
// Parameters:
// - fields: An optional list of field names to include in the query result.
//
// Returns:
// A model of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) First(fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.First()
}

// FirstOrFail retrieves the first document from the MongoDB collection. If no
// document is found, it returns an error with the message "model not found".
//
// Parameters:
// - fields: An optional list of field names to include in the query result.
//
// Returns:
// A model of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) FirstOrFail(fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.FirstOrFail()
}

// FirstOrCreate retrieves the first document from the MongoDB collection
// matching the given attributes. If no document is found, it creates a new
// document with the given attributes and values.
//
// Parameters:
//   - attributes: A map of field names and values to search for in the
//     collection.
//   - values: An optional map of field names and values to include in the
//     created document if no document is found.
//
// Returns:
// A model of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) FirstOrCreate(attributes bson.M, values ...bson.M) (T, error) {
	qb := s.NewQuery()
	for key, value := range attributes {
		qb = qb.Where(key, value)
	}

	model, err := qb.First()
	if err == nil {
		return model, nil
	}

	// Create new model
	createData := make(bson.M)
	for k, v := range attributes {
		createData[k] = v
	}

	if len(values) > 0 {
		for k, v := range values[0] {
			createData[k] = v
		}
	}

	return s.Create(createData)
}

// FirstOrNew retrieves the first document from the MongoDB collection
// matching the given attributes. If no document is found, it creates a new
// instance of the model with the given attributes and values. The boolean
// return value indicates whether a new instance was created.
//
// Parameters:
//   - attributes: A map of field names and values to search for in the
//     collection.
//   - values: An optional map of field names and values to include in the
//     created document if no document is found.
//
// Returns:
// A model of type T, a boolean indicating whether a new instance was created,
// and an error if the operation fails.
func (s *BaseEloquentService[T]) FirstOrNew(attributes bson.M, values ...bson.M) (T, bool, error) {
	qb := s.NewQuery()
	for key, value := range attributes {
		qb = qb.Where(key, value)
	}

	model, err := qb.First()
	if err == nil {
		return model, false, nil // Found existing
	}

	// Create new instance (not saved)
	newModel := s.Factory()
	createData := make(bson.M)
	for k, v := range attributes {
		createData[k] = v
	}

	if len(values) > 0 {
		for k, v := range values[0] {
			createData[k] = v
		}
	}

	err = s.FromMap(&newModel, createData)
	return newModel, true, err // New instance
}

// UpdateOrCreate retrieves the first document from the MongoDB collection
// matching the given attributes. If no document is found, it creates a new
// document with the given attributes and values.
//
// Parameters:
//   - attributes: A map of field names and values to search for in the
//     collection.
//   - values: A map of field names and values to include in the updated or
//     created document.
//
// Returns:
// A model of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) UpdateOrCreate(attributes bson.M, values bson.M) (T, error) {
	qb := s.NewQuery()
	for key, value := range attributes {
		qb = qb.Where(key, value)
	}

	model, err := qb.First()
	if err == nil {
		// Update existing
		modelMap, _ := s.ToMap(model)
		id := modelMap["_id"]
		_, updateErr := s.NewQuery().Where("_id", id).Update(values)
		if updateErr != nil {
			return model, updateErr
		}
		// Reload model
		return s.Find(id)
	}

	// Create new
	createData := make(bson.M)
	for k, v := range attributes {
		createData[k] = v
	}
	for k, v := range values {
		createData[k] = v
	}

	return s.Create(createData)
}

// ========================
// Create, Update, Save (Laravel-style)
// ========================

// Create creates a new document in the MongoDB collection with the given
// attributes. It automatically adds timestamps if the model has timestamps
// enabled. It also executes the Creating and Saving hooks. Finally, it
// hydrates the model and executes the Created and Saved hooks.
//
// Parameters:
//   - attributes: A map of field names and values to include in the created
//     document.
//
// Returns:
// A model of type T and an error if the operation fails.
func (s *BaseEloquentService[T]) Create(attributes bson.M) (T, error) {
	model := s.Factory()

	// Add timestamps
	if model.IsTimestamps() {
		now := time.Now()
		attributes[model.GetCreatedAtColumn()] = now
		attributes[model.GetUpdatedAtColumn()] = now
	}

	// Execute Creating hook
	if s.Creating != nil {
		if err := s.Creating(model); err != nil {
			return model, err
		}
	}

	// Execute Saving hook
	if s.Saving != nil {
		if err := s.Saving(model); err != nil {
			return model, err
		}
	}

	// Filter fillable/guarded attributes
	filteredAttrs := s.filterAttributes(attributes)

	// Insert document
	result, err := s.Collection.InsertOne(s.Ctx, filteredAttrs)
	if err != nil {
		return model, err
	}

	// Add the generated ID
	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		filteredAttrs["_id"] = oid
	}

	// Hydrate model
	if err := s.FromMap(&model, filteredAttrs); err != nil {
		return model, err
	}

	// Execute Created hook
	if s.Created != nil {
		if err := s.Created(model); err != nil {
			return model, err
		}
	}

	// Execute Saved hook
	if s.Saved != nil {
		if err := s.Saved(model); err != nil {
			return model, err
		}
	}

	return model, nil
}

// Save persists the model to the database.
//
// If the model has an ID, it updates the existing document.
// Otherwise, it creates a new document.
//
// It executes the following hooks in order:
// - Updating (if the model has an ID)
// - Saving
// - Updated (if the model has an ID)
// - Saved
//
// It returns the updated model and an error if any.
func (s *BaseEloquentService[T]) Save(model T) (T, error) {
	modelMap, err := s.ToMap(model)
	if err != nil {
		return model, err
	}

	// Check if model exists (has _id)
	if id, exists := modelMap["_id"]; exists && id != nil {
		// Update existing
		delete(modelMap, "_id") // Don't update ID

		if model.IsTimestamps() {
			modelMap[model.GetUpdatedAtColumn()] = time.Now()
		}

		// Execute Updating hook
		if s.Updating != nil {
			if err := s.Updating(model); err != nil {
				return model, err
			}
		}

		// Execute Saving hook
		if s.Saving != nil {
			if err := s.Saving(model); err != nil {
				return model, err
			}
		}

		filteredAttrs := s.filterAttributes(modelMap)
		_, err := s.NewQuery().Where("_id", id).Update(filteredAttrs)
		if err != nil {
			return model, err
		}

		// Execute Updated hook
		if s.Updated != nil {
			if err := s.Updated(model); err != nil {
				return model, err
			}
		}

		// Execute Saved hook
		if s.Saved != nil {
			if err := s.Saved(model); err != nil {
				return model, err
			}
		}

		// Reload model
		return s.Find(id)
	} else {
		// Create new
		return s.Create(modelMap)
	}
}

// Destroy permanently deletes the given model(s) by ID(s). It executes the
// Deleting hook before deletion and the Deleted hook after deletion. If any
// error occurs during deletion, it continues to the next ID.
//
// Parameters:
// - ids: The IDs of the models to delete.
//
// Returns:
// The number of deleted models and an error if any.
func (s *BaseEloquentService[T]) Destroy(ids ...interface{}) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	var totalDeleted int64

	for _, id := range ids {
		// Execute Deleting hook
		model, err := s.Find(id)
		if err != nil {
			continue
		}

		if s.Deleting != nil {
			if err := s.Deleting(model); err != nil {
				continue
			}
		}

		deleted, err := s.NewQuery().Where("_id", id).Delete()
		if err != nil {
			continue
		}

		totalDeleted += deleted

		// Execute Deleted hook
		if s.Deleted != nil {
			if err := s.Deleted(model); err != nil {
				continue
			}
		}
	}

	return totalDeleted, nil
}

// ========================
// Helper Methods
// ========================

// filterAttributes filters out the given attributes by checking if they are
// guarded or fillable. It iterates over the given attributes and adds them to
// the filtered map if they are not guarded and are fillable. If a field is
// guarded or not fillable, it is skipped. The "*" wildcard can be used for
// guarded and fillable to include or exclude all fields.
//
// Parameters:
// - attributes: The attributes to filter.
//
// Returns:
// A filtered map of attributes.
func (s *BaseEloquentService[T]) filterAttributes(attributes bson.M) bson.M {
	model := s.Factory()
	fillable := model.GetFillable()
	guarded := model.GetGuarded()

	filtered := make(bson.M)

	for key, value := range attributes {
		// Check guarded first
		if len(guarded) > 0 {
			isGuarded := false
			for _, guardedField := range guarded {
				if guardedField == key || guardedField == "*" {
					isGuarded = true
					break
				}
			}
			if isGuarded {
				continue
			}
		}

		// Check fillable
		if len(fillable) > 0 {
			isFillable := false
			for _, fillableField := range fillable {
				if fillableField == key || fillableField == "*" {
					isFillable = true
					break
				}
			}
			if !isFillable {
				continue
			}
		}

		filtered[key] = value
	}

	return filtered
}

// ========================
// Global Scopes
// ========================

// AddGlobalScope registers a global scope that will be applied to all queries
// executed by the service. The first argument is a unique name for the scope,
// and the second argument is a function that takes a MongoQueryBuilder and
// returns a modified MongoQueryBuilder. The scope can modify the query in
// any way, such as adding filters, joining other collections, or modifying
// the sort order. The scope will be applied before any other conditions are
// applied to the query.
func (s *BaseEloquentService[T]) AddGlobalScope(name string, scope func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) {
	s.GlobalScopes[name] = scope
}

// WithoutGlobalScope creates a new MongoQueryBuilder instance that will not
// apply the global scope with the given name. This is useful for creating
// queries that need to bypass a specific global scope.
//
// Parameters:
// - name: The name of the global scope to exclude.
//
// Returns:
// A new MongoQueryBuilder instance that will not apply the specified global
// scope.
func (s *BaseEloquentService[T]) WithoutGlobalScope(name string) *MongoQueryBuilder[T] {
	qb := &MongoQueryBuilder[T]{
		service:    s,
		filter:     bson.M{},
		sort:       bson.D{},
		limit:      0,
		skip:       0,
		projection: bson.M{},
		scopes:     []string{},
	}

	// Apply all global scopes except the specified one
	for scopeName, scope := range s.GlobalScopes {
		if scopeName != name {
			qb.scopes = append(qb.scopes, scopeName)
			qb = scope(qb)
		}
	}

	return qb
}

// WithoutGlobalScopes creates a new MongoQueryBuilder instance that will not
// apply any of the global scopes with the given names. This is useful for
// creating queries that need to bypass multiple global scopes.
//
// Parameters:
// - names: The names of the global scopes to exclude.
//
// Returns:
// A new MongoQueryBuilder instance that will not apply the specified global
// scopes.
func (s *BaseEloquentService[T]) WithoutGlobalScopes(names ...string) *MongoQueryBuilder[T] {
	qb := &MongoQueryBuilder[T]{
		service:    s,
		filter:     bson.M{},
		sort:       bson.D{},
		limit:      0,
		skip:       0,
		projection: bson.M{},
		scopes:     []string{},
	}

	// Create map of excluded scopes
	excluded := make(map[string]bool)
	for _, name := range names {
		excluded[name] = true
	}

	// Apply global scopes except excluded ones
	for scopeName, scope := range s.GlobalScopes {
		if !excluded[scopeName] {
			qb.scopes = append(qb.scopes, scopeName)
			qb = scope(qb)
		}
	}

	return qb
}

// ========================
// Utility Methods
// ========================

// ToMap converts the given model to a bson.M map, which can be used in MongoDB
// queries. This method is typically used when constructing queries that need to
// use the model's data. If the model cannot be converted to a bson.M map, an
// error is returned.
//
// Parameters:
// - model: The model to convert to a bson.M map.
//
// Returns:
// A bson.M map containing the model's data, or an error if the conversion failed.
func (s *BaseEloquentService[T]) ToMap(model T) (bson.M, error) {
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	var m bson.M
	err = json.Unmarshal(b, &m)
	return m, err
}

// FromMap takes a bson.M map and hydrates the given model with the values from
// the map. This method is typically used when constructing a model from a
// MongoDB query result. If the model cannot be hydrated from the given map, an
// error is returned.
//
// Parameters:
// - model: The model to hydrate with the given map.
// - data: The bson.M map containing the data to hydrate the model with.
//
// Returns:
// An error if the model could not be hydrated, or nil if the model was hydrated
// successfully.
func (s *BaseEloquentService[T]) FromMap(model *T, data bson.M) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, model)
}

// Fresh retrieves a fresh instance of the given model from the MongoDB
// collection. The model must have an ID set, or an error is returned.
//
// Parameters:
// - model: The model to retrieve a fresh instance of.
//
// Returns:
// A fresh instance of the model, or an error if the model does not have an ID.
func (s *BaseEloquentService[T]) Fresh(model T) (T, error) {
	modelMap, err := s.ToMap(model)
	if err != nil {
		return model, err
	}

	id, exists := modelMap["_id"]
	if !exists || id == nil {
		return model, errors.New("model has no ID")
	}

	return s.Find(id)
}

// Refresh reloads the given model from the MongoDB collection, replacing the
// current attributes with the latest values from the database. If the model
// does not have an ID, an error is returned. If the model cannot be refreshed,
// an error is also returned.
//
// Parameters:
// - model: The model to refresh.
//
// Returns:
// An error if the model could not be refreshed, or nil if the model was
// refreshed successfully.
func (s *BaseEloquentService[T]) Refresh(model *T) error {
	fresh, err := s.Fresh(*model)
	if err != nil {
		return err
	}
	*model = fresh
	return nil
}

// ========================
// Collection-style Methods
// ========================

// Pluck retrieves a map of field values from the database records.
// It selects the specified field and, optionally, a key field to use as the map's keys.
//
// Parameters:
// - field: The name of the field to pluck values from.
// - key: Optional. The name of the field to use as keys in the returned map.
//
// Returns:
// A map where each key is derived from the key field (if provided) or the result index,
// and each value is the corresponding field value. An error is returned if the query fails.
func (s *BaseEloquentService[T]) Pluck(field string, key ...string) (map[interface{}]interface{}, error) {
	var selectFields []string
	if len(key) > 0 {
		selectFields = []string{key[0], field}
	} else {
		selectFields = []string{field}
	}

	results, err := s.NewQuery().Select(selectFields...).Get()
	if err != nil {
		return nil, err
	}

	plucked := make(map[interface{}]interface{})

	for i, result := range results {
		resultMap, err := s.ToMap(result)
		if err != nil {
			continue
		}

		value := resultMap[field]

		if len(key) > 0 {
			keyValue := resultMap[key[0]]
			plucked[keyValue] = value
		} else {
			plucked[i] = value
		}
	}

	return plucked, nil
}

// ========================
// MongoDB-specific Methods
// ========================

// Aggregate performs an aggregation operation on the MongoDB collection using
// the specified pipeline. The pipeline is executed within the context of the
// service's current session and returns the resulting documents.
//
// Parameters:
//   - pipeline: A slice of bson.M representing the aggregation pipeline stages
//     to be executed on the collection.
//
// Returns:
// A slice of bson.M containing the results of the aggregation operation, or
// an error if the operation fails.
func (qb *MongoQueryBuilder[T]) Aggregate(pipeline []bson.M) ([]bson.M, error) {
	cursor, err := qb.service.Collection.Aggregate(qb.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, qb.service.Ctx)

	var results []bson.M
	if err = cursor.All(qb.service.Ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// WhereText adds a $text filter to the query. The $text filter uses the collection's
// text index to perform a full-text search on the specified field with the
// specified search term.
//
// Parameters:
// - searchTerm: The search term to search for in the collection.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereText(searchTerm string) *MongoQueryBuilder[T] {
	qb.filter["$text"] = bson.M{"$search": searchTerm}
	return qb
}

// WhereNear adds a $near filter to the query. The $near filter uses the
// collection's geospatial index to perform a radial search on the specified
// field with the specified longitude, latitude, and optional maximum distance.
//
// Parameters:
//   - field: The name of the field to search in the collection.
//   - lng: The longitude of the center point of the search circle.
//   - lat: The latitude of the center point of the search circle.
//   - maxDistance: An optional parameter that specifies the maximum distance
//     from the center point of the search circle. If omitted, the filter will
//     return all documents within the collection's geospatial index.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereNear(field string, lng, lat float64, maxDistance ...float64) *MongoQueryBuilder[T] {
	near := bson.M{
		"$geometry": bson.M{
			"type":        "Point",
			"coordinates": []float64{lng, lat},
		},
	}

	if len(maxDistance) > 0 {
		near["$maxDistance"] = maxDistance[0]
	}

	qb.filter[field] = bson.M{"$near": near}
	return qb
}

// WhereGeoWithin adds a $geoWithin filter to the query. The $geoWithin filter
// matches documents containing a field with geospatial data (a GeoJSON object)
// that falls within a specified shape.
//
// Parameters:
// - field: The name of the field to search in the collection.
// - geometry: A GeoJSON object that specifies the shape to search within.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereGeoWithin(field string, geometry bson.M) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$geoWithin": geometry}
	return qb
}

// WhereArrayContains adds a $in filter to the query. The $in filter matches
// documents containing an array field with at least one element that matches
// the specified value.
//
// Parameters:
// - field: The name of the array field to search in the collection.
// - value: The value to search for in the array field.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereArrayContains(field string, value interface{}) *MongoQueryBuilder[T] {
	qb.filter[field] = value
	return qb
}

// WhereArraySize adds a $size filter to the query. The $size filter matches
// documents containing an array field with a specified number of elements.
//
// Parameters:
// - field: The name of the array field to search in the collection.
// - size: The number of elements that the array field must contain.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereArraySize(field string, size int) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$size": size}
	return qb
}

// WhereElemMatch adds a $elemMatch filter to the query. The $elemMatch filter
// matches documents containing an array field with at least one element that
// matches the specified condition.
//
// Parameters:
//   - field: The name of the array field to search in the collection.
//   - condition: A document that specifies the condition that an element must
//     match in order to be included in the result set.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereElemMatch(field string, condition bson.M) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$elemMatch": condition}
	return qb
}

// WhereRegex adds a $regex filter to the query. The $regex filter matches
// documents containing a field with a string value that matches the specified
// regular expression pattern.
//
// Parameters:
//   - field: The name of the field to search in the collection.
//   - pattern: The regular expression pattern to match.
//   - options: An optional string of flags to modify the regular expression
//     match behavior. The available flags are:
//   - i: Perform case-insensitive matching.
//   - m: Perform multiline matching. Newlines are considered to be
//     part of the regular expression.
//   - x: Perform extended regular expression matching. Whitespace and
//     comments are ignored, and the regular expression may contain
//     line breaks.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereRegex(field string, pattern string, options ...string) *MongoQueryBuilder[T] {
	regex := bson.M{"$regex": pattern}
	if len(options) > 0 {
		regex["$options"] = options[0]
	}
	qb.filter[field] = regex
	return qb
}

// WhereType adds a $type filter to the query. The $type filter matches
// documents containing a field with the specified BSON type.
//
// Parameters:
//   - field: The name of the field to search in the collection.
//   - bsonType: The BSON type to match. See the BSON type constants in the
//     bson package for a list of valid values.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereType(field string, bsonType interface{}) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$type": bsonType}
	return qb
}

// WhereMod adds a $mod filter to the query. The $mod filter selects documents
// where the value of the field divided by the divisor has the specified remainder.
//
// Parameters:
// - field: The name of the field to apply the $mod filter.
// - divisor: The divisor in the modulo operation.
// - remainder: The expected remainder when the field value is divided by the divisor.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereMod(field string, divisor, remainder int) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$mod": []int{divisor, remainder}}
	return qb
}

// WhereJsonContains adds a filter to the query that matches documents where
// the specified JSON field contains a value at the given path. This is useful
// for querying nested JSON structures.
//
// Parameters:
// - field: The name of the JSON field to search in the collection.
// - path: The dot-separated path within the JSON field to search for the value.
// - value: The value to search for at the specified path within the JSON field.
//
// Returns:
// The modified MongoQueryBuilder.
func (qb *MongoQueryBuilder[T]) WhereJsonContains(field string, path string, value interface{}) *MongoQueryBuilder[T] {
	qb.filter[field+"."+path] = value
	return qb
}

// BulkWrite performs a bulk write of multiple write operations to the collection.
//
// Parameters:
//   - operations: A slice of mongo.WriteModel objects, each representing a write
//     operation to perform. The operations are executed in the order they are
//     specified in the slice.
//
// Returns:
// A mongo.BulkWriteResult containing information about the results of the
// bulk write operation, or an error if the operation failed.
func (s *BaseEloquentService[T]) BulkWrite(operations []mongo.WriteModel) (*mongo.BulkWriteResult, error) {
	return s.Collection.BulkWrite(s.Ctx, operations)
}

// CreateIndex creates a single index on the specified keys for the MongoDB
// collection. It allows for optional index options to be provided.
//
// Parameters:
//   - keys: A bson.D object specifying the index keys.
//   - options: Optional index options, such as unique constraints or expiration
//     times for TTL indexes. If provided, the first option will be used.
//
// Returns:
// A string representing the name of the created index, or an error if the
// index creation fails.
func (s *BaseEloquentService[T]) CreateIndex(keys bson.D, options ...*options.IndexOptions) (string, error) {
	indexModel := mongo.IndexModel{Keys: keys}
	if len(options) > 0 {
		indexModel.Options = options[0]
	}
	return s.Collection.Indexes().CreateOne(s.Ctx, indexModel)
}

// CreateIndexes creates multiple indexes on the MongoDB collection.
//
// Parameters:
//   - indexes: A slice of mongo.IndexModel objects, each representing an index
//     to be created on the collection.
//
// Returns:
// A slice of strings representing the names of the created indexes, or an
// error if any of the index creation operations fail.
func (s *BaseEloquentService[T]) CreateIndexes(indexes []mongo.IndexModel) ([]string, error) {
	return s.Collection.Indexes().CreateMany(s.Ctx, indexes)
}

// DropIndex removes a single index from the MongoDB collection.
//
// Parameters:
// - name: The name of the index to be dropped.
//
// Returns:
// An error if the index removal fails, or nil if the index is successfully removed.
func (s *BaseEloquentService[T]) DropIndex(name string) error {
	_, err := s.Collection.Indexes().DropOne(s.Ctx, name)
	return err
}

// ListIndexes retrieves a list of indexes for the MongoDB collection.
//
// Returns:
// A slice of bson.M objects, each representing an index on the collection,
// or an error if the index retrieval fails.
func (s *BaseEloquentService[T]) ListIndexes() ([]bson.M, error) {
	cursor, err := s.Collection.Indexes().List(s.Ctx)
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, s.Ctx)

	var indexes []bson.M
	if err = cursor.All(s.Ctx, &indexes); err != nil {
		return nil, err
	}

	return indexes, nil
}
