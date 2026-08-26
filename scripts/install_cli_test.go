package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallCLIInstallsDownloadedBinary(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	output, err := env.run()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, output)
	}
	if got := env.installedContent(t); got != "nova-v1" {
		t.Fatalf("installed content=%q", got)
	}
}

func TestInstallCLISkipsIdenticalExecutable(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	wantTime := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	env.writeInstalled(t, "nova-v1", 0o755)
	if err := os.Chtimes(env.installedPath, wantTime, wantTime); err != nil {
		t.Fatal(err)
	}

	output, err := env.run()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, output)
	}
	info, err := os.Stat(env.installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(wantTime) {
		t.Fatalf("identical binary was overwritten: modtime=%s", info.ModTime())
	}
	if !strings.Contains(output, "already the latest") || !strings.Contains(output, "--force") {
		t.Fatalf("output=%q", output)
	}
}

func TestInstallCLIForceOverwritesIdenticalExecutable(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	oldTime := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	env.writeInstalled(t, "nova-v1", 0o755)
	if err := os.Chtimes(env.installedPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	output, err := env.run("--force")
	if err != nil {
		t.Fatalf("force install failed: %v\n%s", err, output)
	}
	info, err := os.Stat(env.installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Equal(oldTime) {
		t.Fatal("--force did not overwrite the identical binary")
	}
}

func TestInstallCLIRejectsUnknownArguments(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	output, err := env.run("--unknown")
	if err == nil {
		t.Fatalf("unknown argument unexpectedly succeeded: %s", output)
	}
}

type installerEnv struct {
	t              *testing.T
	scriptPath     string
	installDir     string
	installedPath  string
	downloadPath   string
	fakeCommandDir string
}

func newInstallerEnv(t *testing.T, downloaded string) *installerEnv {
	t.Helper()
	root := t.TempDir()
	scriptPath, err := filepath.Abs("install-cli.sh")
	if err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(root, "bin")
	fakeCommandDir := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(fakeCommandDir, 0o755); err != nil {
		t.Fatal(err)
	}
	downloadPath := filepath.Join(root, "downloaded-nova")
	if err := os.WriteFile(downloadPath, []byte(downloaded), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeCurl := `#!/bin/sh
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    output="$1"
  fi
  shift
done
cp "$NOVA_TEST_DOWNLOAD" "$output"
`
	if err := os.WriteFile(filepath.Join(fakeCommandDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}
	return &installerEnv{
		t: t, scriptPath: scriptPath, installDir: installDir,
		installedPath: filepath.Join(installDir, "nova"),
		downloadPath:  downloadPath, fakeCommandDir: fakeCommandDir,
	}
}

func (e *installerEnv) run(args ...string) (string, error) {
	e.t.Helper()
	cmd := exec.Command("bash", append([]string{e.scriptPath}, args...)...)
	cmd.Env = append(os.Environ(),
		"NOVA_INSTALL_DIR="+e.installDir,
		"NOVA_TEST_DOWNLOAD="+e.downloadPath,
		"PATH="+e.fakeCommandDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (e *installerEnv) writeInstalled(t *testing.T, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(e.installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.installedPath, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func (e *installerEnv) installedContent(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(e.installedPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
