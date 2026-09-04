package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserIDReadsAuthenticatedContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", uint(42))
	if got := UserID(c); got != 42 {
		t.Fatalf("UserID() = %d, want 42", got)
	}
}

func TestUserIDReturnsZeroForMissingOrMalformedContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := UserID(c); got != 0 {
		t.Fatalf("missing UserID() = %d, want 0", got)
	}
	c.Set("user_id", "42")
	if got := UserID(c); got != 0 {
		t.Fatalf("malformed UserID() = %d, want 0", got)
	}
}

func TestParseIDPreservesNumericParsingBehavior(t *testing.T) {
	if got, err := ParseID("17"); err != nil || got != 17 {
		t.Fatalf("ParseID(17) = %d, %v", got, err)
	}
	if _, err := ParseID("not-a-number"); err == nil {
		t.Fatal("ParseID should reject non-numeric values")
	}
	if _, err := ParsePositiveID("0"); err == nil {
		t.Fatal("ParsePositiveID should reject zero")
	}
}

func TestOptionalIDAndIdentityHelpers(t *testing.T) {
	if got, err := OptionalID(""); err != nil || got != 0 {
		t.Fatalf("OptionalID(empty) = %d, %v", got, err)
	}
	if got, err := OptionalID("9"); err != nil || got != 9 {
		t.Fatalf("OptionalID(9) = %d, %v", got, err)
	}
	if _, err := OptionalID("0"); err == nil {
		t.Fatal("OptionalID should reject zero")
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", uint64(12))
	c.Set("username", "alice")
	if got := UserID(c); got != 12 {
		t.Fatalf("UserID = %d, want 12", got)
	}
	if got := Username(c); got != "alice" {
		t.Fatalf("Username = %q, want alice", got)
	}
}

func TestK8sUnavailableUsesStableAPIError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	K8sUnavailable(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("K8sUnavailable status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != CodeK8sUnavailable || response.Message != "K8s 集群未连接" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestPaginationNormalizesBounds(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?page=0&size=1000", nil)
	page, size, offset := Pagination(c)
	if page != 1 || size != 100 || offset != 0 {
		t.Fatalf("Pagination() = %d, %d, %d", page, size, offset)
	}
}

func TestLimitOffsetNormalizesBounds(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?limit=999&offset=-2", nil)
	limit, offset := LimitOffset(c, 20, 100)
	if limit != 100 || offset != 0 {
		t.Fatalf("LimitOffset() = %d, %d", limit, offset)
	}
}

func TestCommonErrorHelpersUseStableCodes(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*gin.Context)
		code int
	}{
		{"bad request", func(c *gin.Context) { BadRequest(c, "bad") }, CodeBadRequest},
		{"validation", func(c *gin.Context) { ValidationError(c, "invalid") }, CodeValidationFail},
		{"not found", func(c *gin.Context) { NotFound(c, "missing") }, CodeNotFound},
		{"conflict", func(c *gin.Context) { Conflict(c, "conflict") }, CodeConflict},
		{"internal", func(c *gin.Context) { InternalError(c, "error") }, CodeInternalError},
		{"database", func(c *gin.Context) { DBError(c, "db") }, CodeDBError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			tc.fn(c)
			var response APIResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.code {
				t.Fatalf("code = %d, want %d", response.Code, tc.code)
			}
		})
	}
}
