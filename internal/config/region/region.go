package region

import (
	"fmt"
	"sort"
)

const (
	ProviderAWS          = "aws"
	ProviderAlibabaCloud = "alibaba_cloud"
)

type Region struct {
	Code  string
	Label string
}

var supported = map[string][]Region{
	ProviderAWS: {
		{Code: "us-east-1", Label: "N. Virginia"},
		{Code: "us-west-2", Label: "Oregon"},
		{Code: "eu-central-1", Label: "Frankfurt"},
		{Code: "ap-northeast-1", Label: "Tokyo"},
		{Code: "ap-southeast-1", Label: "Singapore"},
	},
	ProviderAlibabaCloud: {
		{Code: "ap-southeast-1", Label: "Singapore"},
	},
}

func Providers() []string {
	providers := make([]string, 0, len(supported))
	for provider := range supported {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

func Regions(provider string) ([]Region, error) {
	regions, ok := supported[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported cloud provider %q", provider)
	}
	return append([]Region(nil), regions...), nil
}

func DefaultRegion(provider string) (string, error) {
	regions, err := Regions(provider)
	if err != nil {
		return "", err
	}
	return regions[0].Code, nil
}

func Validate(provider, regionCode string) error {
	regions, ok := supported[provider]
	if !ok {
		return fmt.Errorf("unsupported cloud provider %q; supported providers: %s", provider, joinProviders())
	}
	for _, region := range regions {
		if region.Code == regionCode {
			return nil
		}
	}
	return fmt.Errorf("unsupported region %q for cloud provider %q; supported regions: %s", regionCode, provider, joinRegions(regions))
}

func joinProviders() string {
	providers := Providers()
	if len(providers) == 0 {
		return ""
	}
	out := providers[0]
	for _, provider := range providers[1:] {
		out += ", " + provider
	}
	return out
}

func joinRegions(regions []Region) string {
	if len(regions) == 0 {
		return ""
	}
	codes := make([]string, 0, len(regions))
	for _, region := range regions {
		codes = append(codes, region.Code)
	}
	sort.Strings(codes)
	out := codes[0]
	for _, code := range codes[1:] {
		out += ", " + code
	}
	return out
}
