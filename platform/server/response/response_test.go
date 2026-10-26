package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestJSONWrapsData(t *testing.T) {
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		return JSON(c, http.StatusOK, map[string]string{"hello": "world"})
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data["hello"] != "world" {
		t.Fatalf("data = %v, want hello=world", body.Data)
	}
}
