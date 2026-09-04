package main

import (
	"net/http"
	"path/filepath"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

func registerFrontendRoutes(r *gin.Engine, distDir string) {
	r.Static("/assets", filepath.Join(distDir, "assets"))
	r.NoRoute(func(c *gin.Context) {
		// API typos must remain JSON 404s; only browser application routes use
		// the SPA fallback document.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			apiShared.Error(c, http.StatusNotFound, apiShared.CodeNotFound, "API 路径不存在")
			return
		}
		c.File(filepath.Join(distDir, "index.html"))
	})
}
