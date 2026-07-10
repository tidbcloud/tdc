package fscred

import (
	"fmt"
	"strings"

	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config"
	"github.com/tidbcloud/tdc/internal/config/region"
	"github.com/tidbcloud/tdc/internal/config/store"
)

type Resource struct {
	Name          string `json:"file_system_name"`
	TenantID      string `json:"tenant_id,omitempty"`
	CloudProvider string `json:"cloud_provider,omitempty"`
	RegionCode    string `json:"region_code,omitempty"`
	HasAPIKey     bool   `json:"has_api_key"`
}

func FromProfile(profile *config.Profile) Resource {
	if profile == nil {
		return Resource{}
	}
	return Resource{
		Name:          profile.FSResourceName,
		TenantID:      profile.FSTenantID,
		CloudProvider: profile.FSCloudProvider,
		RegionCode:    profile.FSPlacementRegionCode,
		HasAPIKey:     strings.TrimSpace(profile.FSAPIKey) != "",
	}
}

func Store(homeDir string, profile *config.Profile, resourceName, tenantID, cloudProvider, regionCode, apiKey string) error {
	if profile == nil {
		return apperr.New("fs.missing_profile", "config", 2, "active profile is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return apperr.New("fs.missing_api_key", "api", 1, "tdc fs provision response did not include an api_key")
	}
	if strings.TrimSpace(tenantID) == "" {
		return apperr.New("fs.missing_tenant_id", "api", 1, "tdc fs provision response did not include a tenant_id")
	}
	canonicalCode, resolvedProvider, err := canonicalPlacement(profile, cloudProvider, regionCode)
	if err != nil {
		return err
	}
	if err := store.WriteProfile(homeDir, profile.Name, store.ConfigProfile{
		FSResourceName:  strings.TrimSpace(resourceName),
		FSTenantID:      strings.TrimSpace(tenantID),
		FSCloudProvider: resolvedProvider,
		FSRegionCode:    canonicalCode,
	}, store.CredentialsProfile{
		FSAPIKey: strings.TrimSpace(apiKey),
	}); err != nil {
		return fmt.Errorf("store tdc fs credentials: %w", err)
	}
	return nil
}

func canonicalPlacement(profile *config.Profile, cloudProvider, regionCode string) (string, string, error) {
	cloudProvider = normalizeProvider(cloudProvider)
	regionCode = strings.TrimSpace(regionCode)
	if regionCode == "" && profile != nil {
		regionCode = profile.PlacementRegionCode
	}
	if parsed, err := region.ParsePlacementCode(regionCode); err == nil {
		return parsed.Code, parsed.Provider, nil
	}
	if cloudProvider == "" && profile != nil {
		cloudProvider = profile.CloudProvider
	}
	if regionCode == "" && profile != nil {
		regionCode = profile.RegionCode
	}
	canonicalCode, err := region.CanonicalCode(cloudProvider, regionCode)
	if err != nil {
		return "", "", apperr.Wrap("fs.invalid_resource_region", "config", 2, err.Error(), err)
	}
	return canonicalCode, cloudProvider, nil
}

func normalizeProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case "alicloud", "ali", region.ProviderAlibabaCloud:
		return region.ProviderAlibabaCloud
	case region.ProviderAWS:
		return region.ProviderAWS
	default:
		return strings.TrimSpace(provider)
	}
}

func Clear(homeDir string, profile *config.Profile) error {
	if profile == nil {
		return apperr.New("fs.missing_profile", "config", 2, "active profile is required")
	}
	if err := store.ClearFSResource(homeDir, profile.Name); err != nil {
		return fmt.Errorf("clear tdc fs credentials: %w", err)
	}
	return nil
}
