package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateVaultSecretParsesFieldAssignments(t *testing.T) {
	tempDir := t.TempDir()
	fieldFile := filepath.Join(tempDir, "password.txt")
	if err := os.WriteFile(fieldFile, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write field file: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/vault/secrets" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var req struct {
			Name      string            `json:"name"`
			Fields    map[string]string `json:"fields"`
			CreatedBy string            `json:"created_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Name != "db-prod" || req.CreatedBy != "tdc" {
			t.Fatalf("unexpected request: %#v", req)
		}
		if req.Fields["DB_URL"] != "mysql://example" || req.Fields["PASSWORD"] != "from-file" || req.Fields["TOKEN"] != "from-stdin" {
			t.Fatalf("fields = %#v", req.Fields)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "db-prod", "revision": 1})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CreateVaultSecret(context.Background(), VaultCreateSecretOptions{
		Profile:    dataProfile(),
		SecretName: "db-prod",
		Fields:     []string{"DB_URL=mysql://example", "PASSWORD=@" + fieldFile, "TOKEN=-"},
		Stdin:      strings.NewReader("from-stdin"),
	})
	if err != nil {
		t.Fatalf("CreateVaultSecret failed: %v", err)
	}
	if result.Status != "created" || result.Secret.Name != "db-prod" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestReadVaultSecretWithDelegatedTokenUsesReadEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/vault/read/db-prod/password" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer delegated-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte("hunter2"))
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).ReadVaultSecret(context.Background(), VaultReadSecretOptions{
		Profile:    dataProfile(),
		SecretName: "db-prod",
		Field:      "password",
		Format:     "raw",
		VaultToken: "delegated-token",
	})
	if err != nil {
		t.Fatalf("ReadVaultSecret failed: %v", err)
	}
	data, ok := result.([]byte)
	if !ok || string(data) != "hunter2" {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestReadVaultSecretEnvFormatNormalizesFieldName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/vault/secrets/db-prod/value/db-password" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write([]byte("hunter2"))
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).ReadVaultSecret(context.Background(), VaultReadSecretOptions{
		Profile:    dataProfile(),
		SecretName: "db-prod",
		Field:      "db-password",
		Format:     "env",
	})
	if err != nil {
		t.Fatalf("ReadVaultSecret failed: %v", err)
	}
	data, ok := result.([]byte)
	if !ok || string(data) != "DB_PASSWORD=hunter2\n" {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestRunWithVaultSecretInjectsEnvAndScrubsTDCCredentials(t *testing.T) {
	if os.Getenv("TDC_TEST_VAULT_CHILD") == "1" {
		for _, key := range []string{"TDC_PRIVATE_KEY", "TDC_PUBLIC_KEY", "TDC_VAULT_TOKEN", "TDC_FS_API_KEY"} {
			if value := os.Getenv(key); value != "" {
				fmt.Fprintf(os.Stderr, "%s leaked as %q\n", key, value)
				os.Exit(2)
			}
		}
		fmt.Fprintf(os.Stdout, "%s|%s|%s", os.Getenv("DB_URL"), os.Getenv("PASSWORD"), os.Getenv("EXISTING"))
		os.Exit(0)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/vault/secrets/db-prod/value" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"DB_URL":   "mysql://example",
			"PASSWORD": "hunter2",
		})
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := testService(t.TempDir(), server.URL).RunWithVaultSecret(context.Background(), VaultRunWithSecretOptions{
		Profile:    dataProfile(),
		SecretPath: "/n/vault/db-prod",
		Command:    []string{os.Args[0], "-test.run=TestRunWithVaultSecretInjectsEnvAndScrubsTDCCredentials"},
		Stdout:     &stdout,
		Stderr:     &stderr,
		Env: []string{
			"TDC_TEST_VAULT_CHILD=1",
			"TDC_PRIVATE_KEY=leak-private",
			"TDC_PUBLIC_KEY=leak-public",
			"TDC_VAULT_TOKEN=leak-token",
			"TDC_FS_API_KEY=leak-fs",
			"EXISTING=kept",
		},
	})
	if err != nil {
		t.Fatalf("RunWithVaultSecret failed: %v stderr=%s", err, stderr.String())
	}
	if got := stdout.String(); got != "mysql://example|hunter2|kept" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestVaultEnvValidationMatchesDrive9Rules(t *testing.T) {
	if _, err := validateVaultEnvFields(map[string]string{"db-url": "mysql://example"}); err == nil || !strings.Contains(err.Error(), "charset") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
	if _, err := validateVaultEnvFields(map[string]string{"DB_URL": "line1\nline2"}); err == nil || !strings.Contains(err.Error(), "control byte") {
		t.Fatalf("expected invalid value error, got %v", err)
	}
	if env, err := validateVaultEnvFields(map[string]string{"DB_URL": "with\ttab"}); err != nil || env["DB_URL"] != "with\ttab" {
		t.Fatalf("expected tab to be accepted, env=%#v err=%v", env, err)
	}
}
