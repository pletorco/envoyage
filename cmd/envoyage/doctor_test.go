package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDoctorReportsDebianDockerUpdateRecommended(t *testing.T) {
	env := fakeDoctorEnv{
		paths: map[string]string{
			"docker":     "/usr/bin/docker",
			"dpkg-query": "/usr/bin/dpkg-query",
			"apt-cache":  "/usr/bin/apt-cache",
		},
		outputs: map[string]fakeDoctorCommandResult{
			doctorKey("docker", "version", "--format", "{{.Client.Version}}"): {
				output: "27.5.1",
			},
			doctorKey("docker", "compose", "version", "--short"): {
				output: "v2.32.4",
			},
			doctorKey("dpkg-query", "-W", "-f=${Version}", "docker-ce"): {
				output: "27.5.1-1~ubuntu.24.04~noble",
			},
			doctorKey("apt-cache", "policy", "docker-ce"): {
				output: "Installed: 27.5.1-1~ubuntu.24.04~noble\nCandidate: 28.1.1-1~ubuntu.24.04~noble",
			},
			doctorKey("dpkg-query", "-W", "-f=${Version}", "docker-ce-cli"): {
				output: "27.5.1-1~ubuntu.24.04~noble",
			},
			doctorKey("apt-cache", "policy", "docker-ce-cli"): {
				output: "Installed: 27.5.1-1~ubuntu.24.04~noble\nCandidate: 27.5.1-1~ubuntu.24.04~noble",
			},
			doctorKey("dpkg-query", "-W", "-f=${Version}", "docker-compose-plugin"): {
				output: "2.32.4-1~ubuntu.24.04~noble",
			},
			doctorKey("apt-cache", "policy", "docker-compose-plugin"): {
				output: "Installed: 2.32.4-1~ubuntu.24.04~noble\nCandidate: 2.33.0-1~ubuntu.24.04~noble",
			},
		},
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, env.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Envoyage:",
		"update: no update detected",
		"Docker:",
		"installed: yes",
		"binary: /usr/bin/docker",
		"version: 27.5.1",
		"compose: v2.32.4",
		"package manager: apt/dpkg",
		"docker-ce: 27.5.1-1~ubuntu.24.04~noble -> 28.1.1-1~ubuntu.24.04~noble (update available)",
		"docker-compose-plugin: 2.32.4-1~ubuntu.24.04~noble -> 2.33.0-1~ubuntu.24.04~noble (update available)",
		"action: update recommended",
		"command: sudo apt update && sudo apt install docker-ce docker-compose-plugin",
		"Podman:\n  installed: no",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDoctorReportsEnvoyageUpdateAvailable(t *testing.T) {
	env := fakeDoctorEnv{
		updates: envoyageUpdateDoctorReport{
			Latest:  "0.5.1",
			Action:  "update available",
			Command: "envoyage update",
		},
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, env.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Envoyage:",
		"version: 0.5.0",
		"latest: 0.5.1",
		"update: update available",
		"command: envoyage update",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDoctorReportsRPMRuntimeWithoutUpdates(t *testing.T) {
	env := fakeDoctorEnv{
		paths: map[string]string{
			"podman": "/usr/bin/podman",
			"rpm":    "/usr/bin/rpm",
			"dnf":    "/usr/bin/dnf",
		},
		outputs: map[string]fakeDoctorCommandResult{
			doctorKey("podman", "version", "--format", "{{.Client.Version}}"): {
				output: "5.4.0",
			},
			doctorKey("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", "podman"): {
				output: "5.4.0-2.fc41",
			},
			doctorKey("dnf", "--quiet", "check-update", "podman"): {
				output: "",
			},
			doctorKey("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", "crun"): {
				output: "1.19-1.fc41",
			},
			doctorKey("dnf", "--quiet", "check-update", "crun"): {
				output: "",
			},
		},
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{RuntimeOnly: true}, env.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"scope: runtime",
		"Podman:",
		"version: 5.4.0",
		"package manager: dnf/rpm",
		"podman: 5.4.0-2.fc41 (current candidate)",
		"crun: 1.19-1.fc41 (current candidate)",
		"action: no runtime package update detected",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "command: sudo dnf upgrade") {
		t.Fatalf("output included update command without updates:\n%s", output)
	}
}

func TestDoctorReportsUnableToVerifyWithoutPackageManager(t *testing.T) {
	env := fakeDoctorEnv{
		paths: map[string]string{"docker": "/opt/docker/bin/docker"},
		outputs: map[string]fakeDoctorCommandResult{
			doctorKey("docker", "--version"): {
				output: "Docker version 28.0.0, build abc123",
			},
		},
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, env.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Docker:",
		"binary: /opt/docker/bin/docker",
		"version: Docker version 28.0.0, build abc123",
		"action: unable to verify package updates",
		"reason: supported package manager not detected",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDoctorReportsRPMUpdateRecommendedWhenDNFFindsCandidate(t *testing.T) {
	env := fakeDoctorEnv{
		paths: map[string]string{
			"podman": "/usr/bin/podman",
			"rpm":    "/usr/bin/rpm",
			"dnf":    "/usr/bin/dnf",
		},
		outputs: map[string]fakeDoctorCommandResult{
			doctorKey("podman", "version", "--format", "{{.Client.Version}}"): {
				output: "5.4.0",
			},
			doctorKey("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", "podman"): {
				output: "5.4.0-2.fc41",
			},
			doctorKey("dnf", "--quiet", "check-update", "podman"): {
				output: "podman.x86_64 5.5.0-1.fc41 updates",
				err:    errors.New("exit status 100"),
			},
		},
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, env.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"podman: 5.4.0-2.fc41 -> 5.5.0-1.fc41 (update available)",
		"action: update recommended",
		"command: sudo dnf upgrade podman",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDoctorProjectRecommendsSecretExtraction(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeDoctorFile(t, "compose.yaml", `services:
  app:
    image: app
    environment:
      APP_ENV: production
      DB_PASSWORD: secret-password
      EXISTING: ${EXISTING}
`)

	env := fakeDoctorEnv{}
	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, env.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Project Env Check:",
		"compose: compose.yaml",
		".env: missing",
		".secrets.env: missing",
		".env.age: missing",
		"fixed env keys: APP_ENV",
		"fixed secret keys: DB_PASSWORD",
		"action: secret extraction recommended",
		"command: envoyage env extract --write --secrets && envoyage encrypt",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "secret-password") || strings.Contains(output, "production") {
		t.Fatalf("doctor output leaked env values:\n%s", output)
	}
}

func TestDoctorProjectReportsNoExtractionNeeded(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeDoctorFile(t, "compose.yaml", `services:
  app:
    image: app
    environment:
      APP_ENV: ${APP_ENV}
`)
	writeDoctorFile(t, ".env", "APP_ENV=production\n")
	writeDoctorFile(t, ".secrets.env", "DB_PASSWORD=secret-password\n")
	writeDoctorFile(t, ".env.age", "age payload\n")
	now := time.Now()
	if err := os.Chtimes(".env.age", now.Add(time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatalf("Chtimes(.env.age) error = %v", err)
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, fakeDoctorEnv{}.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		".env: found",
		".secrets.env: found",
		".env.age: found",
		"action: no env extraction needed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDoctorProjectRecommendsMovingSecretsFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeDoctorFile(t, "compose.yaml", "services: {}\n")
	writeDoctorFile(t, ".env", "API_TOKEN=secret-token\nAPP_ENV=test\n")

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, fakeDoctorEnv{}.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		".env secret-looking keys: API_TOKEN",
		"action: move secrets from .env recommended",
		"command: move secret-looking keys to .secrets.env && envoyage encrypt",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "secret-token") {
		t.Fatalf("doctor output leaked env values:\n%s", output)
	}
}

func TestDoctorProjectRecommendsEncryptWhenSecretsAreNewer(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeDoctorFile(t, "compose.yaml", "services: {}\n")
	writeDoctorFile(t, ".secrets.env", "DB_PASSWORD=secret-password\n")
	writeDoctorFile(t, ".env.age", "old age payload\n")
	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(".env.age", oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(.env.age) error = %v", err)
	}
	if err := os.Chtimes(".secrets.env", newTime, newTime); err != nil {
		t.Fatalf("Chtimes(.secrets.env) error = %v", err)
	}

	var out bytes.Buffer
	err := writeDoctorReport(context.Background(), doctorOptions{}, fakeDoctorEnv{}.environment(), &out)
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"action: encrypt recommended",
		"command: envoyage encrypt --force",
		"reason: .secrets.env is newer than .env.age",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunDoctorRejectsUnexpectedArgs(t *testing.T) {
	err := runDoctor([]string{"extra"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runDoctor() error = nil, want unexpected argument error")
	}
	if !strings.Contains(err.Error(), "doctor does not accept arguments") {
		t.Fatalf("error = %q, want unexpected argument error", err.Error())
	}
}

func TestDoctorScope(t *testing.T) {
	if got := doctorScope(doctorOptions{}); got != "all" {
		t.Fatalf("doctorScope(default) = %q, want all", got)
	}
	if got := doctorScope(doctorOptions{RuntimeOnly: true}); got != "runtime" {
		t.Fatalf("doctorScope(runtime) = %q, want runtime", got)
	}
}

type fakeDoctorEnv struct {
	paths   map[string]string
	outputs map[string]fakeDoctorCommandResult
	updates envoyageUpdateDoctorReport
}

type fakeDoctorCommandResult struct {
	output string
	err    error
}

func (env fakeDoctorEnv) environment() doctorEnvironment {
	return doctorEnvironment{
		LookPath: func(file string) (string, error) {
			if path, ok := env.paths[file]; ok {
				return path, nil
			}
			return "", execNotFound(file)
		},
		Run: func(ctx context.Context, name string, args ...string) (string, error) {
			result, ok := env.outputs[doctorKey(name, args...)]
			if ok {
				return result.output, result.err
			}
			return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		},
		Update: func(ctx context.Context) envoyageUpdateDoctorReport {
			if env.updates.Action != "" {
				return env.updates
			}
			return envoyageUpdateDoctorReport{
				Latest: "0.5.0",
				Action: "no update detected",
			}
		},
	}
}

func doctorKey(name string, args ...string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}

func execNotFound(name string) error {
	return fmt.Errorf("%s not found", name)
}

func writeDoctorFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
