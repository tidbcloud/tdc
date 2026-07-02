package endpoints

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/config/region"
)

type Service string

const (
	ServiceStarter Service = "starter"
	ServiceIAM     Service = "iam"
	ServiceFS      Service = "fs"
)

const (
	DefaultStarterBaseURL = "https://serverless.tidbapi.com"
	DefaultIAMBaseURL     = "https://iam.tidbapi.com"
)

type ProviderRegion struct {
	Provider string
	Region   string
}

type Endpoint struct {
	Service     Service `json:"service"`
	BaseURL     string  `json:"base_url"`
	Provider    string  `json:"provider,omitempty"`
	APIProvider string  `json:"api_provider,omitempty"`
	RegionCode  string  `json:"region_code,omitempty"`
	RegionName  string  `json:"region_name,omitempty"`
}

type Resolver struct {
	StarterBaseURL string
	IAMBaseURL     string
	FSBaseURLs     map[ProviderRegion]string
}

func NewResolver() Resolver {
	return Resolver{
		StarterBaseURL: DefaultStarterBaseURL,
		IAMBaseURL:     DefaultIAMBaseURL,
	}
}

func (r Resolver) Resolve(service Service, provider, regionCode string) (Endpoint, error) {
	switch service {
	case ServiceStarter:
		return r.ResolveStarter(provider, regionCode)
	case ServiceIAM:
		return r.ResolveIAM()
	case ServiceFS:
		return r.ResolveFS(provider, regionCode)
	default:
		return Endpoint{}, apperr.New("api.unknown_service", "usage", 2, fmt.Sprintf("unknown API service %q", service))
	}
}

func (r Resolver) ResolveStarter(provider, regionCode string) (Endpoint, error) {
	if err := region.Validate(provider, regionCode); err != nil {
		return Endpoint{}, apperr.Wrap("config.invalid_region", "config", 2, err.Error(), err)
	}
	baseURL := r.StarterBaseURL
	if baseURL == "" {
		baseURL = DefaultStarterBaseURL
	}
	if err := validateBaseURL(baseURL); err != nil {
		return Endpoint{}, err
	}
	apiProvider := APIProvider(provider)
	return Endpoint{
		Service:     ServiceStarter,
		BaseURL:     strings.TrimRight(baseURL, "/"),
		Provider:    provider,
		APIProvider: apiProvider,
		RegionCode:  regionCode,
		RegionName:  "regions/" + apiProvider + "-" + regionCode,
	}, nil
}

func (r Resolver) ResolveIAM() (Endpoint, error) {
	baseURL := r.IAMBaseURL
	if baseURL == "" {
		baseURL = DefaultIAMBaseURL
	}
	if err := validateBaseURL(baseURL); err != nil {
		return Endpoint{}, err
	}
	return Endpoint{
		Service: ServiceIAM,
		BaseURL: strings.TrimRight(baseURL, "/"),
	}, nil
}

func (r Resolver) ResolveFS(provider, regionCode string) (Endpoint, error) {
	if err := region.Validate(provider, regionCode); err != nil {
		return Endpoint{}, apperr.Wrap("config.invalid_region", "config", 2, err.Error(), err)
	}
	if r.FSBaseURLs == nil {
		return Endpoint{}, apperr.New(
			"api.fs_endpoint_unavailable",
			"api",
			1,
			fmt.Sprintf("tdc fs endpoint is not configured for %s/%s; a product endpoint contract or service discovery API is required before fs requests can run", provider, regionCode),
		)
	}
	baseURL := r.FSBaseURLs[ProviderRegion{Provider: provider, Region: regionCode}]
	if baseURL == "" {
		return Endpoint{}, apperr.New(
			"api.fs_endpoint_unavailable",
			"api",
			1,
			fmt.Sprintf("tdc fs endpoint is not configured for %s/%s; a product endpoint contract or service discovery API is required before fs requests can run", provider, regionCode),
		)
	}
	if err := validateBaseURL(baseURL); err != nil {
		return Endpoint{}, err
	}
	return Endpoint{
		Service:     ServiceFS,
		BaseURL:     strings.TrimRight(baseURL, "/"),
		Provider:    provider,
		APIProvider: APIProvider(provider),
		RegionCode:  regionCode,
	}, nil
}

func APIProvider(provider string) string {
	switch provider {
	case region.ProviderAlibabaCloud:
		return "alicloud"
	default:
		return provider
	}
}

func validateBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return apperr.Wrap("api.invalid_endpoint", "config", 2, fmt.Sprintf("invalid internal endpoint %q", value), err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return apperr.New("api.invalid_endpoint", "config", 2, fmt.Sprintf("invalid internal endpoint %q: scheme must be https or http", value))
	}
	return nil
}
