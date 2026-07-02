package endpoints

import (
	"strings"
	"testing"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/config/region"
)

func TestResolveStarterEndpoint(t *testing.T) {
	resolver := NewResolver()
	endpoint, err := resolver.ResolveStarter(region.ProviderAWS, "us-east-1")
	if err != nil {
		t.Fatalf("ResolveStarter failed: %v", err)
	}
	if endpoint.BaseURL != DefaultStarterBaseURL {
		t.Fatalf("unexpected base URL %q", endpoint.BaseURL)
	}
	if endpoint.RegionName != "regions/aws-us-east-1" {
		t.Fatalf("unexpected region name %q", endpoint.RegionName)
	}
}

func TestResolveStarterAlibabaRegion(t *testing.T) {
	endpoint, err := NewResolver().ResolveStarter(region.ProviderAlibabaCloud, "ap-southeast-1")
	if err != nil {
		t.Fatalf("ResolveStarter failed: %v", err)
	}
	if endpoint.APIProvider != "alicloud" {
		t.Fatalf("unexpected API provider %q", endpoint.APIProvider)
	}
	if endpoint.RegionName != "regions/alicloud-ap-southeast-1" {
		t.Fatalf("unexpected region name %q", endpoint.RegionName)
	}
}

func TestResolveIAMEndpoint(t *testing.T) {
	endpoint, err := NewResolver().ResolveIAM()
	if err != nil {
		t.Fatalf("ResolveIAM failed: %v", err)
	}
	if endpoint.BaseURL != DefaultIAMBaseURL {
		t.Fatalf("unexpected IAM base URL %q", endpoint.BaseURL)
	}
}

func TestResolveFSWithTestOverrides(t *testing.T) {
	resolver := NewResolver()
	resolver.FSBaseURLs = map[ProviderRegion]string{
		{Provider: region.ProviderAWS, Region: "us-east-1"}:               "https://fs.aws.test",
		{Provider: region.ProviderAlibabaCloud, Region: "ap-southeast-1"}: "https://fs.alibaba.test",
	}

	aws, err := resolver.ResolveFS(region.ProviderAWS, "us-east-1")
	if err != nil {
		t.Fatalf("ResolveFS aws failed: %v", err)
	}
	alibaba, err := resolver.ResolveFS(region.ProviderAlibabaCloud, "ap-southeast-1")
	if err != nil {
		t.Fatalf("ResolveFS alibaba failed: %v", err)
	}
	if aws.BaseURL != "https://fs.aws.test" || alibaba.BaseURL != "https://fs.alibaba.test" {
		t.Fatalf("unexpected fs endpoints: %#v %#v", aws, alibaba)
	}
}

func TestResolveUnsupportedRegionShowsValidRegions(t *testing.T) {
	_, err := NewResolver().ResolveStarter(region.ProviderAlibabaCloud, "us-east-1")
	if err == nil {
		t.Fatal("expected invalid region to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
	message := apperr.MessageFor(err)
	if !strings.Contains(message, "ap-southeast-1") {
		t.Fatalf("expected valid region list in message, got %q", message)
	}
}
