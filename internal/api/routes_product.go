package api

import (
	productapi "github.com/cylism/cylism-manager/internal/api/product"
	"github.com/gin-gonic/gin"
)

func registerProductRoutes(group *gin.RouterGroup, handler *productapi.Handler) {
	if handler == nil {
		return
	}
	group.GET("/projects/:projectID/workspace", handler.Workspace)
	group.DELETE("/projects/:projectID/workspace", handler.DeleteWorkspace)
	products := group.Group("/products")
	products.GET("", handler.List)
	products.POST("", handler.Create)
	products.POST("/clear", handler.Clear)
	products.GET("/resource-options", handler.Options)
	products.GET("/workbench", handler.Workbench)
	products.GET("/:id/lifecycle-options", handler.LifecycleOptions)
	products.POST("/:id/import-next-action", handler.ImportNextAction)
	products.GET("/:id/lifecycle/:kind", handler.ListLifecycle)
	products.POST("/:id/lifecycle/:kind", handler.SaveLifecycle)
	products.PUT("/:id/lifecycle/:kind/:recordID", handler.SaveLifecycle)
	products.DELETE("/:id/lifecycle/tasks/:recordID", handler.DeleteTask)
	products.DELETE("/:id/lifecycle/release-units/:recordID", handler.DeleteReleaseUnit)
	products.GET("/:id/distributions/:recordID/events", handler.DistributionEvents)
	products.GET("/:id", handler.Get)
	products.PUT("/:id", handler.Update)
	products.DELETE("/:id", handler.Delete)
}
