package region

import "testing"

func TestValidateSupportedProviderRegions(t *testing.T) {
	tests := []struct {
		provider string
		region   string
	}{
		{ProviderAWS, "us-east-1"},
		{ProviderAWS, "us-west-2"},
		{ProviderAWS, "eu-central-1"},
		{ProviderAWS, "ap-northeast-1"},
		{ProviderAWS, "ap-southeast-1"},
		{ProviderAlibabaCloud, "ap-southeast-1"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.region, func(t *testing.T) {
			if err := Validate(tt.provider, tt.region); err != nil {
				t.Fatalf("expected provider/region to be valid: %v", err)
			}
		})
	}
}

func TestValidateRejectsUnsupportedProviderRegions(t *testing.T) {
	tests := []struct {
		provider string
		region   string
	}{
		{"gcp", "us-east-1"},
		{ProviderAlibabaCloud, "us-east-1"},
		{ProviderAWS, "cn-hangzhou"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.region, func(t *testing.T) {
			if err := Validate(tt.provider, tt.region); err == nil {
				t.Fatal("expected provider/region to be rejected")
			}
		})
	}
}
