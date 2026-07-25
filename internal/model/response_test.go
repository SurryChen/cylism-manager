package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Success(c, []string{"a", "b"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Code != CodeSuccess {
		t.Errorf("expected code=%d, got %d", CodeSuccess, resp.Code)
	}
	if resp.Message != "ok" {
		t.Errorf("expected message='ok', got '%s'", resp.Message)
	}
	data, ok := resp.Data.([]interface{})
	if !ok || len(data) != 2 {
		t.Errorf("expected data=[a,b], got %v", resp.Data)
	}
}

func TestSuccess_NilData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Success(c, nil)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data != nil {
		t.Errorf("expected nil data, got %v", resp.Data)
	}
}

func TestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Error(c, http.StatusBadRequest, CodeBadRequest, "参数错误")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Code != CodeBadRequest {
		t.Errorf("expected code=%d, got %d", CodeBadRequest, resp.Code)
	}
	if resp.Message != "参数错误" {
		t.Errorf("expected message, got '%s'", resp.Message)
	}
	if resp.Data != nil {
		t.Errorf("expected nil data, got %v", resp.Data)
	}
}

func TestError_InternalServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Error(c, http.StatusInternalServerError, CodeInternalError, "数据库异常")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	var resp APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != CodeInternalError {
		t.Errorf("expected code=%d, got %d", CodeInternalError, resp.Code)
	}
}

func TestAPIResponse_FieldsMatchDesign(t *testing.T) {
	resp := APIResponse{Code: 0, Message: "ok", Data: nil}
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	if resp.Message != "ok" {
		t.Errorf("expected ok, got %s", resp.Message)
	}
}

func TestErrorCodes_Unique(t *testing.T) {
	codes := map[int]string{
		CodeSuccess:       "success",
		CodeBadRequest:    "bad_request",
		CodeUnauthorized:  "unauthorized",
		CodeTokenExpired:  "token_expired",
		CodeForbidden:     "forbidden",
		CodeNotFound:      "not_found",
		CodeConflict:      "conflict",
		CodeInternalError: "internal_error",
		CodeK8sUnavailable: "k8s_unavailable",
		CodeK8sAPIError:   "k8s_api_error",
		CodeDBError:       "db_error",
	}
	seen := make(map[int]bool)
	for c := range codes {
		if seen[c] {
			t.Errorf("duplicate error code: %d", c)
		}
		seen[c] = true
	}
}

func TestSuccessWithMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	SuccessWithMessage(c, map[string]int{"id": 1}, "创建成功")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestErrorWithData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ErrorWithData(c, http.StatusBadRequest, CodeValidationFail, "validation failed", []string{"name required"})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
