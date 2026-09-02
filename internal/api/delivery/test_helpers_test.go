package delivery

import (
	"encoding/json"
	"testing"
)

func responseID(t *testing.T, body []byte) uint {
	t.Helper()
	var v struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	return v.Data.ID
}
