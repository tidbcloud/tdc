package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/Icemap/tdc/internal/config"
)

const defaultLiveProfile = "live-e2e"

func TestLiveProfileConfigured(t *testing.T) {
	if os.Getenv("TDC_LIVE") != "1" {
		t.Skip("TDC_LIVE=1 is required; run make live-e2e")
	}

	bin := tdcBinary(t)
	version := runTDC(t, bin, "--version")
	version.wantExitCode(0)
	version.wantStdoutContains("tdc ")

	profileName := os.Getenv("TDC_PROFILE")
	if profileName == "" {
		profileName = defaultLiveProfile
	}
	if profileName != defaultLiveProfile {
		t.Fatalf("live e2e must use profile %q, got %q", defaultLiveProfile, profileName)
	}

	profile, err := config.Load(context.Background(), config.LoadOptions{
		Profile:         profileName,
		ProfileExplicit: true,
	})
	if err != nil {
		t.Fatalf("load live e2e profile %q: %v\nconfigure it with: bin/tdc configure --profile %s", profileName, err, profileName)
	}
	if profile.CloudProvider == "" || profile.RegionCode == "" || profile.TDCPublicKey == "" || profile.TDCPrivateKey == "" {
		t.Fatalf("live e2e profile %q is incomplete", profileName)
	}
}
