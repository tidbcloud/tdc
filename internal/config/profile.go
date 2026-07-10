package config

import (
	"context"
	"fmt"
	"os"

	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config/region"
	"github.com/tidbcloud/tdc/internal/config/store"
)

const DefaultProfile = "default"

type LoadOptions struct {
	Profile         string
	ProfileExplicit bool
	HomeDir         string
	Env             map[string]string
}

type Profile struct {
	Name                  string
	Source                string
	PlacementRegionCode   string
	CloudProvider         string
	RegionCode            string
	TDCPublicKey          string
	TDCPrivateKey         string
	FSResourceName        string
	FSTenantID            string
	FSPlacementRegionCode string
	FSCloudProvider       string
	FSRegionCode          string
	FSAPIKey              string
}

func Load(ctx context.Context, opts LoadOptions) (*Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, apperr.Wrap("config.home_dir", "config", 1, "cannot determine home directory", err)
		}
		opts.HomeDir = home
	}

	profileName := opts.Profile
	if profileName == "" {
		profileName = DefaultProfile
	}

	if !opts.ProfileExplicit && envMode(opts.Env) {
		return loadFromEnv(opts.Env)
	}

	return loadFromFiles(opts.HomeDir, profileName)
}

func loadFromEnv(env map[string]string) (*Profile, error) {
	missing := firstMissingEnv(env, "TDC_REGION_CODE", "TDC_PUBLIC_KEY", "TDC_PRIVATE_KEY")
	if missing != "" {
		return nil, apperr.New(
			"config.env_missing",
			"config",
			2,
			fmt.Sprintf("%s is required when using TDC_* environment credentials", missing),
		)
	}

	placement, err := parsePlacement(envValue(env, "TDC_REGION_CODE"))
	if err != nil {
		return nil, err
	}
	profile := &Profile{
		Name:                "env",
		Source:              "env",
		PlacementRegionCode: placement.Code,
		CloudProvider:       placement.Provider,
		RegionCode:          placement.NativeCode,
		TDCPublicKey:        envValue(env, "TDC_PUBLIC_KEY"),
		TDCPrivateKey:       envValue(env, "TDC_PRIVATE_KEY"),
	}
	return profile, nil
}

func loadFromFiles(homeDir, profileName string) (*Profile, error) {
	configDoc, err := store.ReadConfig(homeDir)
	if err != nil {
		return nil, apperr.Wrap("config.read_config", "config", 1, err.Error(), err)
	}
	credentialsDoc, err := store.ReadCredentials(homeDir)
	if err != nil {
		return nil, apperr.Wrap("config.read_credentials", "config", 1, err.Error(), err)
	}

	cfg, ok := configDoc[profileName]
	if !ok {
		return nil, apperr.New(
			"config.profile_not_found",
			"config",
			2,
			fmt.Sprintf("profile %q not found in %s; run tdc configure --profile %s or write ~/.tdc/config", profileName, store.ConfigPath(homeDir), profileName),
		)
	}
	creds, ok := credentialsDoc[profileName]
	if !ok {
		return nil, missingCredential(profileName, store.CredentialsPath(homeDir), "tdc_public_key")
	}
	if cfg.RegionCode == "" {
		return nil, missingConfig(profileName, store.ConfigPath(homeDir), "region_code")
	}
	if creds.TDCPublicKey == "" {
		return nil, missingCredential(profileName, store.CredentialsPath(homeDir), "tdc_public_key")
	}
	if creds.TDCPrivateKey == "" {
		return nil, missingCredential(profileName, store.CredentialsPath(homeDir), "tdc_private_key")
	}

	placement, err := parsePlacement(cfg.RegionCode)
	if err != nil {
		return nil, err
	}
	var fsPlacement region.Placement
	if cfg.FSRegionCode != "" {
		fsPlacement, err = parsePlacement(cfg.FSRegionCode)
		if err != nil {
			return nil, err
		}
	}

	return &Profile{
		Name:                  profileName,
		Source:                "profile",
		PlacementRegionCode:   placement.Code,
		CloudProvider:         placement.Provider,
		RegionCode:            placement.NativeCode,
		TDCPublicKey:          creds.TDCPublicKey,
		TDCPrivateKey:         creds.TDCPrivateKey,
		FSResourceName:        cfg.FSResourceName,
		FSTenantID:            cfg.FSTenantID,
		FSPlacementRegionCode: fsPlacement.Code,
		FSCloudProvider:       fsPlacement.Provider,
		FSRegionCode:          fsPlacement.NativeCode,
		FSAPIKey:              creds.FSAPIKey,
	}, nil
}

func parsePlacement(regionCode string) (region.Placement, error) {
	placement, err := region.ParsePlacementCode(regionCode)
	if err != nil {
		return region.Placement{}, apperr.Wrap("config.invalid_region", "config", 2, err.Error(), err)
	}
	return placement, nil
}

func missingConfig(profileName, path, key string) error {
	return apperr.New(
		"config.missing_config",
		"config",
		2,
		fmt.Sprintf("%s missing for profile %q in %s; run tdc configure --profile %s or write ~/.tdc/config", key, profileName, path, profileName),
	)
}

func missingCredential(profileName, path, key string) error {
	return apperr.New(
		"config.missing_credentials",
		"config",
		2,
		fmt.Sprintf("%s missing for profile %q in %s; run tdc configure --profile %s or write ~/.tdc/credentials", key, profileName, path, profileName),
	)
}

func envMode(env map[string]string) bool {
	for _, key := range []string{"TDC_REGION_CODE", "TDC_PUBLIC_KEY", "TDC_PRIVATE_KEY"} {
		if envValue(env, key) != "" {
			return true
		}
	}
	return false
}

func firstMissingEnv(env map[string]string, keys ...string) string {
	for _, key := range keys {
		if envValue(env, key) == "" {
			return key
		}
	}
	return ""
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}
	return os.Getenv(key)
}
