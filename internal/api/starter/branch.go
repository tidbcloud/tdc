package starter

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ListBranchesOptions struct {
	PageSize  int32
	PageToken string
}

type ListBranchesResponse struct {
	Branches      []Branch `json:"branches"`
	NextPageToken string   `json:"next_page_token,omitempty"`
	TotalSize     int64    `json:"total_size,omitempty"`
}

type GetBranchOptions struct {
	View string
}

type CreateBranchRequest struct {
	DisplayName string
}

type Branch struct {
	ID                string            `json:"id"`
	Name              string            `json:"name,omitempty"`
	DisplayName       string            `json:"display_name"`
	ClusterID         string            `json:"cluster_id,omitempty"`
	ParentID          string            `json:"parent_id,omitempty"`
	CreatedBy         string            `json:"created_by,omitempty"`
	State             string            `json:"state,omitempty"`
	Endpoints         *Endpoints        `json:"endpoints,omitempty"`
	UserPrefix        string            `json:"user_prefix,omitempty"`
	CreateTime        string            `json:"create_time,omitempty"`
	UpdateTime        string            `json:"update_time,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ParentDisplayName string            `json:"parent_display_name,omitempty"`
	ParentTimestamp   string            `json:"parent_timestamp,omitempty"`
}

func (c *Client) ListBranches(ctx context.Context, clusterID string, opts ListBranchesOptions) (ListBranchesResponse, error) {
	requestPath := "/v1beta1/clusters/" + url.PathEscape(clusterID) + "/branches"
	query := url.Values{}
	if opts.PageSize > 0 {
		query.Set("pageSize", strconv.FormatInt(int64(opts.PageSize), 10))
	}
	if opts.PageToken != "" {
		query.Set("pageToken", opts.PageToken)
	}
	if encoded := query.Encode(); encoded != "" {
		requestPath += "?" + encoded
	}

	req, err := c.api.NewRequest(ctx, http.MethodGet, requestPath, nil)
	if err != nil {
		return ListBranchesResponse{}, err
	}
	var response listBranchesWireResponse
	if err := c.api.DoJSON(req, &response); err != nil {
		return ListBranchesResponse{}, err
	}
	return response.toResponse(), nil
}

func (c *Client) CreateBranch(ctx context.Context, clusterID string, input CreateBranchRequest) (Branch, error) {
	body := branchWire{
		DisplayName: input.DisplayName,
	}
	req, err := c.api.NewRequest(ctx, http.MethodPost, "/v1beta1/clusters/"+url.PathEscape(clusterID)+"/branches", body)
	if err != nil {
		return Branch{}, err
	}
	var response branchWire
	if err := c.api.DoJSON(req, &response); err != nil {
		return Branch{}, err
	}
	return response.toBranch(), nil
}

func (c *Client) GetBranch(ctx context.Context, clusterID, branchID string, opts GetBranchOptions) (Branch, error) {
	requestPath := "/v1beta1/clusters/" + url.PathEscape(clusterID) + "/branches/" + url.PathEscape(branchID)
	if opts.View != "" {
		query := url.Values{}
		query.Set("view", opts.View)
		requestPath += "?" + query.Encode()
	}
	req, err := c.api.NewRequest(ctx, http.MethodGet, requestPath, nil)
	if err != nil {
		return Branch{}, err
	}
	var response branchWire
	if err := c.api.DoJSON(req, &response); err != nil {
		return Branch{}, err
	}
	return response.toBranch(), nil
}

func (c *Client) DeleteBranch(ctx context.Context, clusterID, branchID string) (Branch, error) {
	req, err := c.api.NewRequest(ctx, http.MethodDelete, "/v1beta1/clusters/"+url.PathEscape(clusterID)+"/branches/"+url.PathEscape(branchID), nil)
	if err != nil {
		return Branch{}, err
	}
	var response branchWire
	if err := c.api.DoJSON(req, &response); err != nil {
		return Branch{}, err
	}
	return response.toBranch(), nil
}

type listBranchesWireResponse struct {
	Branches         []branchWire `json:"branches"`
	NextPageToken    string       `json:"nextPageToken"`
	NextPageTokenAlt string       `json:"next_page_token"`
	TotalSize        int64        `json:"totalSize"`
	TotalSizeAlt     int64        `json:"total_size"`
}

func (r listBranchesWireResponse) toResponse() ListBranchesResponse {
	branches := make([]Branch, 0, len(r.Branches))
	for _, branch := range r.Branches {
		branches = append(branches, branch.toBranch())
	}
	nextPageToken := r.NextPageToken
	if nextPageToken == "" {
		nextPageToken = r.NextPageTokenAlt
	}
	totalSize := r.TotalSize
	if totalSize == 0 {
		totalSize = r.TotalSizeAlt
	}
	return ListBranchesResponse{
		Branches:      branches,
		NextPageToken: nextPageToken,
		TotalSize:     totalSize,
	}
}

type branchWire struct {
	Name              string            `json:"name,omitempty"`
	BranchID          string            `json:"branchId,omitempty"`
	DisplayName       string            `json:"displayName"`
	ClusterID         string            `json:"clusterId,omitempty"`
	ParentID          string            `json:"parentId,omitempty"`
	CreatedBy         string            `json:"createdBy,omitempty"`
	State             string            `json:"state,omitempty"`
	Endpoints         *endpointsWire    `json:"endpoints,omitempty"`
	UserPrefix        string            `json:"userPrefix,omitempty"`
	CreateTime        string            `json:"createTime,omitempty"`
	UpdateTime        string            `json:"updateTime,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ParentDisplayName string            `json:"parentDisplayName,omitempty"`
	ParentTimestamp   string            `json:"parentTimestamp,omitempty"`
}

func (b branchWire) toBranch() Branch {
	id := b.BranchID
	if id == "" {
		id = branchIDFromName(b.Name)
	}
	return Branch{
		ID:                id,
		Name:              b.Name,
		DisplayName:       b.DisplayName,
		ClusterID:         b.ClusterID,
		ParentID:          b.ParentID,
		CreatedBy:         b.CreatedBy,
		State:             b.State,
		Endpoints:         b.Endpoints.toEndpoints(),
		UserPrefix:        b.UserPrefix,
		CreateTime:        b.CreateTime,
		UpdateTime:        b.UpdateTime,
		Annotations:       b.Annotations,
		ParentDisplayName: b.ParentDisplayName,
		ParentTimestamp:   b.ParentTimestamp,
	}
}

func branchIDFromName(name string) string {
	if name == "" {
		return ""
	}
	if idx := strings.LastIndex(name, "/branches/"); idx >= 0 {
		return name[idx+len("/branches/"):]
	}
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
