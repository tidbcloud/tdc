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
