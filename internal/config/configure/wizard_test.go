package configure

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/config"
)

func TestRunWritesProfileAndDoesNotPrintSecret(t *testing.T) {
	home := t.TempDir()
	input := strings.NewReader("aws\nus-east-1\npublic-key\nprivate-key\n")
	var output bytes.Buffer

	err := Run(context.Background(), Options{
		Profile: "stage",
		HomeDir: home,
		In:      input,
		Out:     &output,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if strings.Contains(output.String(), "private-key") {
		t.Fatalf("configure output leaked private key:\n%s", output.String())
	}

	profile, err := config.Load(context.Background(), config.LoadOptions{
		Profile:         "stage",
		ProfileExplicit: true,
		HomeDir:         home,
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.CloudProvider != "aws" || profile.RegionCode != "us-east-1" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if profile.TDCPrivateKey != "private-key" {
		t.Fatal("private key was not stored")
	}
}

func TestRunRejectsUnsupportedProviderRegion(t *testing.T) {
	input := strings.NewReader("alibaba_cloud\nus-east-1\npublic-key\nprivate-key\n")

	err := Run(context.Background(), Options{
		HomeDir: t.TempDir(),
		In:      input,
		Out:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected invalid provider/region to fail")
	}
}

func TestRunNonInteractiveUsesEnvironment(t *testing.T) {
	home := t.TempDir()
	var output bytes.Buffer

	err := Run(context.Background(), Options{
		Profile:        "ci",
		HomeDir:        home,
		NonInteractive: true,
		Env: map[string]string{
			"TDC_CLOUD_PROVIDER": "aws",
			"TDC_REGION_CODE":    "us-east-1",
			"TDC_PUBLIC_KEY":     "env-public",
			"TDC_PRIVATE_KEY":    "env-private",
		},
		Out: &output,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.Contains(output.String(), "env-private") {
		t.Fatalf("configure output leaked private key:\n%s", output.String())
	}

	profile, err := config.Load(context.Background(), config.LoadOptions{
		Profile:         "ci",
		ProfileExplicit: true,
		HomeDir:         home,
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.CloudProvider != "aws" || profile.RegionCode != "us-east-1" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if profile.TDCPublicKey != "env-public" || profile.TDCPrivateKey != "env-private" {
		t.Fatalf("env credentials not stored: %#v", profile)
	}
}

func TestRunNonInteractiveRequiresMissingValues(t *testing.T) {
	err := Run(context.Background(), Options{
		HomeDir:        t.TempDir(),
		NonInteractive: true,
		Env: map[string]string{
			"TDC_CLOUD_PROVIDER": "aws",
			"TDC_REGION_CODE":    "us-east-1",
			"TDC_PUBLIC_KEY":     "env-public",
		},
		Out: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected missing private key to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "non-interactive configure") {
		t.Fatalf("unexpected error: %q", got)
	}
}
