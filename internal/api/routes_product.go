package api

import (
	productapi "github.com/cylism/cylism-manager/internal/api/product"
	"github.com/gin-gonic/gin"
)

func registerProductRoutes(group *gin.RouterGroup, handler *productapi.Handler) {
	if handler == nil {
		return
	}
	products := group.Group("/products")
	products.GET("", handler.List)
	products.POST("", handler.Create)
	products.GET("/resource-options", handler.Options)
	products.GET("/:id", handler.Get)
	products.PUT("/:id", handler.Update)
}
