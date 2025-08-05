package GlobalRoutes

import "github.com/gin-gonic/gin"

// ExtraRoute represents a single route with its method, path, handler, middleware, optional subgroup, and nested routes.
type ExtraRoute struct {
	Method     string
	Path       string
	Handler    gin.HandlerFunc
	Middleware []gin.HandlerFunc
	Children   []ExtraRoute // ⬅ nested routes
	Group      string       // ⬅ optional subgroup prefix
}
