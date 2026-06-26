package envops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDryRunSplitsFixedEnvironmentValues(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", `services:
  app:
    image: app
    environment:
      APP_ENV: production
      DB_PASSWORD: secret-password
      EXISTING: ${EXISTING}
  worker:
    image: worker
    environment:
      - API_TOKEN=secret-token
      - LOG_LEVEL=info
      - PASSTHROUGH
      - ALREADY=${ALREADY}
`)

	result, err := Extract(ExtractOptions{Secrets: true})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	assertStrings(t, result.EnvKeys, []string{"APP_ENV", "LOG_LEVEL"})
	assertStrings(t, result.SecretKeys, []string{"API_TOKEN", "DB_PASSWORD"})
	if len(result.Updates) != 4 {
		t.Fatalf("updates = %#v, want 4", result.Updates)
	}
	if _, err := os.Stat(".env"); !os.IsNotExist(err) {
		t.Fatalf(".env should not be written during dry-run: %v", err)
	}

	composeData := readFile(t, "compose.yaml")
	if strings.Contains(composeData, "${APP_ENV}") {
		t.Fatalf("compose file changed during dry-run:\n%s", composeData)
	}
}

func TestExtractWriteUpdatesComposeAndAppendsEnvFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", `services:
  app:
    environment:
      APP_ENV: production
      DB_PASSWORD: secret-password
`)
	writeFile(t, ".env", "EXISTING=kept\n")

	result, err := Extract(ExtractOptions{Write: true, Secrets: true})
	if err != nil {
		t.Fatalf("Extract(write) error = %v", err)
	}
	assertStrings(t, result.EnvKeys, []string{"APP_ENV"})
	assertStrings(t, result.SecretKeys, []string{"DB_PASSWORD"})

	envData := readFile(t, ".env")
	if !strings.Contains(envData, "EXISTING=kept\n") || !strings.Contains(envData, "APP_ENV=production\n") {
		t.Fatalf(".env content = %q, want existing and extracted key", envData)
	}
	secretData := readFile(t, ".secrets.env")
	if strings.Contains(secretData, "secret-password") && !strings.Contains(secretData, "DB_PASSWORD=secret-password\n") {
		t.Fatalf(".secrets.env content = %q, want secret key", secretData)
	}

	composeData := readFile(t, "compose.yaml")
	if !strings.Contains(composeData, "APP_ENV: ${APP_ENV}") {
		t.Fatalf("compose content = %q, want APP_ENV variable", composeData)
	}
	if !strings.Contains(composeData, "DB_PASSWORD: ${DB_PASSWORD}") {
		t.Fatalf("compose content = %q, want DB_PASSWORD variable", composeData)
	}
}

func TestExtractRejectsConflictingValuesWithoutPrintingValues(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", `services:
  app:
    environment:
      SHARED: one
  worker:
    environment:
      SHARED: two
`)

	_, err := Extract(ExtractOptions{Secrets: true})
	if err == nil {
		t.Fatal("Extract(conflict) error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "SHARED") {
		t.Fatalf("error = %q, want key name", err.Error())
	}
	if strings.Contains(err.Error(), "one") || strings.Contains(err.Error(), "two") {
		t.Fatalf("error leaked conflicting values: %q", err.Error())
	}
}

func TestExtractWriteRejectsExistingEnvConflict(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", `services:
  app:
    environment:
      APP_ENV: production
`)
	writeFile(t, ".env", "APP_ENV=staging\n")

	_, err := Extract(ExtractOptions{Write: true, Secrets: true})
	if err == nil {
		t.Fatal("Extract(existing conflict) error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("error = %q, want key name", err.Error())
	}
	if strings.Contains(err.Error(), "production") || strings.Contains(err.Error(), "staging") {
		t.Fatalf("error leaked values: %q", err.Error())
	}
}

func TestInlineWritesRenderedComposeWithoutChangingSource(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", `services:
  app:
    environment:
      APP_ENV: ${APP_ENV}
      DB_PASSWORD: ${DB_PASSWORD}
      KEEP: fixed
  worker:
    environment:
      - API_TOKEN=${API_TOKEN}
      - LOG_LEVEL=${LOG_LEVEL}
`)
	writeFile(t, ".env", "APP_ENV=production\nLOG_LEVEL=info\n")
	writeFile(t, ".secrets.env", "DB_PASSWORD=secret-password\nAPI_TOKEN=secret-token\n")

	result, err := Inline(InlineOptions{OutputPath: "compose.inline.yaml"})
	if err != nil {
		t.Fatalf("Inline() error = %v", err)
	}
	if len(result.Updates) != 4 {
		t.Fatalf("updates = %#v, want 4", result.Updates)
	}

	source := readFile(t, "compose.yaml")
	if strings.Contains(source, "secret-password") {
		t.Fatalf("source compose changed or leaked secret:\n%s", source)
	}
	rendered := readFile(t, "compose.inline.yaml")
	for _, want := range []string{"APP_ENV: production", "DB_PASSWORD: secret-password", "API_TOKEN=secret-token", "LOG_LEVEL=info"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered compose = %q, want %q", rendered, want)
		}
	}
}

func TestInlineRequiresSeparateOutputAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", `services:
  app:
    environment:
      APP_ENV: ${APP_ENV}
`)
	writeFile(t, ".env", "APP_ENV=production\n")
	writeFile(t, "compose.inline.yaml", "existing\n")

	_, err := Inline(InlineOptions{OutputPath: "compose.yaml"})
	if err == nil {
		t.Fatal("Inline(same output) error = nil, want error")
	}

	_, err = Inline(InlineOptions{OutputPath: "compose.inline.yaml"})
	if err == nil {
		t.Fatal("Inline(overwrite) error = nil, want error")
	}
	if _, err := Inline(InlineOptions{OutputPath: "compose.inline.yaml", Force: true}); err != nil {
		t.Fatalf("Inline(force) error = %v", err)
	}
}

func TestResolveComposePathRejectsAmbiguousDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "compose.yaml", "services: {}\n")
	writeFile(t, "docker-compose.yml", "services: {}\n")

	_, err := Extract(ExtractOptions{})
	if err == nil {
		t.Fatal("Extract(ambiguous compose) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "multiple compose files") {
		t.Fatalf("error = %q, want multiple compose files", err.Error())
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
