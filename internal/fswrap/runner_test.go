package fswrap

import (
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/api/endpoints"
	"github.com/tidbcloud/tdc/internal/config"
)

func TestDrive9EnvironmentSanitizesTDCSecrets(t *testing.T) {
	t.Setenv("TDC_PUBLIC_KEY", "ambient-public")
	t.Setenv("TDC_PRIVATE_KEY", "ambient-private")
	t.Setenv("TDC_FS_TOKEN", "ambient-fs-token")
	t.Setenv("TDC_VAULT_TOKEN", "ambient-vault-token")
	t.Setenv("DRIVE9_API_KEY", "ambient-drive9-token")
	profile := &config.Profile{
		Name:                  "default",
		FSResourceName:        "workspace",
		FSPlacementRegionCode: "aws-us-east-1",
		FSCloudProvider:       "aws",
		FSRegionCode:          "us-east-1",
		FSAPIKey:              "selected-fs-token",
		TDCPublicKey:          "selected-public",
		TDCPrivateKey:         "selected-private",
	}
	runner := Runner{Resolver: endpoints.Resolver{FSBaseURLs: map[endpoints.ProviderRegion]string{
		{Provider: "aws", Region: "us-east-1"}: "https://fs.test",
	}}}

	env, err := runner.drive9Env(t.TempDir(), RunOptions{
		Profile:         profile,
		IncludeFSAPIKey: true,
		VaultToken:      "selected-vault-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	values := environmentMap(env)
	for _, key := range []string{"TDC_PUBLIC_KEY", "TDC_PRIVATE_KEY", "TDC_FS_TOKEN", "TDC_VAULT_TOKEN"} {
		if _, ok := values[key]; ok {
			t.Fatalf("%s leaked into companion environment", key)
		}
	}
	if values["DRIVE9_API_KEY"] != "selected-fs-token" || values["DRIVE9_VAULT_TOKEN"] != "selected-vault-token" {
		t.Fatalf("selected credentials were not mapped correctly: %#v", values)
	}
	if _, ok := values["DRIVE9_PUBLIC_KEY"]; ok {
		t.Fatalf("data-plane environment included TiDB Cloud public key")
	}
	if _, ok := values["DRIVE9_PRIVATE_KEY"]; ok {
		t.Fatalf("data-plane environment included TiDB Cloud private key")
	}

	env, err = runner.drive9Env(t.TempDir(), RunOptions{Profile: profile, IncludeTDCKeys: true})
	if err != nil {
		t.Fatal(err)
	}
	values = environmentMap(env)
	if values["DRIVE9_PUBLIC_KEY"] != "selected-public" || values["DRIVE9_PRIVATE_KEY"] != "selected-private" {
		t.Fatalf("create/delete credentials were not mapped correctly: %#v", values)
	}
	if _, ok := values["DRIVE9_API_KEY"]; ok {
		t.Fatalf("provision environment unexpectedly included FS token")
	}
}

func environmentMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}
