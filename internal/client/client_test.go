package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppStatusUsesProjectScopedAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/projects/billing/apps/api/status" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer project-token" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"state":"active"}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, Token: "project-token", Project: "billing"}
	status, err := client.AppStatus(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "active" {
		t.Fatalf("state=%q", status.State)
	}
}

func TestProjectIsRequiredBeforeRemoteRequest(t *testing.T) {
	client := &Client{Endpoint: "http://127.0.0.1:1", Token: "unused"}
	if _, err := client.AppStatus(context.Background(), "api"); err == nil {
		t.Fatal("request without project unexpectedly succeeded")
	}
}
