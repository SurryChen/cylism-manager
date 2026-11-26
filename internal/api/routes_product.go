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
	integration := group.Group("/settings/github")
	integration.GET("", handler.GitHubConnection)
	integration.POST("/manifest", handler.StartGitHubRegistration)
	integration.DELETE("", handler.DeleteGitHubConnection)
	integration.GET("/installations", handler.GitHubInstallations)
	integration.GET("/installations/:installationID/repositories", handler.GitHubInstallationRepositories)
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
	products.DELETE("/:id/lifecycle/task-groups/:recordID", handler.DeleteTaskGroup)
	products.DELETE("/:id/lifecycle/release-units/:recordID", handler.DeleteReleaseUnit)
	products.GET("/:id/distributions/:recordID/events", handler.DistributionEvents)
	products.GET("/:id/github/repositories", handler.GitHubProductRepositories)
	products.POST("/:id/github/repositories", handler.BindGitHubRepository)
	products.DELETE("/:id/github/repositories/:bindingID", handler.UnbindGitHubRepository)
	products.GET("/:id/github/repositories/:bindingID/revisions", handler.GitHubRevisions)
	products.GET("/:id", handler.Get)
	products.PUT("/:id", handler.Update)
	products.DELETE("/:id", handler.Delete)
}
