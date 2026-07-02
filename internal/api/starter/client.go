package starter

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

func (c *Client) ListClusters(ctx context.Context, out any) error {
	req, err := c.api.NewRequest(ctx, http.MethodGet, "/v1beta1/clusters", nil)
	if err != nil {
		return err
	}
	return c.api.DoJSON(req, out)
}

func (c *Client) GetCluster(ctx context.Context, clusterID string, out any) error {
	req, err := c.api.NewRequest(ctx, http.MethodGet, "/v1beta1/clusters/"+clusterID, nil)
	if err != nil {
		return err
	}
	return c.api.DoJSON(req, out)
}
