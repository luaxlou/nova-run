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
	"github.com/luaxlou/glow-ops/internal/deploy"
)

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
