package base

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	// added for caching
	"crypto/sha256"
	"encoding/hex"
	"sync"

	BaseModels "github.com/venomous-maker/go-eloquent/Models/Base"
	strlib "github.com/venomous-maker/go-eloquent/libs/strings"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// helper to detect if provided relation string is already a concrete collection name
// Heuristics: contains underscore and already plural (ends with s or es) OR contains a namespace like "cm_"
func isLikelyCollectionName(name string) bool {
	lower := strings.ToLower(name)
	if name != lower { // if it has uppercase letters, treat as model name, not collection name
		return false
	}
	if strings.Contains(lower, "_") && (strings.HasSuffix(lower, "s") || strings.HasSuffix(lower, "es")) {
		return true
	}
	return false
}

// resolveRelatedCollection decides the Mongo collection name for a relationship
// If caller already supplies a snake_case plural collection (e.g. "cm_covers") we keep it;
// otherwise we derive one from the provided model token.
func resolveRelatedCollection(raw string) string {
	if isLikelyCollectionName(raw) {
		return raw
	}
	// derive from model token
	return strlib.Pluralize(strlib.ConvertToSnakeCase(raw))
}

// ==================== In-memory TTL Cache (collection-scoped) ====================

type cacheEntry struct {
	payload []byte
	expiry  time.Time
}

type serviceCache struct {
	mu   sync.RWMutex
	data map[string]map[string]cacheEntry // collection -> key -> entry
}

func (c *serviceCache) get(coll, key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	bucket, ok := c.data[coll]
	if !ok {
		return nil, false
	}
	entry, ok := bucket[key]
	if !ok {
		return nil, false
	}
	if !entry.expiry.IsZero() && time.Now().After(entry.expiry) {
		return nil, false
	}
	return entry.payload, true
}

func (c *serviceCache) set(coll, key string, payload []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[string]map[string]cacheEntry)
	}
	bucket, ok := c.data[coll]
	if !ok {
		bucket = make(map[string]cacheEntry)
		c.data[coll] = bucket
	}
	bucket[key] = cacheEntry{payload: payload, expiry: time.Now().Add(ttl)}
}

func (c *serviceCache) invalidateCollection(coll string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		return
	}
	delete(c.data, coll)
}

// Eloquent is the main query builder struct similar to Laravel's Eloquent
type Eloquent[T BaseModels.MongoModel] struct {
	service        *EloquentService[T]
	wheres         []bson.M
	orWheres       []bson.M
	whereIns       map[string][]interface{}
	whereNotIns    map[string][]interface{}
	orders         []bson.E
	selectFields   []string
	limitNum       *int64
	skipNum        *int64
	relations      []Relation
	withCount      []string
	scopes         []func(*Eloquent[T]) *Eloquent[T]
	softDeleteMode int // 0=active(default),1=withTrashed,2=onlyTrashed
	cacheTTL       *time.Duration
}

const (
	softDeleteActive = iota
	softDeleteWith
	softDeleteOnly
)

// Relation represents a relationship configuration
type Relation struct {
	Name       string               // The relationship name
	Type       RelationType         // belongsTo, hasOne, hasMany, etc.
	Related    string               // Related collection name
	ForeignKey string               // Foreign key field
	LocalKey   string               // Local key field
	Conditions bson.M               // Additional conditions
	Pivot      *PivotConfig         // For many-to-many relationships
	Nested     map[string]*Relation // Nested relationships
	Required   bool                 // Inner join vs left join
}

type RelationType string

const (
	BelongsTo      RelationType = "belongsTo"
	HasOne         RelationType = "hasOne"
	HasMany        RelationType = "hasMany"
	BelongsToMany  RelationType = "belongsToMany"
	HasOneThrough  RelationType = "hasOneThrough"
	HasManyThrough RelationType = "hasManyThrough"
)

type PivotConfig struct {
	Table      string
	ForeignKey string
	RelatedKey string
	Fields     []string // Additional pivot fields to select
}

// Collection returns the collection name
func (e *Eloquent[T]) Collection() string {
	return e.service.GetCollectionName()
}

// Model returns a fresh model instance
func (e *Eloquent[T]) Model() T {
	return e.service.Factory()
}

// ==================== Query Building (Laravel Style) ====================

// Where adds a basic where clause
func (e *Eloquent[T]) Where(field string, operator interface{}, value ...interface{}) *Eloquent[T] {
	if len(value) == 0 {
		// Where("field", "value") - operator is actually the value
		e.wheres = append(e.wheres, bson.M{field: operator})
	} else {
		// Where("field", ">", value)
		op := fmt.Sprintf("%v", operator)
		val := value[0]

		switch op {
		case "=", "==":
			e.wheres = append(e.wheres, bson.M{field: val})
		case "!=", "<>":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$ne": val}})
		case ">":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$gt": val}})
		case ">=":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$gte": val}})
		case "<":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$lt": val}})
		case "<=":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$lte": val}})
		case "like":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$regex": val, "$options": "i"}})
		case "in":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$in": val}})
		case "not in":
			e.wheres = append(e.wheres, bson.M{field: bson.M{"$nin": val}})
		default:
			e.wheres = append(e.wheres, bson.M{field: bson.M{op: val}})
		}
	}
	return e
}

// OrWhere adds an OR where clause
func (e *Eloquent[T]) OrWhere(field string, operator interface{}, value ...interface{}) *Eloquent[T] {
	if len(value) == 0 {
		e.orWheres = append(e.orWheres, bson.M{field: operator})
	} else {
		op := fmt.Sprintf("%v", operator)
		val := value[0]
		switch op {
		case "=", "==":
			e.orWheres = append(e.orWheres, bson.M{field: val})
		default:
			e.orWheres = append(e.orWheres, bson.M{field: bson.M{op: val}})
		}
	}
	return e
}

// WhereIn adds a where in clause. Accepts any slice/array (e.g. []primitive.ObjectID, []string, []int).
func (e *Eloquent[T]) WhereIn(field string, values interface{}) *Eloquent[T] {
	if e.whereIns == nil {
		e.whereIns = make(map[string][]interface{})
	}
	e.whereIns[field] = toInterfaceSlice(values)
	return e
}

// WhereNotIn adds a where not in clause. Accepts any slice/array (e.g. []primitive.ObjectID, []string, []int).
func (e *Eloquent[T]) WhereNotIn(field string, values interface{}) *Eloquent[T] {
	if e.whereNotIns == nil {
		e.whereNotIns = make(map[string][]interface{})
	}
	e.whereNotIns[field] = toInterfaceSlice(values)
	return e
}

// toInterfaceSlice converts various slices/arrays to []interface{}; non-slice inputs are wrapped as single element
func toInterfaceSlice(v interface{}) []interface{} {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	// if already []interface{}
	if slice, ok := v.([]interface{}); ok {
		return slice
	}
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		length := rv.Len()
		out := make([]interface{}, 0, length)
		for i := 0; i < length; i++ {
			out = append(out, rv.Index(i).Interface())
		}
		return out
	}
	// fallback single value
	return []interface{}{v}
}

// WhereNull checks for null values
func (e *Eloquent[T]) WhereNull(field string) *Eloquent[T] {
	e.wheres = append(e.wheres, bson.M{field: nil})
	return e
}

// WhereNotNull checks for non-null values
func (e *Eloquent[T]) WhereNotNull(field string) *Eloquent[T] {
	e.wheres = append(e.wheres, bson.M{field: bson.M{"$ne": nil}})
	return e
}

// WhereBetween adds a between clause
func (e *Eloquent[T]) WhereBetween(field string, min, max interface{}) *Eloquent[T] {
	e.wheres = append(e.wheres, bson.M{field: bson.M{"$gte": min, "$lte": max}})
	return e
}

// WhereDate filters by date
func (e *Eloquent[T]) WhereDate(field string, operator string, date time.Time) *Eloquent[T] {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)

	switch operator {
	case "=":
		e.wheres = append(e.wheres, bson.M{field: bson.M{"$gte": startOfDay, "$lte": endOfDay}})
	case ">":
		e.wheres = append(e.wheres, bson.M{field: bson.M{"$gt": endOfDay}})
	case ">=":
		e.wheres = append(e.wheres, bson.M{field: bson.M{"$gte": startOfDay}})
	case "<":
		e.wheres = append(e.wheres, bson.M{field: bson.M{"$lt": startOfDay}})
	case "<=":
		e.wheres = append(e.wheres, bson.M{field: bson.M{"$lte": endOfDay}})
	}
	return e
}

// OrderBy adds ordering
func (e *Eloquent[T]) OrderBy(field string, direction ...string) *Eloquent[T] {
	dir := "asc"
	if len(direction) > 0 {
		dir = direction[0]
	}

	order := 1
	if strings.ToLower(dir) == "desc" {
		order = -1
	}

	e.orders = append(e.orders, bson.E{Key: field, Value: order})
	return e
}

// Latest orders by created_at desc
func (e *Eloquent[T]) Latest(field ...string) *Eloquent[T] {
	column := "created_at"
	if len(field) > 0 {
		column = field[0]
	}
	return e.OrderBy(column, "desc")
}

// Oldest orders by created_at asc
func (e *Eloquent[T]) Oldest(field ...string) *Eloquent[T] {
	column := "created_at"
	if len(field) > 0 {
		column = field[0]
	}
	return e.OrderBy(column, "asc")
}

// Select specifies fields to select
func (e *Eloquent[T]) Select(fields ...string) *Eloquent[T] {
	e.selectFields = fields
	return e
}

// Take limits the results
func (e *Eloquent[T]) Take(limit int) *Eloquent[T] {
	if limit > 0 {
		l := int64(limit)
		e.limitNum = &l
	}
	return e
}

// Limit is an alias for Take
func (e *Eloquent[T]) Limit(limit int) *Eloquent[T] {
	return e.Take(limit)
}

// Skip/Offset for pagination
func (e *Eloquent[T]) Skip(offset int) *Eloquent[T] {
	if offset > 0 {
		o := int64(offset)
		e.skipNum = &o
	}
	return e
}

func (e *Eloquent[T]) Offset(offset int) *Eloquent[T] {
	return e.Skip(offset)
}

// ==================== Relationships (Laravel Style) ====================

// With loads relationships (eager loading)
func (e *Eloquent[T]) With(relations ...string) *Eloquent[T] {
	for _, rel := range relations {
		// Parse nested relationships like "user.profile"
		parts := strings.Split(rel, ".")
		e.addRelation(parts, nil)
	}
	return e
}

// WithCount loads relationship counts
func (e *Eloquent[T]) WithCount(relations ...string) *Eloquent[T] {
	e.withCount = append(e.withCount, relations...)
	return e
}

// Load lazy loads relationships on existing models
func (e *Eloquent[T]) Load(model T, relations ...string) T {
	// Implementation would lazy load relationships
	return model
}

// addRelation helper to build relation tree
func (e *Eloquent[T]) addRelation(parts []string, parent *Relation) {
	if len(parts) == 0 {
		return
	}

	relName := parts[0]
	remaining := parts[1:]

	// Find or create relation
	var relation *Relation
	if parent == nil {
		// Top level relation
		for i := range e.relations {
			if e.relations[i].Name == relName {
				relation = &e.relations[i]
				break
			}
		}
		if relation == nil {
			newRel := e.inferRelation(relName)
			e.relations = append(e.relations, newRel)
			relation = &e.relations[len(e.relations)-1]
		}
	} else {
		// Nested relation
		if parent.Nested == nil {
			parent.Nested = make(map[string]*Relation)
		}
		if rel, exists := parent.Nested[relName]; exists {
			relation = rel
		} else {
			newRel := e.inferRelation(relName)
			parent.Nested[relName] = &newRel
			relation = parent.Nested[relName]
		}
	}

	// Continue with remaining parts
	if len(remaining) > 0 {
		e.addRelation(remaining, relation)
	}
}

// inferRelation attempts to infer relationship type and config
func (e *Eloquent[T]) inferRelation(name string) Relation {
	// This would be enhanced based on your model definitions
	// For now, simple heuristics
	modelName := e.getModelName()

	relation := Relation{
		Name:    name,
		Related: resolveRelatedCollection(name),
	}

	// Simple heuristics for relationship types
	if strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "Id") {
		relation.Type = BelongsTo
		relation.ForeignKey = name
		relation.LocalKey = "_id"
	} else if strlib.ConvertToSnakeCase(name) == strings.ToLower(modelName) {
		relation.Type = HasMany
		relation.ForeignKey = strlib.ConvertToSnakeCase(modelName) + "_id"
		relation.LocalKey = "_id"
	} else {
		// Default to hasMany
		relation.Type = HasMany
		relation.ForeignKey = strlib.ConvertToSnakeCase(modelName) + "_id"
		relation.LocalKey = "_id"
	}

	return relation
}

// getModelName gets the current model name
func (e *Eloquent[T]) getModelName() string {
	t := reflect.TypeOf(e.service.Factory())
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// ==================== Relationship Definition Methods ====================

// BelongsTo defines a belongs-to relationship
func (e *Eloquent[T]) BelongsTo(related, foreignKey, localKey string) *Eloquent[T] {
	if foreignKey == "" {
		foreignKey = strlib.ConvertToSnakeCase(related) + "_id"
	}
	if localKey == "" {
		localKey = "_id"
	}
	relation := Relation{
		Name:       related,
		Type:       BelongsTo,
		Related:    resolveRelatedCollection(related),
		ForeignKey: foreignKey,
		LocalKey:   localKey,
	}
	e.relations = append(e.relations, relation)
	return e
}

// HasOne defines a has-one relationship
func (e *Eloquent[T]) HasOne(related, foreignKey, localKey string) *Eloquent[T] {
	if foreignKey == "" {
		foreignKey = strlib.ConvertToSnakeCase(e.getModelName()) + "_id"
	}
	if localKey == "" {
		localKey = "_id"
	}
	relation := Relation{
		Name:       related,
		Type:       HasOne,
		Related:    resolveRelatedCollection(related),
		ForeignKey: foreignKey,
		LocalKey:   localKey,
	}
	e.relations = append(e.relations, relation)
	return e
}

// HasMany defines a has-many relationship
func (e *Eloquent[T]) HasMany(related, foreignKey, localKey string) *Eloquent[T] {
	if foreignKey == "" {
		foreignKey = strlib.ConvertToSnakeCase(e.getModelName()) + "_id"
	}
	if localKey == "" {
		localKey = "_id"
	}
	relation := Relation{
		Name:       related,
		Type:       HasMany,
		Related:    resolveRelatedCollection(related),
		ForeignKey: foreignKey,
		LocalKey:   localKey,
	}
	e.relations = append(e.relations, relation)
	return e
}

// BelongsToMany defines a many-to-many relationship
func (e *Eloquent[T]) BelongsToMany(related, pivotTable, foreignKey, relatedKey string) *Eloquent[T] {
	if pivotTable == "" {
		models := []string{e.getModelName(), related}
		if models[0] > models[1] {
			models[0], models[1] = models[1], models[0]
		}
		pivotTable = strlib.ConvertToSnakeCase(models[0]) + "_" + strlib.ConvertToSnakeCase(models[1])
	}
	relation := Relation{
		Name:    related,
		Type:    BelongsToMany,
		Related: resolveRelatedCollection(related),
		Pivot: &PivotConfig{
			Table:      pivotTable,
			ForeignKey: foreignKey,
			RelatedKey: relatedKey,
		},
	}
	e.relations = append(e.relations, relation)
	return e
}

// ==================== Soft Deletes (Laravel Style) ====================

// WithTrashed includes soft-deleted records
func (e *Eloquent[T]) WithTrashed() *Eloquent[T] {
	e.softDeleteMode = softDeleteWith
	return e
}

// OnlyTrashed only returns soft-deleted records
func (e *Eloquent[T]) OnlyTrashed() *Eloquent[T] {
	e.softDeleteMode = softDeleteOnly
	return e
}

// Active scope - only active records
func (e *Eloquent[T]) Active() *Eloquent[T] {
	e.softDeleteMode = softDeleteActive
	return e
}

// WhereMap merges a bson.M directly into the filter (all ANDed)
func (e *Eloquent[T]) WhereMap(data bson.M) *Eloquent[T] {
	for k, v := range data {
		if k != "" { // ignore empty keys
			e.wheres = append(e.wheres, bson.M{k: v})
		}
	}
	return e
}

// ==================== Execution Methods ====================

// buildFilter constructs the final MongoDB filter
func (e *Eloquent[T]) buildFilter() bson.M {
	filter := bson.M{}

	// Add default soft delete behavior unless user explicitly filtered deleted_at
	if !e.hasTrashedHandling() {
		switch e.softDeleteMode {
		case softDeleteActive:
			filter["deleted_at"] = time.Time{}
		case softDeleteOnly:
			filter["deleted_at"] = bson.M{"$ne": time.Time{}}
		case softDeleteWith:
			// no filter
		}
	}

	// Add where conditions
	for _, where := range e.wheres {
		for k, v := range where {
			filter[k] = v
		}
	}

	// Add OR conditions
	if len(e.orWheres) > 0 {
		orConditions := make([]bson.M, 0, len(e.orWheres))
		for _, orWhere := range e.orWheres {
			orConditions = append(orConditions, orWhere)
		}
		if len(orConditions) > 0 {
			if existingOr, exists := filter["$or"]; exists {
				if orSlice, ok := existingOr.([]bson.M); ok {
					filter["$or"] = append(orSlice, orConditions...)
				}
			} else {
				filter["$or"] = orConditions
			}
		}
	}

	// Add whereIn conditions
	for field, values := range e.whereIns {
		filter[field] = bson.M{"$in": values}
	}

	// Add whereNotIn conditions
	for field, values := range e.whereNotIns {
		filter[field] = bson.M{"$nin": values}
	}

	return filter
}

// hasTrashedHandling checks if explicit trashed handling was used
func (e *Eloquent[T]) hasTrashedHandling() bool {
	for _, where := range e.wheres {
		if _, exists := where["deleted_at"]; exists {
			return true
		}
	}
	return false
}

// Cache enables query-level caching for this builder instance
func (e *Eloquent[T]) Cache(ttl time.Duration) *Eloquent[T] {
	e.cacheTTL = &ttl
	return e
}

// cacheTTLOrDefault resolves effective TTL (query-level overrides service default)
func (e *Eloquent[T]) cacheTTLOrDefault() time.Duration {
	if e.cacheTTL != nil && *e.cacheTTL > 0 {
		return *e.cacheTTL
	}
	if e.service != nil && e.service.defaultCacheTTL > 0 {
		return e.service.defaultCacheTTL
	}
	return 0
}

// buildCacheKey constructs a stable cache key for the current query
func (e *Eloquent[T]) buildCacheKey(filter bson.M, isAggregate bool) string {
	keyObj := bson.M{
		"collection": e.service.CollectionName,
		"filter":     filter,
		"orders":     e.orders,
		"select":     e.selectFields,
		"limit":      e.limitNum,
		"skip":       e.skipNum,
		"relations":  e.relations,
		"withCount":  e.withCount,
		"aggregate":  isAggregate,
	}
	bytes, _ := bson.Marshal(keyObj)
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

// Get retrieves all matching records
func (e *Eloquent[T]) Get() ([]T, error) {
	if len(e.relations) > 0 {
		return e.getWithRelations()
	}

	filter := e.buildFilter()
	opts := options.Find()

	if e.limitNum != nil {
		opts.SetLimit(*e.limitNum)
	}
	if e.skipNum != nil {
		opts.SetSkip(*e.skipNum)
	}
	if len(e.orders) > 0 {
		opts.SetSort(e.orders)
	}
	if len(e.selectFields) > 0 {
		projection := bson.M{}
		for _, field := range e.selectFields {
			projection[field] = 1
		}
		opts.SetProjection(projection)
	}

	// Attempt cache
	ttl := e.cacheTTLOrDefault()
	if ttl > 0 && e.service != nil && e.service.cache != nil {
		key := e.buildCacheKey(filter, false)
		if payload, ok := e.service.cache.get(e.service.CollectionName, key); ok {
			var docs []bson.M
			if err := bson.Unmarshal(payload, &docs); err == nil {
				results := make([]T, 0, len(docs))
				for _, doc := range docs {
					obj := e.service.Factory()
					if e.service.BeforeFetch != nil {
						if err := e.service.BeforeFetch(obj); err != nil {
							continue
						}
					}
					if err := e.service.FromBson(&obj, doc); err == nil {
						if e.service.AfterFetch != nil {
							if err := e.service.AfterFetch(obj); err != nil {
								continue
							}
						}
						results = append(results, obj)
					}
				}
				return results, nil
			}
		}
	}

	cursor, err := e.service.Collection.Find(e.service.Ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(e.service.Ctx)

	var results []T
	var docsForCache []bson.M
	if ttl > 0 {
		docsForCache = make([]bson.M, 0)
	}
	for cursor.Next(e.service.Ctx) {
		obj := e.service.Factory()
		if e.service.BeforeFetch != nil {
			if err := e.service.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			if e.service.AfterFetch != nil {
				if err := e.service.AfterFetch(obj); err != nil {
					continue
				}
			}
			results = append(results, obj)
			if ttl > 0 {
				if m, err := e.service.ToBson(obj); err == nil {
					docsForCache = append(docsForCache, m)
				}
			}
		}
	}

	// Store in cache
	if ttl > 0 && e.service != nil && e.service.cache != nil {
		key := e.buildCacheKey(filter, false)
		if bytes, err := bson.Marshal(docsForCache); err == nil {
			e.service.cache.set(e.service.CollectionName, key, bytes, ttl)
		}
	}

	return results, nil
}

// getWithRelations handles eager loading
func (e *Eloquent[T]) getWithRelations() ([]T, error) {
	pipeline := e.buildAggregationPipeline()

	// Attempt cache
	ttl := e.cacheTTLOrDefault()
	if ttl > 0 && e.service != nil && e.service.cache != nil {
		keyObj := bson.M{"collection": e.service.CollectionName, "pipeline": pipeline, "op": "aggregate"}
		bytesKey, _ := bson.Marshal(keyObj)
		sum := sha256.Sum256(bytesKey)
		key := hex.EncodeToString(sum[:])
		if payload, ok := e.service.cache.get(e.service.CollectionName, key); ok {
			var docs []bson.M
			if err := bson.Unmarshal(payload, &docs); err == nil {
				results := make([]T, 0, len(docs))
				for _, doc := range docs {
					obj := e.service.Factory()
					if e.service.BeforeFetch != nil {
						if err := e.service.BeforeFetch(obj); err != nil {
							continue
						}
					}
					if err := e.service.FromBson(&obj, doc); err == nil {
						if e.service.AfterFetch != nil {
							if err := e.service.AfterFetch(obj); err != nil {
								continue
							}
						}
						results = append(results, obj)
					}
				}
				return results, nil
			}
		}
	}

	cursor, err := e.service.Collection.Aggregate(e.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(e.service.Ctx)

	var results []T
	var docsForCache []bson.M
	if ttl > 0 {
		docsForCache = make([]bson.M, 0)
	}
	for cursor.Next(e.service.Ctx) {
		obj := e.service.Factory()
		if e.service.BeforeFetch != nil {
			if err := e.service.BeforeFetch(obj); err != nil {
				continue
			}
		}
		if err := cursor.Decode(obj); err == nil {
			if e.service.AfterFetch != nil {
				if err := e.service.AfterFetch(obj); err != nil {
					continue
				}
			}
			results = append(results, obj)
			if ttl > 0 {
				if m, err := e.service.ToBson(obj); err == nil {
					docsForCache = append(docsForCache, m)
				}
			}
		}
	}

	// Store in cache
	if ttl > 0 && e.service != nil && e.service.cache != nil {
		keyObj := bson.M{"collection": e.service.CollectionName, "pipeline": pipeline, "op": "aggregate"}
		bytesKey, _ := bson.Marshal(keyObj)
		sum := sha256.Sum256(bytesKey)
		key := hex.EncodeToString(sum[:])
		if bytes, err := bson.Marshal(docsForCache); err == nil {
			e.service.cache.set(e.service.CollectionName, key, bytes, ttl)
		}
	}

	return results, nil
}

// buildAggregationPipeline builds MongoDB aggregation pipeline for relationships
func (e *Eloquent[T]) buildAggregationPipeline() []bson.M {
	pipeline := []bson.M{}

	// Add match stage
	filter := e.buildFilter()
	if len(filter) > 0 {
		pipeline = append(pipeline, bson.M{"$match": filter})
	}

	// Add lookup stages for relationships
	for _, rel := range e.relations {
		pipeline = append(pipeline, e.buildLookupStage(rel)...)
	}

	// withCount (only for array relations, e.g., hasMany / belongsToMany)
	if len(e.withCount) > 0 {
		countSet := map[string]struct{}{}
		for _, name := range e.withCount {
			countSet[name] = struct{}{}
		}
		addFields := bson.M{}
		for _, rel := range e.relations {
			if _, ok := countSet[rel.Name]; ok {
				if rel.Type == HasMany || rel.Type == BelongsToMany { // arrays
					addFields[rel.Name+"_count"] = bson.M{"$size": bson.M{"$ifNull": []interface{}{"$" + rel.Name, []interface{}{}}}}
				}
			}
		}
		if len(addFields) > 0 {
			pipeline = append(pipeline, bson.M{"$addFields": addFields})
		}
	}

	// Add sort
	if len(e.orders) > 0 {
		pipeline = append(pipeline, bson.M{"$sort": e.orders})
	}

	// Add skip/limit
	if e.skipNum != nil {
		pipeline = append(pipeline, bson.M{"$skip": *e.skipNum})
	}
	if e.limitNum != nil {
		pipeline = append(pipeline, bson.M{"$limit": *e.limitNum})
	}

	return pipeline
}

// buildLookupStage builds lookup stages for a relationship (with conditions support)
func (e *Eloquent[T]) buildLookupStage(rel Relation) []bson.M {
	stages := []bson.M{}

	switch rel.Type {
	case BelongsTo:
		// parent holds foreignKey referencing related localKey (default _id)
		lookup := bson.M{"$lookup": bson.M{
			"from": rel.Related,
			"let":  bson.M{"fk": "$" + rel.ForeignKey},
			"pipeline": append([]bson.M{bson.M{"$match": bson.M{"$expr": bson.M{
				"$cond": bson.M{
					"if":   bson.M{"$isArray": "$$fk"},
					"then": bson.M{"$in": bson.A{"$" + rel.LocalKey, "$$fk"}},
					"else": bson.M{"$eq": bson.A{"$" + rel.LocalKey, "$$fk"}},
				},
			}}}},
				func() []bson.M {
					if len(rel.Conditions) > 0 {
						return []bson.M{{"$match": rel.Conditions}}
					}
					return nil
				}()...),
			"as": rel.Name,
		}}
		stages = append(stages, lookup)
		stages = append(stages, bson.M{"$unwind": bson.M{"path": "$" + rel.Name, "preserveNullAndEmptyArrays": !rel.Required}})
	case HasOne:
		lookup := bson.M{"$lookup": bson.M{
			"from": rel.Related,
			"let":  bson.M{"local": "$" + rel.LocalKey},
			"pipeline": append([]bson.M{bson.M{"$match": bson.M{"$expr": bson.M{
				"$cond": bson.M{
					"if":   bson.M{"$isArray": "$$local"},
					"then": bson.M{"$in": bson.A{"$" + rel.ForeignKey, "$$local"}},
					"else": bson.M{"$eq": bson.A{"$" + rel.ForeignKey, "$$local"}},
				},
			}}}},
				func() []bson.M {
					if len(rel.Conditions) > 0 {
						return []bson.M{{"$match": rel.Conditions}}
					}
					return nil
				}()...),
			"as": rel.Name,
		}}
		stages = append(stages, lookup)
		stages = append(stages, bson.M{"$unwind": bson.M{"path": "$" + rel.Name, "preserveNullAndEmptyArrays": !rel.Required}})
	case HasMany:
		lookup := bson.M{"$lookup": bson.M{
			"from": rel.Related,
			"let":  bson.M{"local": "$" + rel.LocalKey},
			"pipeline": append([]bson.M{bson.M{"$match": bson.M{"$expr": bson.M{
				"$cond": bson.M{
					"if":   bson.M{"$isArray": "$$local"},
					"then": bson.M{"$in": bson.A{"$" + rel.ForeignKey, "$$local"}},
					"else": bson.M{"$eq": bson.A{"$" + rel.ForeignKey, "$$local"}},
				},
			}}}},
				func() []bson.M {
					if len(rel.Conditions) > 0 {
						return []bson.M{{"$match": rel.Conditions}}
					}
					return nil
				}()...),
			"as": rel.Name,
		}}
		stages = append(stages, lookup)
		if rel.Required {
			stages = append(stages, bson.M{"$match": bson.M{"$expr": bson.M{"$gt": bson.A{bson.M{"$size": "$" + rel.Name}, 0}}}})
		}
	case BelongsToMany:
		if rel.Pivot != nil {
			// pivot lookup
			pivotLookup := bson.M{"$lookup": bson.M{
				"from":         rel.Pivot.Table,
				"localField":   "_id",
				"foreignField": rel.Pivot.ForeignKey,
				"as":           rel.Name + "_pivot",
			}}
			stages = append(stages, pivotLookup)
			// related lookup using pipeline for conditions
			relatedLookup := bson.M{"$lookup": bson.M{
				"from": rel.Related,
				"let":  bson.M{"relIds": "$" + rel.Name + "_pivot." + rel.Pivot.RelatedKey},
				"pipeline": append([]bson.M{bson.M{"$match": bson.M{"$expr": bson.M{"$in": bson.A{"$_id", "$$relIds"}}}}},
					func() []bson.M {
						if len(rel.Conditions) > 0 {
							return []bson.M{{"$match": rel.Conditions}}
						}
						return nil
					}()...),
				"as": rel.Name,
			}}
			stages = append(stages, relatedLookup)
			if rel.Required {
				stages = append(stages, bson.M{"$match": bson.M{"$expr": bson.M{"$gt": bson.A{bson.M{"$size": "$" + rel.Name}, 0}}}})
			}
		}
	}

	return stages
}

// First retrieves the first matching record
func (e *Eloquent[T]) First() (T, error) {
	e.limitNum = &[]int64{1}[0]
	results, err := e.Get()
	if err != nil {
		return e.service.Factory(), err
	}
	if len(results) == 0 {
		return e.service.Factory(), mongo.ErrNoDocuments
	}
	return results[0], nil
}

// FirstOrFail retrieves first record or fails
func (e *Eloquent[T]) FirstOrFail() (T, error) {
	result, err := e.First()
	if err == mongo.ErrNoDocuments {
		return result, errors.New("no records found")
	}
	return result, err
}

// Find retrieves a record by ID
func (e *Eloquent[T]) Find(id interface{}) (T, error) {
	var objID primitive.ObjectID
	var err error

	switch v := id.(type) {
	case string:
		objID, err = primitive.ObjectIDFromHex(v)
		if err != nil {
			return e.service.Factory(), errors.New("invalid ID format")
		}
	case primitive.ObjectID:
		objID = v
	default:
		return e.service.Factory(), errors.New("unsupported ID type")
	}

	return e.Where("_id", objID).First()
}

// FindOrFail finds by ID or fails
func (e *Eloquent[T]) FindOrFail(id interface{}) (T, error) {
	result, err := e.Find(id)
	if err == mongo.ErrNoDocuments {
		return result, fmt.Errorf("no record found with ID: %v", id)
	}
	return result, err
}

// Paginate returns paginated results
func (e *Eloquent[T]) Paginate(page, perPage int) ([]T, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15 // Laravel default
	}

	// Count total using cached Count()
	total, err := e.Clone().Count()
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	skip := (page - 1) * perPage
	e.Skip(skip).Take(perPage)

	results, err := e.Get()
	return results, total, err
}

// Count returns the count of matching records
func (e *Eloquent[T]) Count() (int64, error) {
	filter := e.buildFilter()

	// Cache for count
	ttl := e.cacheTTLOrDefault()
	if ttl > 0 && e.service != nil && e.service.cache != nil {
		keyObj := bson.M{
			"collection": e.service.CollectionName,
			"filter":     filter,
			"op":         "count",
		}
		bytesKey, _ := bson.Marshal(keyObj)
		sum := sha256.Sum256(bytesKey)
		key := hex.EncodeToString(sum[:])
		if payload, ok := e.service.cache.get(e.service.CollectionName, key); ok {
			var wrap struct {
				Count int64 `bson:"count"`
			}
			if err := bson.Unmarshal(payload, &wrap); err == nil {
				return wrap.Count, nil
			}
		}
	}

	count, err := e.service.Collection.CountDocuments(e.service.Ctx, filter)
	if err != nil {
		return 0, err
	}
	if ttl > 0 && e.service != nil && e.service.cache != nil {
		wrap := bson.M{"count": count}
		if bytes, err := bson.Marshal(wrap); err == nil {
			e.service.cache.set(e.service.CollectionName, keyForCount(e.service.CollectionName, filter), bytes, ttl)
		}
	}
	return count, nil
}

// keyForCount builds a stable key for count queries (separate from Get keys)
func keyForCount(coll string, filter bson.M) string {
	keyObj := bson.M{"collection": coll, "filter": filter, "op": "count"}
	bytesKey, _ := bson.Marshal(keyObj)
	sum := sha256.Sum256(bytesKey)
	return hex.EncodeToString(sum[:])
}

// Exists checks if any records exist
func (e *Eloquent[T]) Exists() (bool, error) {
	count, err := e.Count()
	return count > 0, err
}

// ==================== Updates and Deletes ====================
// Update updates matching records
func (e *Eloquent[T]) Update(updates bson.M) (int64, error) {
	filter := e.buildFilter()
	updates["updated_at"] = time.Now()

	// Fetch matching docs for hooks
	var preModels []T
	if e.service.BeforeSave != nil || e.service.AfterUpdate != nil {
		cur, err := e.service.Collection.Find(e.service.Ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
		if err != nil {
			return 0, err
		}
		for cur.Next(e.service.Ctx) {
			m := e.service.Factory()
			if err := cur.Decode(m); err == nil {
				preModels = append(preModels, m)
			}
		}
		_ = cur.Close(e.service.Ctx)
	}

	// BeforeSave hooks
	if e.service.BeforeSave != nil {
		for _, m := range preModels {
			if err := e.service.BeforeSave(m); err != nil {
				// skip on error for that model (continue others)
				continue
			}
		}
	}

	result, err := e.service.Collection.UpdateMany(e.service.Ctx, filter, bson.M{"$set": updates})
	if err != nil {
		return 0, err
	}

	// Invalidate cache for this collection on write
	if e.service != nil && e.service.cache != nil {
		e.service.cache.invalidateCollection(e.service.CollectionName)
	}

	// AfterUpdate hooks
	if e.service.AfterUpdate != nil && len(preModels) > 0 {
		// refetch updated docs to pass full state
		ids := make([]primitive.ObjectID, 0, len(preModels))
		for _, m := range preModels {
			ids = append(ids, m.GetID())
		}
		cur, err2 := e.service.Collection.Find(e.service.Ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err2 == nil {
			for cur.Next(e.service.Ctx) {
				m := e.service.Factory()
				if errD := cur.Decode(m); errD == nil {
					_ = e.service.AfterUpdate(m)
				}
			}
			_ = cur.Close(e.service.Ctx)
		}
	}
	return result.ModifiedCount, nil
}

// Delete soft deletes matching records (BeforeDelete/AfterDelete hooks)
func (e *Eloquent[T]) Delete() (int64, error) {
	filter := e.buildFilter()
	var ids []primitive.ObjectID
	if e.service.BeforeDelete != nil || e.service.AfterDelete != nil {
		cur, err := e.service.Collection.Find(e.service.Ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
		if err != nil {
			return 0, err
		}
		for cur.Next(e.service.Ctx) {
			m := struct {
				ID primitive.ObjectID `bson:"_id"`
			}{}
			if err := cur.Decode(&m); err == nil {
				ids = append(ids, m.ID)
			}
		}
		_ = cur.Close(e.service.Ctx)
		if e.service.BeforeDelete != nil {
			for _, id := range ids {
				_ = e.service.BeforeDelete(id.Hex())
			}
		}
	}
	res, err := e.service.Collection.UpdateMany(e.service.Ctx, filter, bson.M{"$set": bson.M{"deleted_at": time.Now()}})
	if err != nil {
		return 0, err
	}
	if e.service.AfterDelete != nil {
		for _, id := range ids {
			_ = e.service.AfterDelete(id.Hex())
		}
	}
	// Invalidate cache after delete
	if e.service != nil && e.service.cache != nil {
		e.service.cache.invalidateCollection(e.service.CollectionName)
	}
	return res.ModifiedCount, nil
}

// ForceDelete permanently deletes matching records (hooks)
func (e *Eloquent[T]) ForceDelete() (int64, error) {
	filter := e.buildFilter()
	var ids []primitive.ObjectID
	if e.service.BeforeDelete != nil || e.service.AfterDelete != nil {
		cur, err := e.service.Collection.Find(e.service.Ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
		if err != nil {
			return 0, err
		}
		for cur.Next(e.service.Ctx) {
			m := struct {
				ID primitive.ObjectID `bson:"_id"`
			}{}
			if err := cur.Decode(&m); err == nil {
				ids = append(ids, m.ID)
			}
		}
	}
	result, err := e.service.Collection.DeleteMany(e.service.Ctx, filter)
	if err != nil {
		return 0, err
	}
	if e.service.AfterDelete != nil {
		for _, id := range ids {
			_ = e.service.AfterDelete(id.Hex())
		}
	}
	// Invalidate cache after force delete
	if e.service != nil && e.service.cache != nil {
		e.service.cache.invalidateCollection(e.service.CollectionName)
	}
	return result.DeletedCount, nil
}

// Restore restores soft deleted records (BeforeSave/AfterUpdate as a logical update)
func (e *Eloquent[T]) Restore() (int64, error) {
	filter := e.buildFilter()
	// ensure we target deleted docs
	filter["deleted_at"] = bson.M{"$ne": time.Time{}}
	return e.Update(bson.M{"deleted_at": time.Time{}})
}

// ==================== EloquentService Updates ====================

// Query returns an Eloquent query builder
func (s *EloquentService[T]) Query() *Eloquent[T] {
	return &Eloquent[T]{
		service:     s,
		wheres:      []bson.M{},
		orWheres:    []bson.M{},
		whereIns:    make(map[string][]interface{}),
		whereNotIns: make(map[string][]interface{}),
		relations:   []Relation{},
		withCount:   []string{},
		scopes:      []func(*Eloquent[T]) *Eloquent[T]{},
	}
}

// All returns all records (with soft delete filtering)
func (s *EloquentService[T]) All() *Eloquent[T] {
	return s.Query()
}

// Create creates a new record
func (s *EloquentService[T]) Create(data bson.M) (T, error) {
	model := s.Factory()

	// Set timestamps
	now := time.Now()
	data["created_at"] = now
	data["updated_at"] = now
	data["deleted_at"] = time.Time{}

	// Convert bson.M to model
	if err := s.FromBson(&model, data); err != nil {
		return model, err
	}

	// Set ID if not present
	if model.GetID().IsZero() {
		model.SetID(primitive.NewObjectID())
	}

	// Call hooks
	if s.BeforeSave != nil {
		if err := s.BeforeSave(model); err != nil {
			return model, err
		}
	}

	// Insert
	_, err := s.Collection.InsertOne(s.Ctx, model)
	if err != nil {
		return model, err
	}

	// Invalidate cache for this collection on write
	if s.cache != nil {
		s.cache.invalidateCollection(s.CollectionName)
	}

	// After create hook
	if s.AfterCreate != nil {
		_ = s.AfterCreate(model)
	}

	return model, nil
}

// FirstOrCreate finds first record or creates if not found
func (s *EloquentService[T]) FirstOrCreate(conditions bson.M, model T) (T, error) {
	defaults, err := s.ToBson(model)
	if err != nil {
		return model, err
	}
	qb := s.Query().WhereMap(conditions)
	result, err := qb.First()
	if err == mongo.ErrNoDocuments {
		createData := make(bson.M)
		for k, v := range conditions {
			createData[k] = v
		}
		for k, v := range defaults {
			createData[k] = v
		}
		return s.Create(createData)
	}
	return result, err
}

// UpdateOrCreate updates existing record or creates new one
func (s *EloquentService[T]) UpdateOrCreate(conditions bson.M, model T) (T, error) {
	updates, err := s.ToBson(model)
	if err != nil {
		return model, err
	}
	qb := s.Query().WhereMap(conditions)
	result, err := qb.First()
	if err == mongo.ErrNoDocuments {
		createData := make(bson.M)
		for k, v := range conditions {
			createData[k] = v
		}
		for k, v := range updates {
			createData[k] = v
		}
		return s.Create(createData)
	} else if err != nil {
		return result, err
	}
	_, err = s.Query().Where("_id", result.GetID()).Update(updates)
	if err != nil {
		return result, err
	}
	return s.Find(result.GetID().Hex())
}

// CreateOrUpdate saves the model if it has no ID, otherwise updates the existing document.
func (s *EloquentService[T]) CreateOrUpdate(model T) (T, error) {
	// If no ID present, perform Save
	if model.GetID().IsZero() {
		return s.Save(model)
	}

	id := model.GetID().Hex()

	// Convert model to bson and remove _id to avoid immutable field update
	doc, err := s.ToBson(model)
	if err != nil {
		return model, err
	}
	delete(doc, "_id")

	// Perform update
	err = s.Update(id, doc)
	if err != nil {
		return model, err
	}

	// Return the fresh model
	return s.Find(id)
}

// ==================== Enhanced EloquentService Interface ====================

type EloquentInterface[T BaseModels.MongoModel] interface {
	// Laravel-style query methods
	All() *Eloquent[T]
	Find(id interface{}) (T, error)
	FindOrFail(id interface{}) (T, error)
	Create(data bson.M) (T, error)
	FirstOrCreate(conditions, defaults bson.M) (T, error)
	UpdateOrCreate(conditions, updates bson.M) (T, error)

	// Query builder
	Query() *Eloquent[T]
	Where(field string, operator interface{}, value ...interface{}) *Eloquent[T]
	WhereIn(field string, values interface{}) *Eloquent[T]
	WhereNotIn(field string, values interface{}) *Eloquent[T]
	OrderBy(field string, direction ...string) *Eloquent[T]
	Latest(field ...string) *Eloquent[T]
	Oldest(field ...string) *Eloquent[T]

	// Relationships
	With(relations ...string) *Eloquent[T]
	WithCount(relations ...string) *Eloquent[T]

	// Soft deletes
	WithTrashed() *Eloquent[T]
	OnlyTrashed() *Eloquent[T]

	// Pagination
	Paginate(page, perPage int) ([]T, int64, error)

	// Aggregation
	Count() (int64, error)
	Exists() (bool, error)
}

// ==================== Model Relationships (Method Definitions) ====================

// ModelRelationships interface for models to define relationships
type ModelRelationships[T BaseModels.MongoModel] interface {
	// Relationship definitions
	BelongsTo(service *EloquentService[T], related, foreignKey, localKey string) *Eloquent[T]
	HasOne(service *EloquentService[T], related, foreignKey, localKey string) *Eloquent[T]
	HasMany(service *EloquentService[T], related, foreignKey, localKey string) *Eloquent[T]
	BelongsToMany(service *EloquentService[T], related, pivot, foreignKey, relatedKey string) *Eloquent[T]
}

// ==================== Collection/Table Methods ====================

// Table/Collection method for dynamic collection names
func (e *Eloquent[T]) Table(tableName string) *Eloquent[T] {
	// Create new service with different collection
	newService := *e.service
	newService.Collection = e.service.DB.Collection(tableName)
	newService.CollectionName = tableName
	e.service = &newService
	return e
}

// ==================== Raw Queries ====================

// Raw executes raw MongoDB queries
func (e *Eloquent[T]) Raw(pipeline []bson.M) ([]bson.M, error) {
	cursor, err := e.service.Collection.Aggregate(e.service.Ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(e.service.Ctx)

	var results []bson.M
	for cursor.Next(e.service.Ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err == nil {
			results = append(results, doc)
		}
	}
	return results, nil
}

// ==================== Advanced Query Methods ====================

// WhereHas filters based on relationship existence
func (e *Eloquent[T]) WhereHas(relation string, callback func(*Eloquent[T]) *Eloquent[T]) *Eloquent[T] {
	// This would build a complex aggregation pipeline
	// For now, simplified implementation
	if callback != nil {
		subQuery := e.service.Query()
		subQuery = callback(subQuery)
		// Apply subquery logic here
	}
	return e
}

// WhereDoesntHave filters based on relationship non-existence
func (e *Eloquent[T]) WhereDoesntHave(relation string, callback func(*Eloquent[T]) *Eloquent[T]) *Eloquent[T] {
	// Similar to WhereHas but inverted
	return e
}

// WithAggregate adds aggregation functions like withCount, withSum, etc.
func (e *Eloquent[T]) WithAggregate(relation, function, column string) *Eloquent[T] {
	// Implementation for withCount, withSum, withAvg, etc.
	return e
}

// ==================== Chunking and Batching ====================

// Chunk processes records in chunks
func (e *Eloquent[T]) Chunk(size int, callback func([]T) error) error {
	if size <= 0 {
		return errors.New("chunk size must be greater than 0")
	}

	page := 1
	for {
		results, total, err := e.Paginate(page, size)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			break
		}

		if err := callback(results); err != nil {
			return err
		}

		if int64(page*size) >= total {
			break
		}
		page++
	}

	return nil
}

// ChunkById processes records in chunks by ID for better memory usage
func (e *Eloquent[T]) ChunkById(size int, callback func([]T) error) error {
	var lastId primitive.ObjectID

	for {
		query := e.service.Query().OrderBy("_id").Take(size)
		if !lastId.IsZero() {
			query = query.Where("_id", ">", lastId)
		}

		results, err := query.Get()
		if err != nil {
			return err
		}

		if len(results) == 0 {
			break
		}

		if err := callback(results); err != nil {
			return err
		}

		// Update lastId for next iteration
		lastId = results[len(results)-1].GetID()

		if len(results) < size {
			break
		}
	}

	return nil
}

// ==================== Utility Methods ====================

// ToSql returns the equivalent MongoDB query (for debugging)
func (e *Eloquent[T]) ToSql() (bson.M, error) {
	return e.buildFilter(), nil
}

// Explain returns query execution plan
func (e *Eloquent[T]) Explain() (bson.M, error) {
	filter := e.buildFilter()
	opts := options.Find()

	// MongoDB explain functionality
	cursor, err := e.service.Collection.Find(e.service.Ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(e.service.Ctx)

	// Return explain plan (simplified)
	return bson.M{
		"filter":     filter,
		"collection": e.service.CollectionName,
	}, nil
}

// Clone creates a copy of the query builder
func (e *Eloquent[T]) Clone() *Eloquent[T] {
	clone := &Eloquent[T]{
		service:      e.service,
		wheres:       make([]bson.M, len(e.wheres)),
		orWheres:     make([]bson.M, len(e.orWheres)),
		whereIns:     make(map[string][]interface{}),
		whereNotIns:  make(map[string][]interface{}),
		orders:       make([]bson.E, len(e.orders)),
		selectFields: make([]string, len(e.selectFields)),
		relations:    make([]Relation, len(e.relations)),
		withCount:    make([]string, len(e.withCount)),
		scopes:       make([]func(*Eloquent[T]) *Eloquent[T], len(e.scopes)),
	}

	copy(clone.wheres, e.wheres)
	copy(clone.orWheres, e.orWheres)
	copy(clone.orders, e.orders)
	copy(clone.selectFields, e.selectFields)
	copy(clone.relations, e.relations)
	copy(clone.withCount, e.withCount)
	copy(clone.scopes, e.scopes)

	for k, v := range e.whereIns {
		clone.whereIns[k] = make([]interface{}, len(v))
		copy(clone.whereIns[k], v)
	}

	for k, v := range e.whereNotIns {
		clone.whereNotIns[k] = make([]interface{}, len(v))
		copy(clone.whereNotIns[k], v)
	}

	if e.limitNum != nil {
		limit := *e.limitNum
		clone.limitNum = &limit
	}

	if e.skipNum != nil {
		skip := *e.skipNum
		clone.skipNum = &skip
	}

	return clone
}

// ==================== Transaction Support ====================

// WithTransaction executes queries within a transaction
func (s *EloquentService[T]) WithTransaction(fn func(*mongo.SessionContext) error) error {
	session, err := s.DB.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(s.Ctx)

	return mongo.WithSession(s.Ctx, session, func(sc mongo.SessionContext) error {
		if err := session.StartTransaction(); err != nil {
			return err
		}

		if err := fn(&sc); err != nil {
			session.AbortTransaction(sc)
			return err
		}

		return session.CommitTransaction(sc)
	})
}

// ==================== Usage Examples in Comments ====================

/*
Usage Examples:

// WhereIn with ObjectIDs
ids := []primitive.ObjectID{id1, id2}
users, _ := userService.ORM().Query().WhereIn("_id", ids).Get()

// HasMany where parent local key is an array (e.g., product.valuer_ids -> users._id)
products, _ := productService.ORM().Query().HasMany("users", "_id", "valuer_ids").Get()

*/

// ========================
// EloquentService
// ========================

type EloquentService[T BaseModels.MongoModel] struct {
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

	// Caching
	cache           *serviceCache
	defaultCacheTTL time.Duration
}

// EloquentServiceInterface defines the main service operations for a model T.
type EloquentServiceInterface[T BaseModels.MongoModel] interface {
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
	Query() *Eloquent[T]

	// Additional helpers
	// UpdateOrCreate by conditions (matches implemented method signature)
	UpdateOrCreate(conditions bson.M, model T) (T, error)
	// CreateOrUpdate accepts a model instance and will save or update depending on ID
	CreateOrUpdate(model T) (T, error)
	// FirstOrCreate using conditions and a model instance (matches implemented method signature)
	FirstOrCreate(conditions bson.M, model T) (T, error)
	Reload(model T) (T, error)

	// Internal / helpers exposed for interface use
	NewInstance() EloquentService[T]
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

	// Cache configuration
	SetDefaultCacheTTL(ttl time.Duration)
	ClearCache()
}

// NewEloquentService returns a new EloquentService instance.
//
// The provided context will be used for all operations.
// The provided mongo database will be used for all operations.
// The provided factory function will be used to create new instances of the model.
//
// The collection name will be determined by calling the `GetCollectionName` method
// on the model returned by the factory function.
func NewEloquentService[T BaseModels.MongoModel](ctx context.Context, db *mongo.Database, factory func() T) *EloquentService[T] {
	model := factory()

	// Define a local function to get the collection name with custom logic
	getCollectionName := func(model T) string {
		if model.GetCollectionName() != "" {
			return model.GetCollectionName()
		}
		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return strlib.Pluralize(strlib.ConvertToSnakeCase(t.Name()))
	}

	eloquentService := &EloquentService[T]{
		Ctx:        ctx,
		DB:         db,
		Collection: db.Collection(getCollectionName(model)),
		Factory:    factory,
		cache:      &serviceCache{},
	}

	// Define a local function to get the collection name with custom logic
	eloquentService.CollectionName = getCollectionName(model)

	_, _ = eloquentService.Collection.UpdateMany(ctx, bson.M{
		"$or": []bson.M{
			{"deleted_at": bson.M{"$exists": false}},
			{"deleted_at": nil},
		},
	}, bson.M{
		"$set": bson.M{"deleted_at": time.Time{}},
	})

	return eloquentService
}

// Find retrieves a document by its ID.
//
// It converts the provided string ID to a primitive.ObjectID and searches for a document
// with the matching "_id" and a "deleted_at" field set to the zero time value, indicating
// that the document is active.
//
// Returns the document if found, or an error if the ID is invalid or the document is not found.
func (s *EloquentService[T]) Find(id string) (T, error) {
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
func (s *EloquentService[T]) FindOrFail(id string) (T, error) {
	result, err := s.Find(id)
	if err != nil {
		return result, errors.New("record with id " + id + " not found")
	}
	return result, nil
}

// Save creates a new record with BeforeSave and AfterCreate hooks
func (s *EloquentService[T]) Save(model T) (T, error) {
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
	// Invalidate cache for this collection on write
	if s.cache != nil {
		s.cache.invalidateCollection(s.CollectionName)
	}
	if s.AfterCreate != nil {
		_ = s.AfterCreate(model)
	}
	return model, nil
}

// Update by ID with AfterUpdate hook
func (s *EloquentService[T]) Update(id string, updates bson.M) error {
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
	if err == nil {
		// Invalidate cache on write
		if s.cache != nil {
			s.cache.invalidateCollection(s.CollectionName)
		}
		if s.AfterUpdate != nil {
			model, findErr := s.Find(id)
			if findErr == nil {
				_ = s.AfterUpdate(model)
			}
		}
	}
	return err
}

// Delete (soft delete) with lifecycle hooks
func (s *EloquentService[T]) Delete(id string) error {
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
	if err == nil {
		if s.cache != nil {
			s.cache.invalidateCollection(s.CollectionName)
		}
		if s.AfterDelete != nil {
			_ = s.AfterDelete(id)
		}
	}
	return err
}

// Restore a soft deleted document
func (s *EloquentService[T]) Restore(id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.Collection.UpdateOne(s.Ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"deleted_at": time.Time{}}})
	if err == nil {
		if s.cache != nil {
			s.cache.invalidateCollection(s.CollectionName)
		}
	}
	return err
}

// ForceDelete hard deletes a document by ID
func (s *EloquentService[T]) ForceDelete(id string) error {
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
	if err == nil {
		if s.cache != nil {
			s.cache.invalidateCollection(s.CollectionName)
		}
		if s.AfterDelete != nil {
			_ = s.AfterDelete(id)
		}
	}
	return err
}

// Reload reloads model from DB
func (s *EloquentService[T]) Reload(model T) (T, error) {
	return s.Find(model.GetID().Hex())
}

// findOne helper
func (s *EloquentService[T]) findOne(filter bson.M) (T, error) {
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
func (s *EloquentService[T]) findWithFilter(filter bson.M) ([]T, error) {
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
func (s *EloquentService[T]) paginate(filter bson.M, page, limit int) ([]T, int64, error) {
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
func (s *EloquentService[T]) NewInstance() EloquentService[T] {
	return *s
}

// GetCollection returns the collection
func (s *EloquentService[T]) GetCollection() *mongo.Collection {
	return s.Collection
}

// GetModel returns the model factory
func (s *EloquentService[T]) GetModel() T {
	return s.Factory()
}

// NewModel returns a new instance of the model
func (s *EloquentService[T]) NewModel() T {
	return s.Factory()
}

// GetFactory returns the model factory
func (s *EloquentService[T]) GetFactory() func() T {
	return s.Factory
}

func (s *EloquentService[T]) GetCtx() context.Context {
	return s.Ctx
}

func (s *EloquentService[T]) GetCollectionName() string {
	return s.CollectionName
}

func (s *EloquentService[T]) GetRoutePrefix() string {
	t := reflect.TypeOf(s.GetFactory()())
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	hyphenated := strlib.Hyphenate(t.Name())
	return strlib.Pluralize(strings.ToLower(hyphenated))
}

// SetDefaultCacheTTL configures a default TTL for read-query caching on this service
func (s *EloquentService[T]) SetDefaultCacheTTL(ttl time.Duration) {
	s.defaultCacheTTL = ttl
}

// ClearCache clears all cached queries for this service's collection
func (s *EloquentService[T]) ClearCache() {
	if s.cache != nil {
		s.cache.invalidateCollection(s.CollectionName)
	}
}

// ToBson converts a model instance to bson.M by marshalling/unmarshalling inside the service.
func (s *EloquentService[T]) ToBson(model T) (bson.M, error) {
	data, err := bson.Marshal(model)
	if err != nil {
		return nil, err
	}
	var doc bson.M
	err = bson.Unmarshal(data, &doc)
	return doc, err
}

// ToJson converts a model instance to map[string]interface{} by marshalling/unmarshalling inside the service.
func (s *EloquentService[T]) ToJson(model T) (map[string]interface{}, error) {
	bytes, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(bytes, &result)
	return result, err
}

// FromBson converts a bson.M document to the model instance
func (s *EloquentService[T]) FromBson(model *T, data bson.M) error {
	bytes, err := bson.Marshal(data)
	if err != nil {
		return err
	}
	return bson.Unmarshal(bytes, model)
}

// FromJson converts a JSON-like map to the model instance
// FromJson converts a JSON-like map to the model instance (must be pointer)
func (s *EloquentService[T]) FromJson(model *T, data map[string]interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, model)
}
