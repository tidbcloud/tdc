package fs

import (
	"context"
	"net/http"

	"github.com/Icemap/tdc/internal/api"
)

type Client struct {
	api *api.Client
}

func New(client *api.Client) *Client {
	return &Client{api: client}
}

func (c *Client) CheckStatus(ctx context.Context, out any) error {
	req, err := c.api.NewRequest(ctx, http.MethodGet, "/v1/status", nil)
	if err != nil {
		return err
	}
	return c.api.DoJSON(req, out)
}

type StatusResponse struct {
	Status          string         `json:"status,omitempty"`
	TenantID        string         `json:"tenant_id,omitempty"`
	Kind            string         `json:"kind,omitempty"`
	Version         string         `json:"version,omitempty"`
	InlineThreshold int64          `json:"inline_threshold,omitempty"`
	MaxUploadBytes  int64          `json:"max_upload_bytes,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

type ProvisionRequest struct {
	PublicKey              string `json:"public_key,omitempty"`
	PrivateKey             string `json:"private_key,omitempty"`
	TiDBCloudSpendingLimit *int64 `json:"tidbcloud_spending_limit,omitempty"`
}

type ProvisionResponse struct {
	TenantID      string `json:"tenant_id"`
	APIKey        string `json:"api_key"`
	Status        string `json:"status"`
	CloudProvider string `json:"cloud_provider,omitempty"`
	RegionCode    string `json:"region_code,omitempty"`
	Region        string `json:"region,omitempty"`
}

type DeleteResponse struct {
	TenantID string `json:"tenant_id,omitempty"`
	Status   string `json:"status"`
}

type DeprovisionRequest struct {
	PublicKey  string `json:"public_key,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var response StatusResponse
	if err := c.CheckStatus(ctx, &response); err != nil {
		return StatusResponse{}, err
	}
	return response, nil
}

func (c *Client) Provision(ctx context.Context, request ProvisionRequest) (ProvisionResponse, error) {
	req, err := c.api.NewRequest(ctx, http.MethodPost, "/v1/provision", provisionBody(request))
	if err != nil {
		return ProvisionResponse{}, err
	}
	var response ProvisionResponse
	if err := c.api.DoJSON(req, &response); err != nil {
		return ProvisionResponse{}, err
	}
	return response, nil
}

func (c *Client) DeleteTenant(ctx context.Context, request DeprovisionRequest) (DeleteResponse, error) {
	req, err := c.api.NewRequest(ctx, http.MethodDelete, "/v1/tenant", deprovisionBody(request))
	if err != nil {
		return DeleteResponse{}, err
	}
	var response DeleteResponse
	if err := c.api.DoJSON(req, &response); err != nil {
		return DeleteResponse{}, err
	}
	return response, nil
}

func provisionBody(request ProvisionRequest) any {
	if request.PublicKey == "" && request.PrivateKey == "" && request.TiDBCloudSpendingLimit == nil {
		return nil
	}
	return request
}

func deprovisionBody(request DeprovisionRequest) any {
	if request.PublicKey == "" && request.PrivateKey == "" {
		return nil
	}
	return request
}
