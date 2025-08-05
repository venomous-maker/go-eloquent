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
)

// ========================
// BaseService
// ========================
type BaseService[T GlobalModels.ORMModel] struct {
	Ctx       context.Context
	DB        *sql.DB
	Factory   func() T
	TableName string
	// Lifecycle hooks
	BeforeSave   func(model T) error
	AfterCreate  func(model T) error
	AfterUpdate  func(model T) error
	BeforeDelete func(id string) error
	AfterDelete  func(id string) error
	BeforeFetch  func(model T) error
	AfterFetch   func(model T) error
}

func NewBaseService[T GlobalModels.ORMModel](ctx context.Context, db *sql.DB, factory func() T) *BaseService[T] {
	model := factory()
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	tableName := StringLibs.Pluralize(StringLibs.ConvertToSnakeCase(t.Name()))
	return &BaseService[T]{
		Ctx:       ctx,
		DB:        db,
		Factory:   factory,
		TableName: tableName,
	}
}

// ========================
// MySQLQueryBuilder
// ========================
type MySQLQueryBuilder[T GlobalModels.ORMModel] struct {
	service *BaseService[T]
	where   []string
	args    []interface{}
	order   string
	limit   int
	offset  int
	columns []string
}

func (s *BaseService[T]) Query() *MySQLQueryBuilder[T] {
	return &MySQLQueryBuilder[T]{
		service: s,
		where:   []string{},
		args:    []interface{}{},
		limit:   -1,
		columns: []string{"*"},
	}
}

func (qb *MySQLQueryBuilder[T]) Where(condition string, args ...interface{}) *MySQLQueryBuilder[T] {
	qb.where = append(qb.where, condition)
	qb.args = append(qb.args, args...)
	return qb
}

func (qb *MySQLQueryBuilder[T]) OrderBy(order string) *MySQLQueryBuilder[T] {
	qb.order = order
	return qb
}

func (qb *MySQLQueryBuilder[T]) Limit(limit int) *MySQLQueryBuilder[T] {
	qb.limit = limit
	return qb
}

func (qb *MySQLQueryBuilder[T]) Offset(offset int) *MySQLQueryBuilder[T] {
	qb.offset = offset
	return qb
}

func (qb *MySQLQueryBuilder[T]) Select(columns ...string) *MySQLQueryBuilder[T] {
	if len(columns) > 0 {
		qb.columns = columns
	}
	return qb
}

func (qb *MySQLQueryBuilder[T]) buildQuery() (string, []interface{}) {
	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(qb.columns, ","), qb.service.TableName)
	if len(qb.where) > 0 {
		query += " WHERE " + strings.Join(qb.where, " AND ")
	}
	if qb.order != "" {
		query += " ORDER BY " + qb.order
	}
	if qb.limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", qb.limit)
	}
	if qb.offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", qb.offset)
	}
	return query, qb.args
}

func (qb *MySQLQueryBuilder[T]) Get() ([]T, error) {
	query, args := qb.buildQuery()
	rows, err := qb.service.DB.QueryContext(qb.service.Ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []T
	for rows.Next() {
		model := qb.service.Factory()
		columns, _ := qb.service.ToMap(model)
		columnNames := make([]string, 0, len(columns))
		for k := range columns {
			columnNames = append(columnNames, k)
		}
		columnPointers := make([]interface{}, len(columnNames))
		for i := range columnNames {
			var v interface{}
			columnPointers[i] = &v
		}
		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}
		data := map[string]interface{}{}
		for i, k := range columnNames {
			data[k] = *(columnPointers[i].(*interface{}))
		}
		if qb.service.BeforeFetch != nil {
			if err := qb.service.BeforeFetch(model); err != nil {
				continue
			}
		}
		qb.service.FromMap(&model, data)
		if qb.service.AfterFetch != nil {
			if err := qb.service.AfterFetch(model); err != nil {
				continue
			}
		}
		results = append(results, model)
	}
	return results, nil
}

func (qb *MySQLQueryBuilder[T]) First() (T, error) {
	var zero T
	qb.Limit(1)
	results, err := qb.Get()
	if err != nil || len(results) == 0 {
		return zero, errors.New("no record found")
	}
	return results[0], nil
}

func (qb *MySQLQueryBuilder[T]) Each(fn func(T) error) error {
	results, err := qb.Get()
	if err != nil {
		return err
	}
	for _, model := range results {
		if err := fn(model); err != nil {
			return err
		}
	}
	return nil
}

func (qb *MySQLQueryBuilder[T]) Chunk(chunkSize int, fn func([]T) error) error {
	if chunkSize <= 0 {
		return errors.New("chunk size must be > 0")
	}
	page := 0
	for {
		qb.Limit(chunkSize).Offset(page * chunkSize)
		results, err := qb.Get()
		if err != nil {
			return err
		}
		if len(results) == 0 {
			break
		}
		if err := fn(results); err != nil {
			return err
		}
		if len(results) < chunkSize {
			break
		}
		page++
	}
	return nil
}

// ========================
// Additional QueryBuilder Methods to match MongoDB
// ========================
func (qb *MySQLQueryBuilder[T]) Apply(scope func(*MySQLQueryBuilder[T]) *MySQLQueryBuilder[T]) *MySQLQueryBuilder[T] {
	return scope(qb)
}

// Soft delete support (assumes a deleted_at column)
func (qb *MySQLQueryBuilder[T]) WithTrashed() *MySQLQueryBuilder[T] {
	// Remove any deleted_at IS NULL filter
	var newWhere []string
	var newArgs []interface{}
	for i, cond := range qb.where {
		if !strings.Contains(cond, "deleted_at") {
			newWhere = append(newWhere, cond)
			newArgs = append(newArgs, qb.args[i])
		}
	}
	qb.where = newWhere
	qb.args = newArgs
	return qb
}

func (qb *MySQLQueryBuilder[T]) OnlyTrashed() *MySQLQueryBuilder[T] {
	return qb.Where("deleted_at IS NOT NULL")
}

func (qb *MySQLQueryBuilder[T]) Active() *MySQLQueryBuilder[T] {
	return qb.Where("deleted_at IS NULL")
}

func (qb *MySQLQueryBuilder[T]) Paginate(page, limit int) ([]T, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit
	qb.Limit(limit).Offset(offset)
	results, err := qb.Get()
	if err != nil {
		return nil, 0, err
	}
	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", qb.service.TableName)
	if len(qb.where) > 0 {
		countQuery += " WHERE " + strings.Join(qb.where, " AND ")
	}
	row := qb.service.DB.QueryRowContext(qb.service.Ctx, countQuery, qb.args...)
	var total int
	if err := row.Scan(&total); err != nil {
		return results, 0, err
	}
	return results, total, nil
}

func (qb *MySQLQueryBuilder[T]) UpdateByID(id string, data map[string]interface{}) error {
	return qb.Where("id = ?", id).Update(data)
}

func (qb *MySQLQueryBuilder[T]) Restore() error {
	return qb.Update(map[string]interface{}{"deleted_at": nil})
}

func (qb *MySQLQueryBuilder[T]) ForceDelete() error {
	return qb.Delete()
}

// Add missing Update and Delete methods to MySQLQueryBuilder
func (qb *MySQLQueryBuilder[T]) Update(data map[string]interface{}) error {
	if len(qb.where) == 0 {
		return errors.New("Update requires a WHERE clause")
	}
	var sets []string
	var values []interface{}
	for k, v := range data {
		sets = append(sets, fmt.Sprintf("%s = ?", k))
		values = append(values, v)
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", qb.service.TableName, strings.Join(sets, ", "), strings.Join(qb.where, " AND "))
	_, err := qb.service.DB.ExecContext(qb.service.Ctx, query, append(values, qb.args...)...)
	return err
}

func (qb *MySQLQueryBuilder[T]) Delete() error {
	if len(qb.where) == 0 {
		return errors.New("Delete requires a WHERE clause")
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", qb.service.TableName, strings.Join(qb.where, " AND "))
	_, err := qb.service.DB.ExecContext(qb.service.Ctx, query, qb.args...)
	return err
}

// ========================
// BaseService Methods (MySQL)
// ========================
func (s *BaseService[T]) GetTableName() string          { return s.TableName }
func (s *BaseService[T]) GetCtx() context.Context       { return s.Ctx }
func (s *BaseService[T]) GetFactory() func() T          { return s.Factory }
func (s *BaseService[T]) GetBeforeFetch() func(T) error { return s.BeforeFetch }
func (s *BaseService[T]) GetAfterFetch() func(T) error  { return s.AfterFetch }

func (s *BaseService[T]) ToMap(model T) (map[string]interface{}, error) {
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	return m, err
}

func (s *BaseService[T]) FromMap(model *T, data map[string]interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, model)
}

// ========================
// Additional BaseService Methods to match MongoDB
// ========================
func (s *BaseService[T]) Find(id string) (T, error) {
	qb := s.Query().Where("id = ?", id)
	return qb.First()
}

func (s *BaseService[T]) FindOrFail(id string) (T, error) {
	result, err := s.Find(id)
	if err != nil {
		return result, errors.New("record with id " + id + " not found")
	}
	return result, nil
}

func (s *BaseService[T]) Save(model T) (T, error) {
	columns, err := s.ToMap(model)
	if err != nil {
		return model, err
	}
	var keys []string
	var placeholders []string
	var values []interface{}
	for k, v := range columns {
		keys = append(keys, k)
		placeholders = append(placeholders, "?")
		values = append(values, v)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", s.TableName, strings.Join(keys, ","), strings.Join(placeholders, ","))
	_, err = s.DB.ExecContext(s.Ctx, query, values...)
	return model, err
}

func (s *BaseService[T]) Update(id string, updates map[string]interface{}) error {
	return s.Query().Where("id = ?", id).Update(updates)
}

func (s *BaseService[T]) Delete(id string) error {
	return s.Query().Where("id = ?", id).Delete()
}

func (s *BaseService[T]) Restore(id string) error {
	return s.Query().Where("id = ?", id).Restore()
}

func (s *BaseService[T]) ForceDelete(id string) error {
	return s.Query().Where("id = ?", id).ForceDelete()
}

func (s *BaseService[T]) UpdateOrCreate(id string, updates map[string]interface{}) (T, error) {
	var zero T
	_, err := s.Find(id)
	if err != nil {
		model := s.Factory()
		err := s.FromMap(&model, updates)
		if err != nil {
			return zero, err
		}
		return s.Save(model)
	}
	err = s.Update(id, updates)
	if err != nil {
		return zero, err
	}
	return s.Find(id)
}

func (s *BaseService[T]) CreateOrUpdate(model T) (T, error) {
	columns, err := s.ToMap(model)
	if err != nil {
		return model, err
	}
	id, ok := columns["id"].(string)
	if !ok || id == "" {
		return s.Save(model)
	}
	return s.UpdateOrCreate(id, columns)
}

func (s *BaseService[T]) FirstOrCreate(filter map[string]interface{}, newData T) (T, error) {
	qb := s.Query()
	for k, v := range filter {
		qb = qb.Where(fmt.Sprintf("%s = ?", k), v)
	}
	result, err := qb.First()
	if err != nil {
		return s.Save(newData)
	}
	return result, nil
}

func (s *BaseService[T]) Reload(model T) (T, error) {
	columns, err := s.ToMap(model)
	if err != nil {
		return model, err
	}
	id, ok := columns["id"].(string)
	if !ok || id == "" {
		return model, errors.New("missing id")
	}
	return s.Find(id)
}

func (s *BaseService[T]) NewInstance() BaseService[T] {
	return *s
}
