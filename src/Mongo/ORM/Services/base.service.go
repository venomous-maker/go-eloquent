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

// ========================
// BaseEloquentService (Laravel-inspired for MongoDB)
// ========================
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

// ========================
// Laravel-Style Query Builder for MongoDB
// ========================
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

// ========================
// Laravel-Style Query Builder Methods
// ========================
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

// Laravel Query() method
func (s *BaseEloquentService[T]) Query() *MongoQueryBuilder[T] {
	return s.NewQuery()
}

// ========================
// Where Clauses (Laravel-style)
// ========================
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

func (qb *MongoQueryBuilder[T]) WhereIn(column string, values []interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$in": values}
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereNotIn(column string, values []interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$nin": values}
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereBetween(column string, values [2]interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{
		"$gte": values[0],
		"$lte": values[1],
	}
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereNotBetween(column string, values [2]interface{}) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{
		"$not": bson.M{
			"$gte": values[0],
			"$lte": values[1],
		},
	}
	return qb
}

func (qb *MongoQueryBuilder[T]) whereNull(column string) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$exists": false}
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereNull(column string) *MongoQueryBuilder[T] {
	return qb.whereNull(column)
}

func (qb *MongoQueryBuilder[T]) WhereNotNull(column string) *MongoQueryBuilder[T] {
	qb.filter[column] = bson.M{"$exists": true}
	return qb
}

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

func (qb *MongoQueryBuilder[T]) WhereTime(column string, operator string, value interface{}) *MongoQueryBuilder[T] {
	// Extract time part using MongoDB aggregation
	// This is more complex in MongoDB - for now, we'll do a basic comparison
	return qb.Where(column, operator, value)
}

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

func (qb *MongoQueryBuilder[T]) WhereExists(subquery func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	// MongoDB doesn't have direct EXISTS - this would need to be implemented
	// based on specific use case using $lookup or other aggregation operators
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereRaw(filter bson.M) *MongoQueryBuilder[T] {
	for k, v := range filter {
		qb.filter[k] = v
	}
	return qb
}

// ========================
// Ordering (Laravel-style)
// ========================
func (qb *MongoQueryBuilder[T]) OrderBy(column string, direction ...string) *MongoQueryBuilder[T] {
	dir := 1 // ascending
	if len(direction) > 0 && strings.ToLower(direction[0]) == "desc" {
		dir = -1
	}

	qb.sort = append(qb.sort, bson.E{Key: column, Value: dir})
	return qb
}

func (qb *MongoQueryBuilder[T]) OrderByDesc(column string) *MongoQueryBuilder[T] {
	return qb.OrderBy(column, "desc")
}

func (qb *MongoQueryBuilder[T]) Latest(column ...string) *MongoQueryBuilder[T] {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return qb.OrderByDesc(col)
}

func (qb *MongoQueryBuilder[T]) Oldest(column ...string) *MongoQueryBuilder[T] {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return qb.OrderBy(col)
}

func (qb *MongoQueryBuilder[T]) InRandomOrder() *MongoQueryBuilder[T] {
	// MongoDB equivalent of random order using $sample in aggregation
	// This is a simplified version - in practice, you'd use aggregation pipeline
	return qb
}

// ========================
// Limiting and Offsetting
// ========================
func (qb *MongoQueryBuilder[T]) Take(count int) *MongoQueryBuilder[T] {
	qb.limit = int64(count)
	return qb
}

func (qb *MongoQueryBuilder[T]) Limit(count int) *MongoQueryBuilder[T] {
	return qb.Take(count)
}

func (qb *MongoQueryBuilder[T]) Skip(count int) *MongoQueryBuilder[T] {
	qb.skip = int64(count)
	return qb
}

func (qb *MongoQueryBuilder[T]) Offset(count int) *MongoQueryBuilder[T] {
	return qb.Skip(count)
}

// ========================
// Selecting Fields (Projection)
// ========================
func (qb *MongoQueryBuilder[T]) Select(fields ...string) *MongoQueryBuilder[T] {
	qb.projection = bson.M{}
	for _, field := range fields {
		if field != "*" {
			qb.projection[field] = 1
		}
	}
	return qb
}

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
func (qb *MongoQueryBuilder[T]) Scope(name string, scope func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	return scope(qb)
}

func (qb *MongoQueryBuilder[T]) When(condition bool, callback func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	if condition {
		return callback(qb)
	}
	return qb
}

func (qb *MongoQueryBuilder[T]) Unless(condition bool, callback func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) *MongoQueryBuilder[T] {
	if !condition {
		return callback(qb)
	}
	return qb
}

// ========================
// Result Retrieval (Laravel-style)
// ========================
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

func (qb *MongoQueryBuilder[T]) First() (T, error) {
	var zero T
	results, err := qb.Take(1).Get()
	if err != nil || len(results) == 0 {
		return zero, errors.New("no records found")
	}
	return results[0], nil
}

func (qb *MongoQueryBuilder[T]) FirstOrFail() (T, error) {
	result, err := qb.First()
	if err != nil {
		return result, errors.New("model not found")
	}
	return result, nil
}

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

func (qb *MongoQueryBuilder[T]) FindOrFail(id interface{}) (T, error) {
	result, err := qb.Find(id)
	if err != nil {
		return result, fmt.Errorf("no query results for model with id: %v", id)
	}
	return result, nil
}

func (qb *MongoQueryBuilder[T]) FindMany(ids []interface{}) ([]T, error) {
	return qb.WhereIn("_id", ids).Get()
}

// ========================
// Aggregates (Laravel-style)
// ========================
func (qb *MongoQueryBuilder[T]) Count() (int64, error) {
	return qb.service.Collection.CountDocuments(qb.service.Ctx, qb.filter)
}

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

func (qb *MongoQueryBuilder[T]) Exists() (bool, error) {
	count, err := qb.Count()
	return count > 0, err
}

func (qb *MongoQueryBuilder[T]) DoesntExist() (bool, error) {
	exists, err := qb.Exists()
	return !exists, err
}

// ========================
// Pagination (Laravel-style)
// ========================
type PaginationResult[T GlobalModels.ORMModel] struct {
	Data        []T   `json:"data"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	PerPage     int   `json:"per_page"`
	Total       int64 `json:"total"`
	From        int   `json:"from"`
	To          int   `json:"to"`
}

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

func (qb *MongoQueryBuilder[T]) ForceDelete() (int64, error) {
	qb.forceDelete = true
	return qb.Delete()
}

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
func (s *BaseEloquentService[T]) All(fields ...string) ([]T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.Get()
}

func (s *BaseEloquentService[T]) Find(id interface{}, fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.Find(id)
}

func (s *BaseEloquentService[T]) FindOrFail(id interface{}, fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.FindOrFail(id)
}

func (s *BaseEloquentService[T]) FindMany(ids []interface{}, fields ...string) ([]T, error) {
	qb := s.NewQuery().WhereIn("_id", ids)
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.Get()
}

func (s *BaseEloquentService[T]) First(fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.First()
}

func (s *BaseEloquentService[T]) FirstOrFail(fields ...string) (T, error) {
	qb := s.NewQuery()
	if len(fields) > 0 {
		qb = qb.Select(fields...)
	}
	return qb.FirstOrFail()
}

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
func (s *BaseEloquentService[T]) AddGlobalScope(name string, scope func(*MongoQueryBuilder[T]) *MongoQueryBuilder[T]) {
	s.GlobalScopes[name] = scope
}

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
func (s *BaseEloquentService[T]) ToMap(model T) (bson.M, error) {
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	var m bson.M
	err = json.Unmarshal(b, &m)
	return m, err
}

func (s *BaseEloquentService[T]) FromMap(model *T, data bson.M) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, model)
}

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

// Aggregation pipeline support
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

// Text search support
func (qb *MongoQueryBuilder[T]) WhereText(searchTerm string) *MongoQueryBuilder[T] {
	qb.filter["$text"] = bson.M{"$search": searchTerm}
	return qb
}

// Geospatial queries
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

func (qb *MongoQueryBuilder[T]) WhereGeoWithin(field string, geometry bson.M) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$geoWithin": geometry}
	return qb
}

// Array operations
func (qb *MongoQueryBuilder[T]) WhereArrayContains(field string, value interface{}) *MongoQueryBuilder[T] {
	qb.filter[field] = value
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereArraySize(field string, size int) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$size": size}
	return qb
}

func (qb *MongoQueryBuilder[T]) WhereElemMatch(field string, condition bson.M) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$elemMatch": condition}
	return qb
}

// Regex queries
func (qb *MongoQueryBuilder[T]) WhereRegex(field string, pattern string, options ...string) *MongoQueryBuilder[T] {
	regex := bson.M{"$regex": pattern}
	if len(options) > 0 {
		regex["$options"] = options[0]
	}
	qb.filter[field] = regex
	return qb
}

// Type checking
func (qb *MongoQueryBuilder[T]) WhereType(field string, bsonType interface{}) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$type": bsonType}
	return qb
}

// Modulo operation
func (qb *MongoQueryBuilder[T]) WhereMod(field string, divisor, remainder int) *MongoQueryBuilder[T] {
	qb.filter[field] = bson.M{"$mod": []int{divisor, remainder}}
	return qb
}

// JSON path queries (for nested documents)
func (qb *MongoQueryBuilder[T]) WhereJsonContains(field string, path string, value interface{}) *MongoQueryBuilder[T] {
	qb.filter[field+"."+path] = value
	return qb
}

// Bulk operations
func (s *BaseEloquentService[T]) BulkWrite(operations []mongo.WriteModel) (*mongo.BulkWriteResult, error) {
	return s.Collection.BulkWrite(s.Ctx, operations)
}

// Index operations
func (s *BaseEloquentService[T]) CreateIndex(keys bson.D, options ...*options.IndexOptions) (string, error) {
	indexModel := mongo.IndexModel{Keys: keys}
	if len(options) > 0 {
		indexModel.Options = options[0]
	}
	return s.Collection.Indexes().CreateOne(s.Ctx, indexModel)
}

func (s *BaseEloquentService[T]) CreateIndexes(indexes []mongo.IndexModel) ([]string, error) {
	return s.Collection.Indexes().CreateMany(s.Ctx, indexes)
}

func (s *BaseEloquentService[T]) DropIndex(name string) error {
	_, err := s.Collection.Indexes().DropOne(s.Ctx, name)
	return err
}

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
