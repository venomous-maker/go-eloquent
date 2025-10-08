package base

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	BaseServices "github.com/venomous-maker/go-eloquent/Engine/Mongo/Base"
	BaseModels "github.com/venomous-maker/go-eloquent/Models/Base"
	RouteHelpers "github.com/venomous-maker/go-eloquent/Routes/Helpers"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BaseController[T BaseModels.MongoModel] struct {
	Service     BaseServices.EloquentServiceInterface[T]
	ExtraRoutes []RouteHelpers.ExtraRoute
}

// Index handles GET /resource?status=active|deleted|all
func (bc *BaseController[T]) Index(c *gin.Context) {
	status := c.DefaultQuery("status", "active")
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "1000000")

	pageInt, err := strconv.Atoi(page)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page number", "details": err.Error(), "status": "fail"})
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit number", "details": err.Error(), "status": "fail"})
		return
	}

	var results []T
	switch status {
	case "active":
		results, _, err = bc.Service.Query().OrderBy("created_at", "desc").Paginate(pageInt, limitInt)
		//results, _, err = bc.Service.Query().Paginate(pageInt, limitInt)
	case "deleted":
		results, _, err = bc.Service.Query().OnlyTrashed().OrderBy("created_at", "desc").Paginate(pageInt, limitInt)
	case "all":
		results, _, err = bc.Service.Query().WithTrashed().OrderBy("created_at", "desc").Paginate(pageInt, limitInt)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status filter: use active, deleted, or all", "status": "fail"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch records", "details": err.Error(), "status": "fail"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": results, "status": "success"})
}

// Show handles GET /resource/:id
func (bc *BaseController[T]) Show(c *gin.Context) {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID", "details": err.Error(), "status": "fail"})
		return
	}

	status := c.DefaultQuery("status", "active")
	var record T
	switch status {
	case "all":
		record, err = bc.Service.Query().WithTrashed().Find(objID.Hex())
	case "active":
		record, err = bc.Service.Find(objID.Hex())
	case "deleted":
		record, err = bc.Service.Query().OnlyTrashed().Find(objID.Hex())
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status filter: use active or deleted", "status": "fail"})
		return
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found", "details": err.Error(), "status": "fail"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": record, "status": "success"})
}

// Store handles POST /resource
func (bc *BaseController[T]) Store(c *gin.Context) {
	model := bc.Service.NewModel()
	if err := c.ShouldBindJSON(model); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error(), "status": "fail"})
		return
	}
	model.SetID(primitive.NewObjectID())
	model.SetTimestampsOnCreate()

	result, err := bc.Service.CreateOrUpdate(model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create record", "details": err.Error(), "status": "fail"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": result, "status": "success"})
}

// Update handles PUT /resource/:id
func (bc *BaseController[T]) Update(c *gin.Context) {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID", "details": err.Error(), "status": "fail"})
		return
	}

	model := bc.Service.NewModel()
	if err := c.ShouldBindJSON(model); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error(), "status": "fail"})
		return
	}

	bsonMap, err := bc.Service.ToBson(model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to convert model to bson", "details": err.Error(), "status": "fail"})
		return
	}
	found, err := bc.Service.Query().WithTrashed().Find(objID.Hex())
	if err != nil {
		return
	}

	status := c.DefaultQuery("status", "active")
	if found.GetDeletedAt() != (time.Time{}) && (status == "deleted" || status == "all") {
		err = bc.Service.Restore(objID.Hex())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore record before update", "details": err.Error(), "status": "fail"})
			return
		}

		err = bc.Service.Update(objID.Hex(), bsonMap)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update record", "details": err.Error(), "status": "fail"})
			return
		}

		err = bc.Service.Delete(objID.Hex())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete record after update", "details": err.Error(), "status": "fail"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Updated successfully", "status": "success"})
		return

	}

	err = bc.Service.Update(objID.Hex(), bsonMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update record", "details": err.Error(), "status": "fail"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Updated successfully", "status": "success"})
}

// Delete handles DELETE /resource/:id?force=true
func (bc *BaseController[T]) Delete(c *gin.Context) {
	id := c.Param("id")
	force := c.DefaultQuery("force", "false") == "true"

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID", "details": err.Error(), "status": "fail"})
		return
	}

	if force {
		err = bc.Service.ForceDelete(objID.Hex())
	} else {
		err = bc.Service.Delete(objID.Hex())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete record", "details": err.Error(), "status": "fail"})
		return
	}

	msg := "Soft deleted successfully"
	if force {
		msg = "Permanently deleted successfully"
	}

	c.JSON(http.StatusOK, gin.H{"message": msg, "status": "success"})
}

func (bc *BaseController[T]) Restore(c *gin.Context) {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID", "details": err.Error(), "status": "fail"})
		return
	}

	err = bc.Service.Restore(objID.Hex())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore record", "details": err.Error(), "status": "fail"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Restored successfully", "status": "success"})
}

func (bc *BaseController[T]) AddRoute(method, path string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	bc.ExtraRoutes = append(bc.ExtraRoutes, RouteHelpers.ExtraRoute{
		Method:     strings.ToUpper(method),
		Path:       path,
		Handler:    handler,
		Middleware: middleware,
	})
}

// RegisterRoutes registers standard CRUD routes for this resource, as well as any
// additional "extra" routes specified in the ExtraRoutes field.
//
// The standard CRUD routes are registered as follows:
//   - GET /{prefix}: Index all records
//   - GET /{prefix}/:id: Show a single record
//   - POST /{prefix}: Store a new record
//   - PUT /{prefix}/:id: Update a record
//   - DELETE /{prefix}/:id: Delete a record
//   - PUT /{prefix}/:id/restore: Restore a soft deleted record
//
// Additionally, any routes specified in the ExtraRoutes field will be registered
// recursively. This allows for the creation of arbitrary route structures.
//
// The routes are registered in the order that they are specified, so the standard
// CRUD routes will always be registered before the extra routes.
//
// Example usage:
//
// userController := &UserController{}
// userController.AddRoute(http.MethodGet, "stats", statsHandler)
//
//	userController.ExtraRoutes = append(userController.ExtraRoutes, RouteHelpers.ExtraRoute{
//		Group: "/:userId/benefits",
//		Middleware: []gin.HandlerFunc{UserAuthMiddleware},
//		Children: []RouteHelpers.ExtraRoute{
//			{
//				Method:  http.MethodGet,
//				Path:    "",
//				Handler: ListBenefits,
//			},
//			{
//				Method:  http.MethodPost,
//				Path:    "",
//				Handler: CreateBenefit,
//				Middleware: []gin.HandlerFunc{CheckBenefitPermission},
//			},
//		},
//	})
func (bc *BaseController[T]) RegisterRoutes(router *gin.RouterGroup) {
	prefix := bc.Service.GetRoutePrefix()
	group := router.Group(prefix)

	// Standard CRUD
	group.GET("", bc.Index)
	group.GET("/:id", bc.Show)
	group.POST("", bc.Store)
	group.PUT("/:id", bc.Update)
	group.DELETE("/:id", bc.Delete)
	group.PUT("/:id/restore", bc.Restore)

	// Register dynamic/extra routes (including nested)
	for _, route := range bc.ExtraRoutes {
		bc.RegisterRouteRecursive(group, route)
	}
}

// RegisterRouteRecursive registers a route recursively based on the given ExtraRoute
// object. If the given route has a method and path, it will be registered with the
// given gin.RouterGroup. If the route has a group prefix, it will be applied before
// registering the route. If the route has middleware, it will be applied before
// registering the route. Finally, any child routes will be registered recursively.
//
// The route will be registered with the following methods:
//   - GET
//   - POST
//   - PUT
//   - DELETE
//   - PATCH
//   - HEAD
//   - OPTIONS
func (bc *BaseController[T]) RegisterRouteRecursive(group *gin.RouterGroup, route RouteHelpers.ExtraRoute) {
	var subGroup *gin.RouterGroup

	// Apply group prefix and middleware if present
	if route.Group != "" {
		subGroup = group.Group(route.Group, route.Middleware...)
	} else if len(route.Middleware) > 0 {
		subGroup = group.Group("", route.Middleware...)
	} else {
		subGroup = group
	}

	// Register current route if it has method and path
	if route.Method != "" && route.Path != "" && route.Handler != nil {
		handlers := append(route.Middleware, route.Handler) // apply middleware if defined at this level too
		switch route.Method {
		case http.MethodGet:
			subGroup.GET(route.Path, handlers...)
		case http.MethodPost:
			subGroup.POST(route.Path, handlers...)
		case http.MethodPut:
			subGroup.PUT(route.Path, handlers...)
		case http.MethodDelete:
			subGroup.DELETE(route.Path, handlers...)
		case http.MethodPatch:
			subGroup.PATCH(route.Path, handlers...)
		case http.MethodHead:
			subGroup.HEAD(route.Path, handlers...)
		case http.MethodOptions:
			subGroup.OPTIONS(route.Path, handlers...)
		}
	}

	// Register child routes recursively
	for _, child := range route.Children {
		bc.RegisterRouteRecursive(subGroup, child)
	}
}
