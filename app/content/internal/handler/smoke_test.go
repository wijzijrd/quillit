package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSmoke_Parse_ReturnsParsedEntry(t *testing.T) {
	s := NewSmoke()
	req := httptest.NewRequest(http.MethodGet, "/smoke", nil)
	w := httptest.NewRecorder()

	s.Parse(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var got struct {
		Frontmatter struct {
			Name string   `json:"Name"`
			Tags []string `json:"Tags"`
		} `json:"Frontmatter"`
		Body []struct {
			Kind string `json:"Kind"`
		} `json:"Body"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Frontmatter.Name != "Tom the Innkeeper" {
		t.Errorf("Frontmatter.Name = %q, want %q", got.Frontmatter.Name, "Tom the Innkeeper")
	}
	if len(got.Body) == 0 {
		t.Errorf("expected at least one parsed body block, got 0")
	}
}
