package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/Icemap/tdc/internal/api/transport"
	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/config"
)

type Credentials struct {
	ProfileName string
	PublicKey   string
	PrivateKey  string
}

func LoadProfile(ctx context.Context, opts config.LoadOptions) (*config.Profile, error) {
	profile, err := config.Load(ctx, opts)
	if err == nil {
		if _, err := ValidateProfile(profile); err != nil {
			return nil, err
		}
		return profile, nil
	}

	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case "config.missing_credentials":
			profileName := opts.Profile
			if profileName == "" {
				profileName = config.DefaultProfile
			}
			return nil, MissingCredentials(profileName, "tdc_public_key", "tdc_private_key")
		case "config.env_missing":
			if strings.Contains(appErr.Message, "TDC_PUBLIC_KEY") || strings.Contains(appErr.Message, "TDC_PRIVATE_KEY") {
				return nil, MissingCredentials("env", "TDC_PUBLIC_KEY", "TDC_PRIVATE_KEY")
			}
		}
	}

	return nil, err
}

func ValidateProfile(profile *config.Profile) (Credentials, error) {
	if profile == nil {
		return Credentials{}, MissingCredentials(config.DefaultProfile, "tdc_public_key", "tdc_private_key")
	}

	profileName := profile.Name
	if profileName == "" {
		profileName = config.DefaultProfile
	}

	missing := make([]string, 0, 2)
	if strings.TrimSpace(profile.TDCPublicKey) == "" {
		missing = append(missing, "tdc_public_key")
	}
	if strings.TrimSpace(profile.TDCPrivateKey) == "" {
		missing = append(missing, "tdc_private_key")
	}
	if len(missing) > 0 {
		return Credentials{}, MissingCredentials(profileName, missing...)
	}

	if malformedCredential(profile.TDCPublicKey) || strings.Contains(profile.TDCPublicKey, ":") {
		return Credentials{}, MalformedCredentials(profileName, "tdc_public_key")
	}
	if malformedCredential(profile.TDCPrivateKey) {
		return Credentials{}, MalformedCredentials(profileName, "tdc_private_key")
	}

	return Credentials{
		ProfileName: profileName,
		PublicKey:   profile.TDCPublicKey,
		PrivateKey:  profile.TDCPrivateKey,
	}, nil
}

func NewDigestTransport(creds Credentials, base http.RoundTripper) http.RoundTripper {
	return transport.NewDigest(creds.PublicKey, creds.PrivateKey, base)
}

func MissingCredentials(profileName string, keys ...string) error {
	if profileName == "" {
		profileName = config.DefaultProfile
	}
	if len(keys) == 0 {
		keys = []string{"tdc_public_key", "tdc_private_key"}
	}
	return apperr.New(
		"auth.missing_credentials",
		"authentication",
		3,
		fmt.Sprintf(
			"authentication required: missing %s for profile %q. Run `tdc configure` or set TDC_PUBLIC_KEY and TDC_PRIVATE_KEY.",
			joinKeys(keys),
			profileName,
		),
	)
}

func MalformedCredentials(profileName, key string) error {
	if profileName == "" {
		profileName = config.DefaultProfile
	}
	return apperr.New(
		"auth.malformed_credentials",
		"authentication",
		3,
		fmt.Sprintf("authentication failed: malformed %s for profile %q. Check ~/.tdc/credentials or create a new API key.", key, profileName),
	)
}

func malformedCredential(value string) bool {
	if value != strings.TrimSpace(value) {
		return true
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func joinKeys(keys []string) string {
	if len(keys) == 1 {
		return keys[0]
	}
	if len(keys) == 2 {
		return keys[0] + " and " + keys[1]
	}
	return strings.Join(keys, ", ")
}
