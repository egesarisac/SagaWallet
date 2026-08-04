package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestApplyFileSecrets(t *testing.T) {
	tempDir := t.TempDir()
	jwtPath := filepath.Join(tempDir, "jwt")
	if err := os.WriteFile(jwtPath, []byte("secret-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(tempDir, "database-url")
	if err := os.WriteFile(databasePath, []byte("postgres://from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWT_SECRET_FILE", jwtPath)
	t.Setenv("DATABASE_URL_FILE", databasePath)

	v := viper.New()
	if err := applyFileSecrets(v); err != nil {
		t.Fatal(err)
	}
	if got := v.GetString("JWT_SECRET"); got != "secret-from-file" {
		t.Fatalf("expected file secret, got %q", got)
	}
	if got := v.GetString("DB_DSN"); got != "postgres://from-file" {
		t.Fatalf("expected database file secret, got %q", got)
	}
}

func TestApplyFileSecretsRejectsUnreadablePath(t *testing.T) {
	t.Setenv("JWT_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))

	if err := applyFileSecrets(viper.New()); err == nil {
		t.Fatal("expected missing secret file to fail")
	}
}
