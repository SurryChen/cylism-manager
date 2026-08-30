package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
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

func TestK8sUnavailableUsesStableAPIError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	K8sUnavailable(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("K8sUnavailable status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response model.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != model.CodeK8sUnavailable || response.Message != "K8s 集群未连接" {
		t.Fatalf("unexpected response: %#v", response)
	}
}
