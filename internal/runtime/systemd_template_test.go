package runtime

import (
	"strings"
	"testing"
)

func TestAppServiceTemplateRunsDeployedArtifact(t *testing.T) {
	content := appServiceTemplate("/var/lib/nova/apps")

	if !strings.Contains(content, "WorkingDirectory=/var/lib/nova/apps/%i") {
		t.Fatalf("template missing app working directory: %s", content)
	}
	if !strings.Contains(content, "ExecStart=/var/lib/nova/apps/%i/run") {
		t.Fatalf("template missing app run command: %s", content)
	}
}
