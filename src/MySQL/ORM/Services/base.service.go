package MySQLServices

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	GlobalModels "github.com/venomous-maker/go-eloquent/src/Global/Models"
	StringLibs "github.com/venomous-maker/go-eloquent/src/Libs/Strings"
	"reflect"
	"strings"
	"time"
)

// ========================
// BaseEloquentService (Laravel-inspired)
// ========================
type BaseEloquentService[T GlobalModels.ORMModel] struct {
	Ctx       context.Context
	DB        *sql.DB
	Factory   func() T
	TableName string

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
	GlobalScopes map[string]func(*MySqlQueryBuilder[T]) *MySqlQueryBuilder[T]
}

func NewEloquentService[T GlobalModels.ORMModel](ctx context.Context, db *sql.DB, factory func() T) *BaseEloquentService[T] {
	model := factory()
	tableName := model.GetTable()
	if tableName == "" {
		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		tableName = StringLibs.Pluralize(StringLibs.ConvertToSnakeCase(t.Name()))
	}

	return &BaseEloquentService[T]{
		Ctx:          ctx,
		DB:           db,
		Factory:      factory,
		TableName:    tableName,
		GlobalScopes: make(map[string]func(*MySqlQueryBuilder[T]) *MySqlQueryBuilder[T]),
	}
}

// ========================
// Laravel-Style Query Builder
// ========================
type MySqlQueryBuilder[T GlobalModels.ORMModel] struct {
	service     *BaseEloquentService[T]
	wheres      []WhereClause
	orders      []OrderClause
	limit       int
	offset      int
	columns     []string
	joins       []JoinClause
	groups      []string
	havings     []HavingClause
	bindings    map[string][]interface{}
	scopes      []string
	withTrashed bool
	onlyTrashed bool
	forceDelete bool
}

type WhereClause struct {
	Type     string // basic, in, between, null, exists, raw
	Column   string
	Operator string
	Value    interface{}
	Boolean  string // and, or
}

type OrderClause struct {
	Column    string
	Direction string
}

type JoinClause struct {
	Type     string // inner, left, right, cross
	Table    string
	First    string
	Operator string
	Second   string
}

type HavingClause struct {
	Column   string
	Operator string
	Value    interface{}
	Boolean  string
}

// ========================
// Laravel-Style Query Builder Methods
// ========================
func (s *BaseEloquentService[T]) NewQuery() *MySqlQueryBuilder[T] {
	qb := &MySqlQueryBuilder[T]{
		service:  s,
		wheres:   []WhereClause{},
		orders:   []OrderClause{},
		joins:    []JoinClause{},
		groups:   []string{},
		havings:  []HavingClause{},
		limit:    -1,
		columns:  []string{"*"},
		bindings: make(map[string][]interface{}),
		scopes:   []string{},
	}

	// Apply global scopes
	for name, scope := range s.GlobalScopes {
		qb.scopes = append(qb.scopes, name)
		qb = scope(qb)
	}

	// Apply soft delete scope by default
	model := s.Factory()
	if model.IsSoftDeletes() {
		qb = qb.whereNull(model.GetDeletedAtColumn())
	}

	return qb
}

// Laravel Query() method
func (s *BaseEloquentService[T]) Query() *MySqlQueryBuilder[T] {
	return s.NewQuery()
}

// ========================
// Where Clauses (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) Where(column string, args ...interface{}) *MySqlQueryBuilder[T] {
	var operator string = "="
	var value interface{}

	switch len(args) {
	case 1:
		value = args[0]
	case 2:
		operator = args[0].(string)
		value = args[1]
	}

	qb.wheres = append(qb.wheres, WhereClause{
		Type:     "basic",
		Column:   column,
		Operator: operator,
		Value:    value,
		Boolean:  "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) OrWhere(column string, args ...interface{}) *MySqlQueryBuilder[T] {
	var operator string = "="
	var value interface{}

	switch len(args) {
	case 1:
		value = args[0]
	case 2:
		operator = args[0].(string)
		value = args[1]
	}

	qb.wheres = append(qb.wheres, WhereClause{
		Type:     "basic",
		Column:   column,
		Operator: operator,
		Value:    value,
		Boolean:  "or",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereIn(column string, values []interface{}) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:    "in",
		Column:  column,
		Value:   values,
		Boolean: "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereNotIn(column string, values []interface{}) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:     "in",
		Column:   column,
		Operator: "not",
		Value:    values,
		Boolean:  "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereBetween(column string, values [2]interface{}) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:    "between",
		Column:  column,
		Value:   values,
		Boolean: "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereNotBetween(column string, values [2]interface{}) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:     "between",
		Column:   column,
		Operator: "not",
		Value:    values,
		Boolean:  "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) whereNull(column string) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:    "null",
		Column:  column,
		Boolean: "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereNull(column string) *MySqlQueryBuilder[T] {
	return qb.whereNull(column)
}

func (qb *MySqlQueryBuilder[T]) WhereNotNull(column string) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:     "null",
		Column:   column,
		Operator: "not",
		Boolean:  "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereDate(column string, operator string, value interface{}) *MySqlQueryBuilder[T] {
	return qb.Where(fmt.Sprintf("DATE(%s)", column), operator, value)
}

func (qb *MySqlQueryBuilder[T]) WhereTime(column string, operator string, value interface{}) *MySqlQueryBuilder[T] {
	return qb.Where(fmt.Sprintf("TIME(%s)", column), operator, value)
}

func (qb *MySqlQueryBuilder[T]) WhereYear(column string, operator string, value interface{}) *MySqlQueryBuilder[T] {
	return qb.Where(fmt.Sprintf("YEAR(%s)", column), operator, value)
}

func (qb *MySqlQueryBuilder[T]) WhereMonth(column string, operator string, value interface{}) *MySqlQueryBuilder[T] {
	return qb.Where(fmt.Sprintf("MONTH(%s)", column), operator, value)
}

func (qb *MySqlQueryBuilder[T]) WhereDay(column string, operator string, value interface{}) *MySqlQueryBuilder[T] {
	return qb.Where(fmt.Sprintf("DAY(%s)", column), operator, value)
}

func (qb *MySqlQueryBuilder[T]) WhereColumn(first string, operator string, second string) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:     "column",
		Column:   first,
		Operator: operator,
		Value:    second,
		Boolean:  "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) WhereRaw(sql string, bindings ...interface{}) *MySqlQueryBuilder[T] {
	qb.wheres = append(qb.wheres, WhereClause{
		Type:    "raw",
		Column:  sql,
		Value:   bindings,
		Boolean: "and",
	})
	return qb
}

// ========================
// Ordering (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) OrderBy(column string, direction ...string) *MySqlQueryBuilder[T] {
	dir := "asc"
	if len(direction) > 0 {
		dir = strings.ToLower(direction[0])
	}
	qb.orders = append(qb.orders, OrderClause{
		Column:    column,
		Direction: dir,
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) OrderByDesc(column string) *MySqlQueryBuilder[T] {
	return qb.OrderBy(column, "desc")
}

func (qb *MySqlQueryBuilder[T]) Latest(column ...string) *MySqlQueryBuilder[T] {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return qb.OrderByDesc(col)
}

func (qb *MySqlQueryBuilder[T]) Oldest(column ...string) *MySqlQueryBuilder[T] {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return qb.OrderBy(col)
}

func (qb *MySqlQueryBuilder[T]) InRandomOrder() *MySqlQueryBuilder[T] {
	return qb.OrderBy("RAND()")
}

// ========================
// Joins (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) Join(table, first, operator, second string) *MySqlQueryBuilder[T] {
	qb.joins = append(qb.joins, JoinClause{
		Type:     "inner",
		Table:    table,
		First:    first,
		Operator: operator,
		Second:   second,
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) LeftJoin(table, first, operator, second string) *MySqlQueryBuilder[T] {
	qb.joins = append(qb.joins, JoinClause{
		Type:     "left",
		Table:    table,
		First:    first,
		Operator: operator,
		Second:   second,
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) RightJoin(table, first, operator, second string) *MySqlQueryBuilder[T] {
	qb.joins = append(qb.joins, JoinClause{
		Type:     "right",
		Table:    table,
		First:    first,
		Operator: operator,
		Second:   second,
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) CrossJoin(table string) *MySqlQueryBuilder[T] {
	qb.joins = append(qb.joins, JoinClause{
		Type:  "cross",
		Table: table,
	})
	return qb
}

// ========================
// Grouping and Having (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) GroupBy(columns ...string) *MySqlQueryBuilder[T] {
	qb.groups = append(qb.groups, columns...)
	return qb
}

func (qb *MySqlQueryBuilder[T]) Having(column, operator string, value interface{}) *MySqlQueryBuilder[T] {
	qb.havings = append(qb.havings, HavingClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Boolean:  "and",
	})
	return qb
}

func (qb *MySqlQueryBuilder[T]) OrHaving(column, operator string, value interface{}) *MySqlQueryBuilder[T] {
	qb.havings = append(qb.havings, HavingClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Boolean:  "or",
	})
	return qb
}

// ========================
// Limiting and Offsetting
// ========================
func (qb *MySqlQueryBuilder[T]) Take(count int) *MySqlQueryBuilder[T] {
	qb.limit = count
	return qb
}

func (qb *MySqlQueryBuilder[T]) Limit(count int) *MySqlQueryBuilder[T] {
	return qb.Take(count)
}

func (qb *MySqlQueryBuilder[T]) Skip(count int) *MySqlQueryBuilder[T] {
	qb.offset = count
	return qb
}

func (qb *MySqlQueryBuilder[T]) Offset(count int) *MySqlQueryBuilder[T] {
	return qb.Skip(count)
}

// ========================
// Selecting Columns
// ========================
func (qb *MySqlQueryBuilder[T]) Select(columns ...string) *MySqlQueryBuilder[T] {
	if len(columns) > 0 {
		qb.columns = columns
	}
	return qb
}

func (qb *MySqlQueryBuilder[T]) SelectRaw(expression string, bindings ...interface{}) *MySqlQueryBuilder[T] {
	qb.columns = append(qb.columns, expression)
	if len(bindings) > 0 {
		qb.bindings["select"] = append(qb.bindings["select"], bindings...)
	}
	return qb
}

func (qb *MySqlQueryBuilder[T]) AddSelect(columns ...string) *MySqlQueryBuilder[T] {
	if qb.columns[0] == "*" {
		qb.columns = columns
	} else {
		qb.columns = append(qb.columns, columns...)
	}
	return qb
}

func (qb *MySqlQueryBuilder[T]) Distinct() *MySqlQueryBuilder[T] {
	// Prepend DISTINCT to first column
	if len(qb.columns) > 0 && qb.columns[0] != "DISTINCT *" {
		if qb.columns[0] == "*" {
			qb.columns[0] = "DISTINCT *"
		} else {
			qb.columns[0] = "DISTINCT " + qb.columns[0]
		}
	}
	return qb
}

// ========================
// Soft Deletes (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) WithTrashed() *MySqlQueryBuilder[T] {
	qb.withTrashed = true
	qb.onlyTrashed = false
	// Remove soft delete scope
	var newWheres []WhereClause
	model := qb.service.Factory()
	deletedCol := model.GetDeletedAtColumn()

	for _, where := range qb.wheres {
		if !(where.Type == "null" && where.Column == deletedCol && where.Operator == "") {
			newWheres = append(newWheres, where)
		}
	}
	qb.wheres = newWheres
	return qb
}

func (qb *MySqlQueryBuilder[T]) WithoutTrashed() *MySqlQueryBuilder[T] {
	qb.withTrashed = false
	qb.onlyTrashed = false
	model := qb.service.Factory()
	if model.IsSoftDeletes() {
		return qb.WhereNull(model.GetDeletedAtColumn())
	}
	return qb
}

func (qb *MySqlQueryBuilder[T]) OnlyTrashed() *MySqlQueryBuilder[T] {
	qb.onlyTrashed = true
	qb.withTrashed = false
	model := qb.service.Factory()
	if model.IsSoftDeletes() {
		return qb.WhereNotNull(model.GetDeletedAtColumn())
	}
	return qb
}

// ========================
// Scopes (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) Scope(name string, scope func(*MySqlQueryBuilder[T]) *MySqlQueryBuilder[T]) *MySqlQueryBuilder[T] {
	return scope(qb)
}

func (qb *MySqlQueryBuilder[T]) When(condition bool, callback func(*MySqlQueryBuilder[T]) *MySqlQueryBuilder[T]) *MySqlQueryBuilder[T] {
	if condition {
		return callback(qb)
	}
	return qb
}

func (qb *MySqlQueryBuilder[T]) Unless(condition bool, callback func(*MySqlQueryBuilder[T]) *MySqlQueryBuilder[T]) *MySqlQueryBuilder[T] {
	if !condition {
		return callback(qb)
	}
	return qb
}

// ========================
// Query Building
// ========================
func (qb *MySqlQueryBuilder[T]) buildSelectQuery() (string, []interface{}) {
	var query strings.Builder
	var args []interface{}

	// SELECT
	query.WriteString("SELECT ")
	query.WriteString(strings.Join(qb.columns, ", "))

	// FROM
	query.WriteString(" FROM ")
	query.WriteString(qb.service.TableName)

	// JOINS
	for _, join := range qb.joins {
		switch join.Type {
		case "inner":
			query.WriteString(" INNER JOIN ")
		case "left":
			query.WriteString(" LEFT JOIN ")
		case "right":
			query.WriteString(" RIGHT JOIN ")
		case "cross":
			query.WriteString(" CROSS JOIN ")
		}
		query.WriteString(join.Table)
		if join.Type != "cross" {
			query.WriteString(" ON ")
			query.WriteString(join.First)
			query.WriteString(" ")
			query.WriteString(join.Operator)
			query.WriteString(" ")
			query.WriteString(join.Second)
		}
	}

	// WHERE
	if len(qb.wheres) > 0 {
		query.WriteString(" WHERE ")
		whereSQL, whereArgs := qb.buildWhereClause()
		query.WriteString(whereSQL)
		args = append(args, whereArgs...)
	}

	// GROUP BY
	if len(qb.groups) > 0 {
		query.WriteString(" GROUP BY ")
		query.WriteString(strings.Join(qb.groups, ", "))
	}

	// HAVING
	if len(qb.havings) > 0 {
		query.WriteString(" HAVING ")
		havingSQL, havingArgs := qb.buildHavingClause()
		query.WriteString(havingSQL)
		args = append(args, havingArgs...)
	}

	// ORDER BY
	if len(qb.orders) > 0 {
		query.WriteString(" ORDER BY ")
		var orderParts []string
		for _, order := range qb.orders {
			orderParts = append(orderParts, order.Column+" "+strings.ToUpper(order.Direction))
		}
		query.WriteString(strings.Join(orderParts, ", "))
	}

	// LIMIT
	if qb.limit > 0 {
		query.WriteString(fmt.Sprintf(" LIMIT %d", qb.limit))
	}

	// OFFSET
	if qb.offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", qb.offset))
	}

	return query.String(), args
}

func (qb *MySqlQueryBuilder[T]) buildWhereClause() (string, []interface{}) {
	var parts []string
	var args []interface{}

	for i, where := range qb.wheres {
		var part string
		var whereArgs []interface{}

		// Add boolean operator (AND/OR) before clause (except for first)
		if i > 0 {
			part += strings.ToUpper(where.Boolean) + " "
		}

		switch where.Type {
		case "basic":
			part += where.Column + " " + where.Operator + " ?"
			whereArgs = append(whereArgs, where.Value)
		case "in":
			if where.Operator == "not" {
				part += where.Column + " NOT IN ("
			} else {
				part += where.Column + " IN ("
			}
			values := where.Value.([]interface{})
			placeholders := make([]string, len(values))
			for j, v := range values {
				placeholders[j] = "?"
				whereArgs = append(whereArgs, v)
			}
			part += strings.Join(placeholders, ", ") + ")"
		case "between":
			values := where.Value.([2]interface{})
			if where.Operator == "not" {
				part += where.Column + " NOT BETWEEN ? AND ?"
			} else {
				part += where.Column + " BETWEEN ? AND ?"
			}
			whereArgs = append(whereArgs, values[0], values[1])
		case "null":
			if where.Operator == "not" {
				part += where.Column + " IS NOT NULL"
			} else {
				part += where.Column + " IS NULL"
			}
		case "column":
			part += where.Column + " " + where.Operator + " " + where.Value.(string)
		case "raw":
			part += where.Column
			if bindings, ok := where.Value.([]interface{}); ok {
				whereArgs = append(whereArgs, bindings...)
			}
		}

		parts = append(parts, part)
		args = append(args, whereArgs...)
	}

	return strings.Join(parts, " "), args
}

func (qb *MySqlQueryBuilder[T]) buildHavingClause() (string, []interface{}) {
	var parts []string
	var args []interface{}

	for i, having := range qb.havings {
		var part string

		if i > 0 {
			part += strings.ToUpper(having.Boolean) + " "
		}

		part += having.Column + " " + having.Operator + " ?"
		parts = append(parts, part)
		args = append(args, having.Value)
	}

	return strings.Join(parts, " "), args
}

// ========================
// Result Retrieval (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) Get() ([]T, error) {
	query, args := qb.buildSelectQuery()
	rows, err := qb.service.DB.QueryContext(qb.service.Ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		model := qb.service.Factory()

		// Get column info
		columns, err := rows.Columns()
		if err != nil {
			continue
		}

		// Create scan targets
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Build data map
		data := make(map[string]interface{})
		for i, col := range columns {
			data[col] = values[i]
		}

		// Execute Retrieved hook
		if qb.service.Retrieved != nil {
			if err := qb.service.Retrieved(model); err != nil {
				continue
			}
		}

		// Hydrate model
		if err := qb.service.FromMap(&model, data); err != nil {
			continue
		}

		results = append(results, model)
	}

	return results, nil
}

func (qb *MySqlQueryBuilder[T]) First() (T, error) {
	var zero T
	results, err := qb.Take(1).Get()
	if err != nil || len(results) == 0 {
		return zero, errors.New("no records found")
	}
	return results[0], nil
}

func (qb *MySqlQueryBuilder[T]) FirstOrFail() (T, error) {
	result, err := qb.First()
	if err != nil {
		return result, errors.New("model not found")
	}
	return result, nil
}

func (qb *MySqlQueryBuilder[T]) Find(id interface{}) (T, error) {
	return qb.Where("id", id).First()
}

func (qb *MySqlQueryBuilder[T]) FindOrFail(id interface{}) (T, error) {
	result, err := qb.Find(id)
	if err != nil {
		return result, fmt.Errorf("no query results for model with id: %v", id)
	}
	return result, nil
}

func (qb *MySqlQueryBuilder[T]) FindMany(ids []interface{}) ([]T, error) {
	return qb.WhereIn("id", ids).Get()
}

// ========================
// Aggregates (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) Count(columns ...string) (int64, error) {
	column := "*"
	if len(columns) > 0 {
		column = columns[0]
	}

	qb.columns = []string{fmt.Sprintf("COUNT(%s) as aggregate", column)}
	query, args := qb.buildSelectQuery()

	var count int64
	err := qb.service.DB.QueryRowContext(qb.service.Ctx, query, args...).Scan(&count)
	return count, err
}

func (qb *MySqlQueryBuilder[T]) Max(column string) (interface{}, error) {
	qb.columns = []string{fmt.Sprintf("MAX(%s) as aggregate", column)}
	query, args := qb.buildSelectQuery()

	var max interface{}
	err := qb.service.DB.QueryRowContext(qb.service.Ctx, query, args...).Scan(&max)
	return max, err
}

func (qb *MySqlQueryBuilder[T]) Min(column string) (interface{}, error) {
	qb.columns = []string{fmt.Sprintf("MIN(%s) as aggregate", column)}
	query, args := qb.buildSelectQuery()

	var min interface{}
	err := qb.service.DB.QueryRowContext(qb.service.Ctx, query, args...).Scan(&min)
	return min, err
}

func (qb *MySqlQueryBuilder[T]) Sum(column string) (interface{}, error) {
	qb.columns = []string{fmt.Sprintf("SUM(%s) as aggregate", column)}
	query, args := qb.buildSelectQuery()

	var sum interface{}
	err := qb.service.DB.QueryRowContext(qb.service.Ctx, query, args...).Scan(&sum)
	return sum, err
}

func (qb *MySqlQueryBuilder[T]) Avg(column string) (interface{}, error) {
	qb.columns = []string{fmt.Sprintf("AVG(%s) as aggregate", column)}
	query, args := qb.buildSelectQuery()

	var avg interface{}
	err := qb.service.DB.QueryRowContext(qb.service.Ctx, query, args...).Scan(&avg)
	return avg, err
}

func (qb *MySqlQueryBuilder[T]) Exists() (bool, error) {
	count, err := qb.Count()
	return count > 0, err
}

func (qb *MySqlQueryBuilder[T]) DoesntExist() (bool, error) {
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

func (qb *MySqlQueryBuilder[T]) Paginate(page, perPage int) (*PaginationResult[T], error) {
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
	offset := (page - 1) * perPage
	lastPage := int((total + int64(perPage) - 1) / int64(perPage))

	// Get paginated results
	results, err := qb.Skip(offset).Take(perPage).Get()
	if err != nil {
		return nil, err
	}

	from := offset + 1
	to := offset + len(results)
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

func (qb *MySqlQueryBuilder[T]) SimplePaginate(page, perPage int) ([]T, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	offset := (page - 1) * perPage
	return qb.Skip(offset).Take(perPage + 1).Get() // +1 to check if there are more records
}

// ========================
// Chunking (Laravel-style)
// ========================
func (qb *MySqlQueryBuilder[T]) Chunk(size int, callback func([]T) error) error {
	if size <= 0 {
		return errors.New("chunk size must be greater than 0")
	}

	page := 1
	for {
		results, err := qb.Skip((page - 1) * size).Take(size).Get()
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

		page++
	}

	return nil
}

func (qb *MySqlQueryBuilder[T]) Each(callback func(T) error) error {
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
func (qb *MySqlQueryBuilder[T]) Update(values map[string]interface{}) (int64, error) {
	if len(qb.wheres) == 0 {
		return 0, errors.New("update requires WHERE clause for safety")
	}

	// Add updated_at timestamp
	model := qb.service.Factory()
	if model.IsTimestamps() {
		values[model.GetUpdatedAtColumn()] = time.Now()
	}

	var setParts []string
	var args []interface{}

	for column, value := range values {
		setParts = append(setParts, column+" = ?")
		args = append(args, value)
	}

	whereSQL, whereArgs := qb.buildWhereClause()
	args = append(args, whereArgs...)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		qb.service.TableName,
		strings.Join(setParts, ", "),
		whereSQL)

	result, err := qb.service.DB.ExecContext(qb.service.Ctx, query, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (qb *MySqlQueryBuilder[T]) Increment(column string, amount ...int) (int64, error) {
	value := 1
	if len(amount) > 0 {
		value = amount[0]
	}

	updates := map[string]interface{}{
		column: fmt.Sprintf("%s + %d", column, value),
	}

	return qb.UpdateRaw(updates)
}

func (qb *MySqlQueryBuilder[T]) Decrement(column string, amount ...int) (int64, error) {
	value := 1
	if len(amount) > 0 {
		value = amount[0]
	}

	updates := map[string]interface{}{
		column: fmt.Sprintf("%s - %d", column, value),
	}

	return qb.UpdateRaw(updates)
}

func (qb *MySqlQueryBuilder[T]) UpdateRaw(values map[string]interface{}) (int64, error) {
	if len(qb.wheres) == 0 {
		return 0, errors.New("update requires WHERE clause for safety")
	}

	var setParts []string
	var args []interface{}

	for column, value := range values {
		if strVal, ok := value.(string); ok && strings.Contains(strVal, column) {
			// Raw expression
			setParts = append(setParts, column+" = "+strVal)
		} else {
			setParts = append(setParts, column+" = ?")
			args = append(args, value)
		}
	}

	whereSQL, whereArgs := qb.buildWhereClause()
	args = append(args, whereArgs...)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		qb.service.TableName,
		strings.Join(setParts, ", "),
		whereSQL)

	result, err := qb.service.DB.ExecContext(qb.service.Ctx, query, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (qb *MySqlQueryBuilder[T]) Delete() (int64, error) {
	if len(qb.wheres) == 0 {
		return 0, errors.New("delete requires WHERE clause for safety")
	}

	model := qb.service.Factory()

	// Soft delete if enabled
	if model.IsSoftDeletes() && !qb.forceDelete {
		updates := map[string]interface{}{
			model.GetDeletedAtColumn(): time.Now(),
		}
		return qb.Update(updates)
	}

	// Hard delete
	whereSQL, args := qb.buildWhereClause()
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", qb.service.TableName, whereSQL)

	result, err := qb.service.DB.ExecContext(qb.service.Ctx, query, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (qb *MySqlQueryBuilder[T]) ForceDelete() (int64, error) {
	qb.forceDelete = true
	return qb.Delete()
}

func (qb *MySqlQueryBuilder[T]) Restore() (int64, error) {
	model := qb.service.Factory()
	if !model.IsSoftDeletes() {
		return 0, errors.New("model does not support soft deletes")
	}

	updates := map[string]interface{}{
		model.GetDeletedAtColumn(): nil,
	}

	return qb.Update(updates)
}

// ========================
// Eloquent Model Methods (Laravel-style)
// ========================
func (s *BaseEloquentService[T]) All(columns ...string) ([]T, error) {
	qb := s.NewQuery()
	if len(columns) > 0 {
		qb = qb.Select(columns...)
	}
	return qb.Get()
}

func (s *BaseEloquentService[T]) Find(id interface{}, columns ...string) (T, error) {
	qb := s.NewQuery().Where("id", id)
	if len(columns) > 0 {
		qb = qb.Select(columns...)
	}
	return qb.First()
}

func (s *BaseEloquentService[T]) FindOrFail(id interface{}, columns ...string) (T, error) {
	qb := s.NewQuery().Where("id", id)
	if len(columns) > 0 {
		qb = qb.Select(columns...)
	}
	return qb.FirstOrFail()
}

func (s *BaseEloquentService[T]) FindMany(ids []interface{}, columns ...string) ([]T, error) {
	qb := s.NewQuery().WhereIn("id", ids)
	if len(columns) > 0 {
		qb = qb.Select(columns...)
	}
	return qb.Get()
}

func (s *BaseEloquentService[T]) First(columns ...string) (T, error) {
	qb := s.NewQuery()
	if len(columns) > 0 {
		qb = qb.Select(columns...)
	}
	return qb.First()
}

func (s *BaseEloquentService[T]) FirstOrFail(columns ...string) (T, error) {
	qb := s.NewQuery()
	if len(columns) > 0 {
		qb = qb.Select(columns...)
	}
	return qb.FirstOrFail()
}

func (s *BaseEloquentService[T]) FirstOrCreate(attributes map[string]interface{}, values ...map[string]interface{}) (T, error) {
	qb := s.NewQuery()
	for key, value := range attributes {
		qb = qb.Where(key, value)
	}

	model, err := qb.First()
	if err == nil {
		return model, nil
	}

	// Create new model
	createData := make(map[string]interface{})
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

func (s *BaseEloquentService[T]) FirstOrNew(attributes map[string]interface{}, values ...map[string]interface{}) (T, bool, error) {
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
	createData := make(map[string]interface{})
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

func (s *BaseEloquentService[T]) UpdateOrCreate(attributes map[string]interface{}, values map[string]interface{}) (T, error) {
	qb := s.NewQuery()
	for key, value := range attributes {
		qb = qb.Where(key, value)
	}

	model, err := qb.First()
	if err == nil {
		// Update existing
		modelMap, _ := s.ToMap(model)
		id := modelMap["id"]
		_, updateErr := s.NewQuery().Where("id", id).Update(values)
		if updateErr != nil {
			return model, updateErr
		}
		// Reload model
		return s.Find(id)
	}

	// Create new
	createData := make(map[string]interface{})
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
func (s *BaseEloquentService[T]) Create(attributes map[string]interface{}) (T, error) {
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

	// Build INSERT query
	var columns []string
	var placeholders []string
	var values []interface{}

	for column, value := range filteredAttrs {
		columns = append(columns, column)
		placeholders = append(placeholders, "?")
		values = append(values, value)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		s.TableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	result, err := s.DB.ExecContext(s.Ctx, query, values...)
	if err != nil {
		return model, err
	}

	// Get last insert ID if available
	if lastID, err := result.LastInsertId(); err == nil && lastID > 0 {
		filteredAttrs["id"] = lastID
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

	// Check if model exists (has ID)
	if id, exists := modelMap["id"]; exists && id != nil && id != "" {
		// Update existing
		delete(modelMap, "id") // Don't update ID

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
		_, err := s.NewQuery().Where("id", id).Update(filteredAttrs)
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

		deleted, err := s.NewQuery().Where("id", id).Delete()
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
func (s *BaseEloquentService[T]) filterAttributes(attributes map[string]interface{}) map[string]interface{} {
	model := s.Factory()
	fillable := model.GetFillable()
	guarded := model.GetGuarded()

	filtered := make(map[string]interface{})

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
func (s *BaseEloquentService[T]) AddGlobalScope(name string, scope func(*MySqlQueryBuilder[T]) *MySqlQueryBuilder[T]) {
	s.GlobalScopes[name] = scope
}

func (s *BaseEloquentService[T]) WithoutGlobalScope(name string) *MySqlQueryBuilder[T] {
	qb := &MySqlQueryBuilder[T]{
		service:  s,
		wheres:   []WhereClause{},
		orders:   []OrderClause{},
		joins:    []JoinClause{},
		groups:   []string{},
		havings:  []HavingClause{},
		limit:    -1,
		columns:  []string{"*"},
		bindings: make(map[string][]interface{}),
		scopes:   []string{},
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

func (s *BaseEloquentService[T]) WithoutGlobalScopes(names ...string) *MySqlQueryBuilder[T] {
	qb := &MySqlQueryBuilder[T]{
		service:  s,
		wheres:   []WhereClause{},
		orders:   []OrderClause{},
		joins:    []JoinClause{},
		groups:   []string{},
		havings:  []HavingClause{},
		limit:    -1,
		columns:  []string{"*"},
		bindings: make(map[string][]interface{}),
		scopes:   []string{},
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
func (s *BaseEloquentService[T]) ToMap(model T) (map[string]interface{}, error) {
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	return m, err
}

func (s *BaseEloquentService[T]) FromMap(model *T, data map[string]interface{}) error {
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

	id, exists := modelMap["id"]
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
func (s *BaseEloquentService[T]) Pluck(column string, key ...string) (map[interface{}]interface{}, error) {
	var selectColumns []string
	if len(key) > 0 {
		selectColumns = []string{key[0], column}
	} else {
		selectColumns = []string{column}
	}

	results, err := s.NewQuery().Select(selectColumns...).Get()
	if err != nil {
		return nil, err
	}

	plucked := make(map[interface{}]interface{})

	for _, result := range results {
		resultMap, err := s.ToMap(result)
		if err != nil {
			continue
		}

		value := resultMap[column]

		if len(key) > 0 {
			keyValue := resultMap[key[0]]
			plucked[keyValue] = value
		} else {
			plucked[len(plucked)] = value
		}
	}

	return plucked, nil
}
