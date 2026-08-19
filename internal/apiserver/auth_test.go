package apiserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/internal/storage"
	"path/filepath"
)

func setupTestDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nova.db")
	storage.Init(dbPath)
	storage.Reload()

	// Set a test API key
	if err := configmanager.SetSystemConfig("api_key", "test-api-key-123"); err != nil {
		t.Fatalf("failed to set api_key: %v", err)
	}
}

func TestRequireAPIKey_MissingHeader(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Test without Authorization header
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}

	if w.Body.String() != `{"success":false,"message":"missing Authorization header"}` {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestRequireAPIKey_InvalidFormat(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "missing Bearer prefix",
			authHeader:     "test-api-key-123",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"success":false,"message":"invalid Authorization header"}`,
		},
		{
			name:           "wrong scheme Basic",
			authHeader:     "Basic test-api-key-123",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"success":false,"message":"invalid Authorization header"}`,
		},
		{
			name:           "empty token",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"success":false,"message":"invalid Authorization header"}`,
		},
		{
			name:           "extra spaces",
			authHeader:     "Bearer    extra    spaces",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"success":false,"message":"invalid Authorization header"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %s, got %s", tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestRequireAPIKey_InvalidAPIKey(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-api-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}

	if w.Body.String() != `{"success":false,"message":"invalid api key"}` {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestRequireAPIKey_ValidAPIKey(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-api-key-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != `{"message":"success"}` {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestRequireAPIKey_CaseInsensitiveBearer(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	tests := []string{
		"Bearer test-api-key-123",
		"bearer test-api-key-123",
		"BEARER test-api-key-123",
		"BeArEr test-api-key-123",
	}

	for _, authHeader := range tests {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200 for header '%s', got %d", authHeader, w.Code)
		}
	}
}

func TestRequireAPIKey_ServerNotConfigured(t *testing.T) {
	// Setup DB but don't set api_key
	dbPath := filepath.Join(t.TempDir(), "nova.db")
	storage.Init(dbPath)
	storage.Reload()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-api-key-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	if w.Body.String() != `{"success":false,"message":"server api_key not configured"}` {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestHealthEndpoint_BypassesAuth(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	s := New()
	router := gin.New()
	s.RegisterRoutes(router)

	// Test /health without Authorization header
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for /health, got %d", w.Code)
	}

	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", w.Body.String())
	}
}

func TestProtectedEndpoints_RequireAuth(t *testing.T) {
	setupTestDB(t)

	gin.SetMode(gin.TestMode)
	s := New()
	router := gin.New()
	s.RegisterRoutes(router)

	protectedEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/apps/list", ""},
		{"POST", "/apps/start", `{"name":"test"}`},
		{"GET", "/node/status", ""},
		{"GET", "/ingress/list", ""},
		{"GET", "/config/testapp", ""},
	}

	for _, endpoint := range protectedEndpoints {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			if endpoint.body != "" {
				req = httptest.NewRequest(endpoint.method, endpoint.path, nil)
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401 for %s %s, got %d", endpoint.method, endpoint.path, w.Code)
			}
		})
	}
}
