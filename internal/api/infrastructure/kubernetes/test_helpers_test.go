package kubernetes

import (
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

func serve(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
