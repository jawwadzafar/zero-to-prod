package testdemo

import (
	"encoding/json"
	"net/http"
)

// Hello answers GET /hello?name=Ada with {"greeting": "hello, Ada"}.
func Hello(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "world"
	}
	if len(name) > 50 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "name is too long"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"greeting": "hello, " + name})
}
