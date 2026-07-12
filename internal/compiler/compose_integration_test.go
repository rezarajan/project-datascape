//go:build integration

package compiler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"datascape.dev/platformctl/internal/artifact"
	"datascape.dev/platformctl/internal/domain"
)

func TestIntegrationComposeConfigValidates(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	result := CompileDocuments(context.Background(), sampleDocs(), Options{CompilerVersion: "integration"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	dir := t.TempDir()
	if err := artifact.WriteFiles(context.Background(), dir, result.Files); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("docker", "compose", "-f", filepath.Join(dir, "compose.yaml"), "config", "-q")
	cmd.Env = append(os.Environ(), "DATASCAPE_SOURCE_PASSWORD=test", "MINIO_ROOT_PASSWORD=test")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, output)
	}
}
