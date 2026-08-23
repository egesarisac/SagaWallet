package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterHealthEndpoints(t *testing.T) {
	router := newRouter("info")
	tests := []struct {
		path    string
		status  string
		service string
	}{
		{path: "/health", status: "healthy", service: "notification-service"},
		{path: "/ready", status: "ready"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
			}
			var response map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response["status"] != test.status {
				t.Fatalf("expected status %q, got %q", test.status, response["status"])
			}
			if response["service"] != test.service {
				t.Fatalf("expected service %q, got %q", test.service, response["service"])
			}
		})
	}
}
