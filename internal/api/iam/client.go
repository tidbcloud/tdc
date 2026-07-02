package iam

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Icemap/tdc/internal/api"
)

type Client struct {
	api *api.Client
}

func New(client *api.Client) *Client {
	return &Client{api: client}
}

type ListProjectsOptions struct {
	PageSize  int32
	PageToken string
}

type ListProjectsResponse struct {
	Projects      []Project `json:"projects"`
	NextPageToken string    `json:"next_page_token,omitempty"`
}

type Project struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	OrgID           string `json:"org_id"`
	ClusterCount    int32  `json:"cluster_count"`
	UserCount       int32  `json:"user_count"`
	CreateTimestamp string `json:"create_timestamp"`
	AWSCMEKEnabled  bool   `json:"aws_cmek_enabled"`
}

func (c *Client) ListProjects(ctx context.Context, opts ListProjectsOptions) (ListProjectsResponse, error) {
	requestPath := "/v1beta1/projects"
	query := url.Values{}
	if opts.PageToken != "" {
		query.Set("pageToken", opts.PageToken)
	}
	if opts.PageSize > 0 {
		query.Set("pageSize", strconv.FormatInt(int64(opts.PageSize), 10))
	}
	if encoded := query.Encode(); encoded != "" {
		requestPath += "?" + encoded
	}

	req, err := c.api.NewRequest(ctx, http.MethodGet, requestPath, nil)
	if err != nil {
		return ListProjectsResponse{}, err
	}
	var response listProjectsWireResponse
	if err := c.api.DoJSON(req, &response); err != nil {
		return ListProjectsResponse{}, err
	}
	return response.toResponse(), nil
}

func (c *Client) ListSQLUsers(ctx context.Context, clusterID string, out any) error {
	req, err := c.api.NewRequest(ctx, http.MethodGet, "/v1beta1/clusters/"+clusterID+"/sqlUsers", nil)
	if err != nil {
		return err
	}
	return c.api.DoJSON(req, out)
}

type listProjectsWireResponse struct {
	Projects         []Project `json:"projects"`
	NextPageToken    string    `json:"nextPageToken"`
	NextPageTokenAlt string    `json:"next_page_token"`
}

func (r listProjectsWireResponse) toResponse() ListProjectsResponse {
	nextPageToken := r.NextPageToken
	if nextPageToken == "" {
		nextPageToken = r.NextPageTokenAlt
	}
	projects := r.Projects
	if projects == nil {
		projects = []Project{}
	}
	return ListProjectsResponse{
		Projects:      projects,
		NextPageToken: nextPageToken,
	}
}
