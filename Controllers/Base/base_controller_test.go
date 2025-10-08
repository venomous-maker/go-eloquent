package base_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	BaseControllers "github.com/venomous-maker/go-eloquent/Controllers/Base"
	BaseServices "github.com/venomous-maker/go-eloquent/Engine/Mongo/Base"
	BaseModels "github.com/venomous-maker/go-eloquent/Models/Base"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Simple User model for tests
type TestUser struct {
	*BaseModels.BaseModel
	Name string `json:"name" bson:"name"`
}

// Provide GetCollectionName on pointer receiver to satisfy interface if needed
func (t *TestUser) GetCollectionName() string { return "" }

// MockService implements BaseServices.EloquentServiceInterface[*TestUser] with minimal behavior for tests.
type MockService struct{}

func (m *MockService) GetCtx() context.Context { return context.TODO() }
func (m *MockService) GetFactory() func() *TestUser {
	return func() *TestUser {
		return &TestUser{BaseModel: &BaseModels.BaseModel{}}
	}
}
func (m *MockService) Find(id string) (*TestUser, error) {
	return &TestUser{BaseModel: &BaseModels.BaseModel{}}, nil
}
func (m *MockService) FindOrFail(id string) (*TestUser, error) { return m.Find(id) }
func (m *MockService) Save(model *TestUser) (*TestUser, error) { return model, nil }
func (m *MockService) Update(id string, updates bson.M) error  { return nil }
func (m *MockService) Delete(id string) error                  { return nil }
func (m *MockService) Restore(id string) error                 { return nil }
func (m *MockService) ForceDelete(id string) error             { return nil }
func (m *MockService) Query() *BaseServices.Eloquent[*TestUser] {
	return &BaseServices.Eloquent[*TestUser]{}
}
func (m *MockService) UpdateOrCreate(id string, updates bson.M) (*TestUser, error) {
	return &TestUser{BaseModel: &BaseModels.BaseModel{}}, nil
}
func (m *MockService) CreateOrUpdate(model *TestUser) (*TestUser, error) { return model, nil }
func (m *MockService) FirstOrCreate(filter bson.M, newData *TestUser) (*TestUser, error) {
	return newData, nil
}
func (m *MockService) Reload(model *TestUser) (*TestUser, error) { return model, nil }
func (m *MockService) NewInstance() BaseServices.EloquentService[*TestUser] {
	var z BaseServices.EloquentService[*TestUser]
	return z
}
func (m *MockService) GetCollection() *mongo.Collection { return nil }
func (m *MockService) GetModel() *TestUser              { return &TestUser{BaseModel: &BaseModels.BaseModel{}} }
func (m *MockService) NewModel() *TestUser              { return &TestUser{BaseModel: &BaseModels.BaseModel{}} }
func (m *MockService) GetCollectionName() string        { return "" }
func (m *MockService) GetRoutePrefix() string           { return "testusers" }
func (m *MockService) ToBson(model *TestUser) (bson.M, error) {
	return bson.M{}, nil
}
func (m *MockService) ToJson(model *TestUser) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (m *MockService) FromBson(model **TestUser, data bson.M) error {
	*model = &TestUser{BaseModel: &BaseModels.BaseModel{}}
	return nil
}
func (m *MockService) FromJson(model **TestUser, data map[string]interface{}) error {
	*model = &TestUser{BaseModel: &BaseModels.BaseModel{}}
	return nil
}

func TestRegisterRoutes_ExtraRouteRegistration(t *testing.T) {
	// Use the mock service
	elo := &MockService{}

	bc := &BaseControllers.BaseController[*TestUser]{
		Service: elo,
	}

	// Add an extra route which simply returns 200
	handler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	bc.AddRoute(http.MethodGet, "stats", handler)

	// Create router group
	g := gin.New()
	router := g.Group("/api")

	// Register routes
	bc.RegisterRoutes(router)

	// Build request to the extra route: /api/{prefix}/stats
	prefix := elo.GetRoutePrefix()
	url := "/api/" + prefix + "/stats"

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}
