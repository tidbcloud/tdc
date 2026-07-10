package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config/store"
)

func TestLoadExplicitProfile(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "stage", "aws-us-west-2", "stage-public", "stage-private")

	profile, err := Load(context.Background(), LoadOptions{
		Profile:         "stage",
		ProfileExplicit: true,
		HomeDir:         home,
		Env: map[string]string{
			"TDC_REGION_CODE": "ali-ap-southeast-1",
			"TDC_PUBLIC_KEY":  "env-public",
			"TDC_PRIVATE_KEY": "env-private",
		},
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.Name != "stage" || profile.Source != "profile" {
		t.Fatalf("unexpected profile identity: %#v", profile)
	}
	if profile.CloudProvider != "aws" || profile.RegionCode != "us-west-2" {
		t.Fatalf("explicit profile did not win over env: %#v", profile)
	}
}

func TestLoadEnvironmentFallback(t *testing.T) {
	profile, err := Load(context.Background(), LoadOptions{
		HomeDir: t.TempDir(),
		Env: map[string]string{
			"TDC_REGION_CODE": "aws-us-east-1",
			"TDC_PUBLIC_KEY":  "env-public",
			"TDC_PRIVATE_KEY": "env-private",
		},
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.Source != "env" {
		t.Fatalf("expected env source, got %#v", profile)
	}
	if profile.TDCPrivateKey != "env-private" {
		t.Fatalf("env private key not loaded")
	}
}

func TestLoadDefaultProfileFallback(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, DefaultProfile, "ali-ap-southeast-1", "public", "private")

	profile, err := Load(context.Background(), LoadOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.Name != DefaultProfile || profile.CloudProvider != "alibaba_cloud" {
		t.Fatalf("default profile not loaded: %#v", profile)
	}
}

func TestLoadRejectsInvalidRegionCombination(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, DefaultProfile, "ali-us-east-1", "public", "private")

	_, err := Load(context.Background(), LoadOptions{HomeDir: home})
	if err == nil {
		t.Fatal("expected invalid region to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "unsupported region") {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestLoadMissingCredentialsSuggestsConfigure(t *testing.T) {
	home := t.TempDir()
	if err := store.WriteProfile(home, DefaultProfile, store.ConfigProfile{
		RegionCode: "aws-us-east-1",
	}, store.CredentialsProfile{}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(context.Background(), LoadOptions{HomeDir: home})
	if err == nil {
		t.Fatal("expected missing credentials to fail")
	}
	message := apperr.MessageFor(err)
	if !strings.Contains(message, "tdc configure") || !strings.Contains(message, "~/.tdc/credentials") {
		t.Fatalf("missing credentials error did not include guidance: %q", message)
	}
}

func TestLoadRejectsServerURLKeys(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, store.TDCDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ConfigPath(home), []byte(`
[default]
region_code = "aws-us-east-1"
api_endpoint = "https://example.invalid"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.CredentialsPath(home), []byte(`
[default]
tdc_public_key = "public"
tdc_private_key = "private"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(context.Background(), LoadOptions{HomeDir: home})
	if err == nil {
		t.Fatal("expected server URL key to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "api_endpoint") {
		t.Fatalf("expected api_endpoint in error, got %q", got)
	}
}

func TestLoadReadsFSCredentials(t *testing.T) {
	home := t.TempDir()
	err := store.WriteProfile(home, DefaultProfile, store.ConfigProfile{
		RegionCode:      "aws-us-east-1",
		FSResourceName:  "workspace",
		FSTenantID:      "tenant",
		FSCloudProvider: "aws",
		FSRegionCode:    "aws-us-east-1",
	}, store.CredentialsProfile{
		TDCPublicKey:  "public",
		TDCPrivateKey: "private",
		FSAPIKey:      "fs-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	profile, err := Load(context.Background(), LoadOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.FSAPIKey != "fs-secret" || profile.FSResourceName != "workspace" {
		t.Fatalf("fs credentials not loaded: %#v", profile)
	}
}

func writeProfile(t *testing.T, home, name, regionCode, publicKey, privateKey string) {
	t.Helper()
	err := store.WriteProfile(home, name, store.ConfigProfile{
		RegionCode: regionCode,
	}, store.CredentialsProfile{
		TDCPublicKey:  publicKey,
		TDCPrivateKey: privateKey,
	})
	if err != nil {
		t.Fatal(err)
	}
}
