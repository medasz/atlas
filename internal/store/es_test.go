package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestESGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	es := NewES(srv.URL, "assets")
	if _, err := es.Get(context.Background(), "host:1.2.3.4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
