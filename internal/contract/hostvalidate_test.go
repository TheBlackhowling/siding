package contract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateHostPatternRejectsInjection(t *testing.T) {
	bad := `{stack}.{project}.{mode}
  handle {
    respond "pwned" 200
  }
  @z host z`
	if err := ValidateHostPattern("host", bad); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestLoadRejectsMaliciousHostPattern(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := "version: 1\nproject: hello\n" +
		"host: \"{stack}.{project}.{mode}\\n  handle {\\n    respond \\\"pwned\\\" 200\\n  }\\n  @z host z\"\n" +
		"modes:\n  test:\n    compose_files: [docker-compose.yml]\n    ports:\n      web: 18080\n"
	if err := os.WriteFile(filepath.Join(repo, DirName, ConfigFileName), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(repo); err == nil {
		t.Fatal("Load should reject injected host pattern")
	}
}
