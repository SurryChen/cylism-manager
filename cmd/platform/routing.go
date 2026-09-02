package main

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

func registerFrontendRoutes(r *gin.Engine, distDir string) {
	r.Static("/assets", filepath.Join(distDir, "assets"))
	r.NoRoute(func(c *gin.Context) {
		// API typos must remain JSON 404s; only browser application routes use
		// the SPA fallback document.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "API 路径不存在")
			return
		}
		c.File(filepath.Join(distDir, "index.html"))
	})
}
