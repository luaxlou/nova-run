package agent

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/luaxlou/glow-ops/internal/artifact"
	"github.com/luaxlou/glow-ops/internal/authz"
	"github.com/luaxlou/glow-ops/internal/deploy"
)

func TestProjectTokenCanOnlyListItsOwnApps(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"billing--api", "orders--api"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := &authz.Store{Dir: t.TempDir()}
	billingToken, err := store.Set("billing")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{AppRoot: root, AuthStore: store}

	request := httptest.NewRequest(http.MethodGet, "/v2/projects/billing/apps", nil)
	request.Header.Set("Authorization", "Bearer "+billingToken)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"api"`)) || bytes.Contains(recorder.Body.Bytes(), []byte("orders")) {
		t.Fatalf("billing list status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v2/projects/orders/apps", nil)
	request.Header.Set("Authorization", "Bearer "+billingToken)
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("cross-project status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProjectScopedDeploySeparatesIdenticalAppNames(t *testing.T) {
	root := t.TempDir()
	store := &authz.Store{Dir: t.TempDir()}
	billingToken, _ := store.Set("billing")
	ordersToken, _ := store.Set("orders")
	server := &Server{AppRoot: root, AuthStore: store}

	deploy := func(project, token, contents string) {
		t.Helper()
		archive := deployArchive(t, root, false, contents)
		request := deployRequest(t, archive, "v1")
		request.URL.Path = "/v2/projects/" + project + "/apps/api"
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("deploy %s status=%d body=%s", project, recorder.Code, recorder.Body.String())
		}
	}
	deploy("billing", billingToken, "billing-content")
	deploy("orders", ordersToken, "orders-content")

	for runtimeName, want := range map[string]string{
		"billing--api": "billing-content",
		"orders--api":  "orders-content",
	} {
		got, err := os.ReadFile(filepath.Join(root, runtimeName, "index.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s contents=%q want=%q", runtimeName, got, want)
		}
	}

	request := httptest.NewRequest(http.MethodPut, "/v2/projects/orders/apps/other", nil)
	request.Header.Set("Authorization", "Bearer "+billingToken)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("cross-project deploy status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandleDeployRestartsServiceAfterArtifactAndMetadataAreSaved(t *testing.T) {
	root := t.TempDir()
	server := &Server{AppRoot: root}
	archive := deployArchive(t, root, true, "new-service")

	restarted := false
	server.restartService = func(name string) error {
		restarted = true
		if name != "api" {
			t.Fatalf("restart app = %q, want api", name)
		}
		if !artifact.HasRunBinary(filepath.Join(root, name)) {
			t.Fatal("service run binary was not installed before restart")
		}
		version, ok, err := deploy.CurrentVersion(root, name)
		if err != nil || !ok || version != "v2" {
			t.Fatalf("version at restart = %q, %t, %v; want v2, true, nil", version, ok, err)
		}
		return nil
	}

	recorder := httptest.NewRecorder()
	server.handleDeploy(recorder, deployRequest(t, archive, "v2"), "api")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !restarted {
		t.Fatal("service artifact was not restarted")
	}
}

func TestHandleDeployDoesNotRestartStaticArtifact(t *testing.T) {
	root := t.TempDir()
	server := &Server{AppRoot: root}
	archive := deployArchive(t, root, false, "static-site")

	server.restartService = func(string) error {
		t.Fatal("static artifact must not restart a service")
		return nil
	}

	recorder := httptest.NewRecorder()
	server.handleDeploy(recorder, deployRequest(t, archive, "v1"), "site")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestHandleDeployDoesNotRestartSkippedVersion(t *testing.T) {
	root := t.TempDir()
	server := &Server{AppRoot: root}
	appDir := filepath.Join(root, "api")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := deploy.SaveMetadata(appDir, deploy.Metadata{Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	archive := deployArchive(t, root, true, "replacement")

	server.restartService = func(string) error {
		t.Fatal("skipped deploy must not restart a service")
		return nil
	}

	recorder := httptest.NewRecorder()
	server.handleDeploy(recorder, deployRequest(t, archive, "v1"), "api")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(appDir, "run")); !os.IsNotExist(err) {
		t.Fatalf("skipped deploy replaced artifact: run stat error = %v, want not exist", err)
	}
}

func TestHandleDeployReturnsServerErrorWhenServiceRestartFailsAfterDeploy(t *testing.T) {
	root := t.TempDir()
	server := &Server{AppRoot: root}
	archive := deployArchive(t, root, true, "new-service")

	server.restartService = func(string) error { return errors.New("systemctl restart failed") }

	recorder := httptest.NewRecorder()
	server.handleDeploy(recorder, deployRequest(t, archive, "v2"), "api")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	// The replacement and version are durable despite the 500. A same-version
	// deploy remains idempotently skipped; recovery uses the explicit restart action.
	if !artifact.HasRunBinary(filepath.Join(root, "api")) {
		t.Fatal("deployed service artifact was not retained after restart failure")
	}
	version, ok, err := deploy.CurrentVersion(root, "api")
	if err != nil || !ok || version != "v2" {
		t.Fatalf("deployed version = %q, %t, %v; want v2, true, nil", version, ok, err)
	}
}

func deployRequest(t *testing.T, archive, version string) *http.Request {
	t.Helper()
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("artifact", filepath.Base(archive))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, file); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("version", version); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPut, "/v1/apps/api", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func deployArchive(t *testing.T, root string, service bool, contents string) string {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "index.txt"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if service {
		manifest := "process:\n  command: ./run\n"
		if err := os.WriteFile(filepath.Join(source, artifact.ManifestFile), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "run"), []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(root, contents+".tar.gz")
	if err := artifact.PackDir(source, archive); err != nil {
		t.Fatal(err)
	}
	return archive
}
