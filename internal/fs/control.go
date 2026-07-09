package fs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tidbcloud/tdc/internal/api"
	"github.com/tidbcloud/tdc/internal/api/endpoints"
	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/auth"
	"github.com/tidbcloud/tdc/internal/authz"
	"github.com/tidbcloud/tdc/internal/config"
	"github.com/tidbcloud/tdc/internal/dryrun"
	"github.com/tidbcloud/tdc/internal/fs/fscred"
)

type Service struct {
	Resolver    endpoints.Resolver
	HTTPClient  *http.Client
	Transport   http.RoundTripper
	Timeout     time.Duration
	Debug       bool
	DebugWriter io.Writer
	HomeDir     string
}

type CreateFileSystemOptions struct {
	Profile        *config.Profile
	FileSystemName string
}

type DeleteFileSystemOptions struct {
	Profile               *config.Profile
	FileSystemName        string
	ConfirmFileSystemName string
}

type CheckFileSystemOptions struct {
	Profile *config.Profile
}

type FileSystemResult struct {
	FileSystemName    string `json:"file_system_name"`
	TenantID          string `json:"tenant_id,omitempty"`
	CloudProvider     string `json:"cloud_provider,omitempty"`
	RegionCode        string `json:"region_code,omitempty"`
	Status            string `json:"status"`
	CredentialsStored bool   `json:"credentials_stored"`
}

type DeleteResult struct {
	FileSystemName      string `json:"file_system_name"`
	TenantID            string `json:"tenant_id,omitempty"`
	Status              string `json:"status"`
	CredentialsRemoved  bool   `json:"credentials_removed"`
	RemoteDeletionState string `json:"remote_deletion_state,omitempty"`
}

type CheckResult struct {
	Status   string                `json:"status"`
	Profile  string                `json:"profile"`
	Resource fscred.Resource       `json:"resource"`
	Endpoint *endpoints.Endpoint   `json:"endpoint,omitempty"`
	Remote   *apifs.StatusResponse `json:"remote,omitempty"`
	Checks   []Check               `json:"checks"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (s Service) CreateFileSystem(ctx context.Context, opts CreateFileSystemOptions) (FileSystemResult, error) {
	request, name, endpoint, err := s.createRequestAndEndpoint(opts, true)
	if err != nil {
		return FileSystemResult{}, err
	}
	if existing := fscred.FromProfile(opts.Profile); existing.Name != "" && existing.TenantID != "" && existing.HasAPIKey {
		if existing.Name != name {
			return FileSystemResult{}, resourceMismatch(existing.Name, name)
		}
		return FileSystemResult{
			FileSystemName:    existing.Name,
			TenantID:          existing.TenantID,
			CloudProvider:     existing.CloudProvider,
			RegionCode:        existing.RegionCode,
			Status:            "exists",
			CredentialsStored: true,
		}, nil
	}

	client, err := s.controlClient(opts.Profile, endpoint, authz.FSVolumeCreate, "create tdc fs resource")
	if err != nil {
		return FileSystemResult{}, err
	}
	response, err := client.Provision(ctx, request)
	if err != nil {
		return FileSystemResult{}, err
	}
	cloudProvider := response.CloudProvider
	if cloudProvider == "" {
		cloudProvider = opts.Profile.CloudProvider
	}
	regionCode := response.RegionCode
	if regionCode == "" {
		regionCode = response.Region
	}
	if regionCode == "" {
		regionCode = opts.Profile.RegionCode
	}
	status := response.Status
	if status == "" {
		status = "provisioned"
	}
	homeDir, err := s.homeDir()
	if err != nil {
		return FileSystemResult{}, err
	}
	if err := fscred.Store(homeDir, opts.Profile, name, response.TenantID, cloudProvider, regionCode, response.APIKey); err != nil {
		return FileSystemResult{}, err
	}
	return FileSystemResult{
		FileSystemName:    name,
		TenantID:          response.TenantID,
		CloudProvider:     cloudProvider,
		RegionCode:        regionCode,
		Status:            status,
		CredentialsStored: true,
	}, nil
}

func (s Service) DeleteFileSystem(ctx context.Context, opts DeleteFileSystemOptions) (DeleteResult, error) {
	name, endpoint, err := s.deleteInputsAndEndpoint(opts, true)
	if err != nil {
		return DeleteResult{}, err
	}
	resource := fscred.FromProfile(opts.Profile)
	request, err := deprovisionRequest(opts.Profile)
	if err != nil {
		return DeleteResult{}, err
	}
	client, err := s.bearerClient(opts.Profile, endpoint, authz.FSVolumeDelete, "delete tdc fs resource")
	if err != nil {
		return DeleteResult{}, err
	}
	response, err := client.DeleteTenant(ctx, request)
	if err != nil {
		return DeleteResult{}, err
	}
	status := response.Status
	if status == "" {
		status = "deleted"
	}
	homeDir, err := s.homeDir()
	if err != nil {
		return DeleteResult{}, err
	}
	if err := fscred.Clear(homeDir, opts.Profile); err != nil {
		return DeleteResult{}, err
	}
	tenantID := response.TenantID
	if tenantID == "" {
		tenantID = resource.TenantID
	}
	return DeleteResult{
		FileSystemName:      name,
		TenantID:            tenantID,
		Status:              "deleted",
		CredentialsRemoved:  true,
		RemoteDeletionState: status,
	}, nil
}

func (s Service) CheckFileSystem(ctx context.Context, opts CheckFileSystemOptions) (CheckResult, error) {
	if err := validateProfile(opts.Profile); err != nil {
		return CheckResult{}, err
	}
	checks := []Check{
		{Name: "config_and_credentials", Status: "passed", Message: fmt.Sprintf("profile %q loaded", profileName(opts.Profile))},
		{Name: "permission_requirement", Status: "passed", Message: string(authz.FSVolumeRead)},
	}
	resource := fscred.FromProfile(opts.Profile)
	if resource.Name == "" || resource.TenantID == "" || !resource.HasAPIKey {
		checks = append(checks, Check{Name: "fs_resource_credentials", Status: "warning", Message: "tdc fs resource credentials are not fully configured; run tdc fs create-file-system"})
	} else {
		checks = append(checks, Check{Name: "fs_resource_credentials", Status: "passed", Message: resource.Name})
	}
	endpoint, err := s.resolveFS(opts.Profile)
	if err != nil {
		checks = append(checks, Check{Name: "endpoint_selection", Status: "failed", Message: apperr.MessageFor(err)})
		return checkResult(opts.Profile, resource, nil, nil, checks), nil
	}
	checks = append(checks, Check{Name: "endpoint_selection", Status: "passed", Message: fmt.Sprintf("%s %s", endpoint.Provider, endpoint.RegionCode)})
	if !resource.HasAPIKey {
		checks = append(checks, Check{Name: "remote_status", Status: "warning", Message: "remote status requires fs_api_key; run tdc fs create-file-system first"})
		return checkResult(opts.Profile, resource, &endpoint, nil, checks), nil
	}

	client, err := s.statusClient(opts.Profile, endpoint, true, authz.FSVolumeRead, "check tdc fs resource")
	if err != nil {
		checks = append(checks, Check{Name: "remote_status", Status: "failed", Message: apperr.MessageFor(err)})
		return checkResult(opts.Profile, resource, &endpoint, nil, checks), nil
	}
	remote, err := client.Status(ctx)
	if err != nil {
		checks = append(checks, Check{Name: "remote_status", Status: "failed", Message: apperr.MessageFor(err)})
		return checkResult(opts.Profile, resource, &endpoint, nil, checks), nil
	}
	message := remote.Status
	if message == "" {
		message = "reachable"
	}
	checks = append(checks, Check{Name: "remote_status", Status: "passed", Message: message})
	return checkResult(opts.Profile, resource, &endpoint, &remote, checks), nil
}

func (s Service) DryRunCreateFileSystem(ctx context.Context, commandPath string, opts CreateFileSystemOptions) (dryrun.Result, error) {
	request, name, endpoint, endpointErr, err := s.createDryRunInputs(opts)
	if err != nil {
		return dryrun.Result{}, err
	}
	checks := []dryrun.Check{
		{Name: "config_and_credentials", Status: "passed", Message: fmt.Sprintf("profile %q loaded", profileName(opts.Profile))},
		{Name: "permission_requirement", Status: "passed", Message: string(authz.FSVolumeCreate)},
		{Name: "file_system_name", Status: "passed", Message: name},
	}
	checks = append(checks, endpointDryRunCheck(endpoint, endpointErr))
	return dryrun.New(
		commandPath,
		"create_file_system",
		dryrun.RequestSummary{
			Method: http.MethodPost,
			Path:   "/v1/provision",
			Body:   redactedProvisionRequest(request),
		},
		checks...,
	), nil
}

func (s Service) DryRunDeleteFileSystem(ctx context.Context, commandPath string, opts DeleteFileSystemOptions) (dryrun.Result, error) {
	name, endpoint, endpointErr, err := s.deleteDryRunInputs(opts)
	if err != nil {
		return dryrun.Result{}, err
	}
	checks := []dryrun.Check{
		{Name: "config_and_credentials", Status: "passed", Message: fmt.Sprintf("profile %q loaded", profileName(opts.Profile))},
		{Name: "permission_requirement", Status: "passed", Message: string(authz.FSVolumeDelete)},
		{Name: "delete_confirmation", Status: "passed", Message: opts.ConfirmFileSystemName},
	}
	if resource := fscred.FromProfile(opts.Profile); resource.HasAPIKey {
		checks = append(checks, dryrun.Check{Name: "fs_resource_credentials", Status: "passed", Message: name})
	} else {
		checks = append(checks, dryrun.Check{Name: "fs_resource_credentials", Status: "warning", Message: "fs_api_key is not configured; normal execution would fail before remote deletion"})
	}
	checks = append(checks, endpointDryRunCheck(endpoint, endpointErr))
	body, bodyErr := deprovisionRequest(opts.Profile)
	if bodyErr != nil {
		return dryrun.Result{}, bodyErr
	}
	return dryrun.New(
		commandPath,
		"delete_file_system",
		dryrun.RequestSummary{
			Method:      http.MethodDelete,
			Path:        "/v1/tenant",
			Body:        redactedDeprovisionRequest(body),
			Description: "normal execution verifies --confirm-file-system-name and uses the stored tdc fs API key before deleting",
		},
		checks...,
	), nil
}

func (s Service) createRequestAndEndpoint(opts CreateFileSystemOptions, requireEndpoint bool) (apifs.ProvisionRequest, string, endpoints.Endpoint, error) {
	request, name, endpoint, endpointErr, err := s.createDryRunInputs(opts)
	if err != nil {
		return apifs.ProvisionRequest{}, "", endpoints.Endpoint{}, err
	}
	if endpointErr != nil && requireEndpoint {
		return apifs.ProvisionRequest{}, "", endpoints.Endpoint{}, endpointErr
	}
	return request, name, endpoint, nil
}

func (s Service) createDryRunInputs(opts CreateFileSystemOptions) (apifs.ProvisionRequest, string, endpoints.Endpoint, error, error) {
	creds, err := auth.ValidateProfile(opts.Profile)
	if err != nil {
		return apifs.ProvisionRequest{}, "", endpoints.Endpoint{}, nil, err
	}
	name, err := fileSystemName(opts.FileSystemName)
	if err != nil {
		return apifs.ProvisionRequest{}, "", endpoints.Endpoint{}, nil, err
	}
	if existing := fscred.FromProfile(opts.Profile); existing.Name != "" && existing.Name != name {
		return apifs.ProvisionRequest{}, "", endpoints.Endpoint{}, nil, resourceMismatch(existing.Name, name)
	}
	endpoint, endpointErr := s.resolveFS(opts.Profile)
	defaultSpendingLimit := int64(0)
	request := apifs.ProvisionRequest{
		PublicKey:              creds.PublicKey,
		PrivateKey:             creds.PrivateKey,
		TiDBCloudSpendingLimit: &defaultSpendingLimit,
	}
	return request, name, endpoint, endpointErr, nil
}

func (s Service) deleteInputsAndEndpoint(opts DeleteFileSystemOptions, requireEndpoint bool) (string, endpoints.Endpoint, error) {
	name, endpoint, endpointErr, err := s.deleteDryRunInputs(opts)
	if err != nil {
		return "", endpoints.Endpoint{}, err
	}
	if endpointErr != nil && requireEndpoint {
		return "", endpoints.Endpoint{}, endpointErr
	}
	return name, endpoint, nil
}

func (s Service) deleteDryRunInputs(opts DeleteFileSystemOptions) (string, endpoints.Endpoint, error, error) {
	if err := validateProfile(opts.Profile); err != nil {
		return "", endpoints.Endpoint{}, nil, err
	}
	name, err := fileSystemName(opts.FileSystemName)
	if err != nil {
		return "", endpoints.Endpoint{}, nil, err
	}
	if strings.TrimSpace(opts.ConfirmFileSystemName) == "" {
		return "", endpoints.Endpoint{}, nil, apperr.New("fs.missing_confirmation", "usage", 2, "--confirm-file-system-name is required")
	}
	if strings.TrimSpace(opts.ConfirmFileSystemName) != name {
		return "", endpoints.Endpoint{}, nil, apperr.New("fs.delete_confirmation_mismatch", "usage", 2, fmt.Sprintf("--confirm-file-system-name must match --file-system-name %q", name))
	}
	if existing := fscred.FromProfile(opts.Profile); existing.Name != "" && existing.Name != name {
		return "", endpoints.Endpoint{}, nil, resourceMismatch(existing.Name, name)
	}
	endpoint, endpointErr := s.resolveFS(opts.Profile)
	return name, endpoint, endpointErr, nil
}

func (s Service) controlClient(profile *config.Profile, endpoint endpoints.Endpoint, permission authz.Permission, action string) (*apifs.Client, error) {
	if _, err := auth.ValidateProfile(profile); err != nil {
		return nil, err
	}
	client, err := api.New(api.Options{
		Endpoint:    endpoint,
		ProfileName: profileName(profile),
		Permission:  permission,
		Action:      action,
		HTTPClient:  s.HTTPClient,
		Transport:   s.Transport,
		Timeout:     s.Timeout,
		Debug:       s.Debug,
		DebugWriter: s.DebugWriter,
		UserAgent:   "tdc fs control",
	})
	if err != nil {
		return nil, err
	}
	return apifs.New(client), nil
}

func (s Service) statusClient(profile *config.Profile, endpoint endpoints.Endpoint, useBearer bool, permission authz.Permission, action string) (*apifs.Client, error) {
	if useBearer {
		return s.bearerClient(profile, endpoint, permission, action)
	}
	return s.controlClient(profile, endpoint, permission, action)
}

func (s Service) bearerClient(profile *config.Profile, endpoint endpoints.Endpoint, permission authz.Permission, action string) (*apifs.Client, error) {
	if profile == nil {
		return nil, apperr.New("fs.missing_profile", "config", 2, "active profile is required")
	}
	client, err := api.NewBearerClient(profileName(profile), profile.FSAPIKey, endpoint, permission, api.Options{
		Action:      action,
		HTTPClient:  s.HTTPClient,
		Transport:   s.Transport,
		Timeout:     s.Timeout,
		Debug:       s.Debug,
		DebugWriter: s.DebugWriter,
		UserAgent:   "tdc fs control",
	})
	if err != nil {
		return nil, err
	}
	return apifs.New(client), nil
}

func (s Service) resolveFS(profile *config.Profile) (endpoints.Endpoint, error) {
	return s.resolver().ResolveFS(profile.CloudProvider, profile.RegionCode)
}

func (s Service) resolver() endpoints.Resolver {
	if s.Resolver.IsZero() {
		return endpoints.NewResolver()
	}
	return s.Resolver
}

func deprovisionRequest(profile *config.Profile) (apifs.DeprovisionRequest, error) {
	creds, err := auth.ValidateProfile(profile)
	if err != nil {
		return apifs.DeprovisionRequest{}, err
	}
	return apifs.DeprovisionRequest{
		PublicKey:  creds.PublicKey,
		PrivateKey: creds.PrivateKey,
	}, nil
}

type redactedProvisionBody struct {
	PublicKey              string `json:"public_key,omitempty"`
	PrivateKey             string `json:"private_key,omitempty"`
	TiDBCloudSpendingLimit *int64 `json:"tidbcloud_spending_limit,omitempty"`
}

type redactedDeprovisionBody struct {
	PublicKey  string `json:"public_key,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
}

func redactedProvisionRequest(request apifs.ProvisionRequest) redactedProvisionBody {
	return redactedProvisionBody{
		PublicKey:              redactedConfiguredValue(request.PublicKey),
		PrivateKey:             redactedSecretValue(request.PrivateKey),
		TiDBCloudSpendingLimit: request.TiDBCloudSpendingLimit,
	}
}

func redactedDeprovisionRequest(request apifs.DeprovisionRequest) redactedDeprovisionBody {
	return redactedDeprovisionBody{
		PublicKey:  redactedConfiguredValue(request.PublicKey),
		PrivateKey: redactedSecretValue(request.PrivateKey),
	}
}

func redactedConfiguredValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[configured]"
}

func redactedSecretValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[redacted]"
}

func (s Service) homeDir() (string, error) {
	if s.HomeDir != "" {
		return s.HomeDir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", apperr.Wrap("fs.home_dir", "config", 1, "cannot determine home directory", err)
	}
	return homeDir, nil
}

func (s Service) metadataStore(profile *config.Profile) (*fsMetadataStore, error) {
	homeDir, err := s.homeDir()
	if err != nil {
		return nil, err
	}
	return newFSMetadataStore(homeDir, profile)
}

func validateProfile(profile *config.Profile) error {
	if _, err := auth.ValidateProfile(profile); err != nil {
		return err
	}
	return nil
}

func fileSystemName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", apperr.New("fs.missing_file_system_name", "usage", 2, "--file-system-name is required")
	}
	if len(trimmed) > 64 || strings.Contains(trimmed, "/") {
		return "", apperr.New("fs.invalid_file_system_name", "usage", 2, "--file-system-name must be 1-64 characters and must not contain /")
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return "", apperr.New("fs.invalid_file_system_name", "usage", 2, "--file-system-name must not contain control characters")
		}
	}
	return trimmed, nil
}

func resourceMismatch(existing, requested string) error {
	return apperr.New(
		"fs.resource_name_mismatch",
		"usage",
		2,
		fmt.Sprintf("profile is already configured for tdc fs resource %q; use that name or delete it before creating %q", existing, requested),
	)
}

func endpointDryRunCheck(endpoint endpoints.Endpoint, err error) dryrun.Check {
	if err != nil {
		return dryrun.Check{Name: "endpoint_selection", Status: "skipped", Message: apperr.MessageFor(err)}
	}
	return dryrun.Check{Name: "endpoint_selection", Status: "passed", Message: fmt.Sprintf("%s %s", endpoint.Provider, endpoint.RegionCode)}
}

func checkResult(profile *config.Profile, resource fscred.Resource, endpoint *endpoints.Endpoint, remote *apifs.StatusResponse, checks []Check) CheckResult {
	status := "passed"
	for _, check := range checks {
		if check.Status == "failed" {
			status = "failed"
			break
		}
		if check.Status == "warning" && status == "passed" {
			status = "warning"
		}
	}
	return CheckResult{
		Status:   status,
		Profile:  profileName(profile),
		Resource: resource,
		Endpoint: endpoint,
		Remote:   remote,
		Checks:   checks,
	}
}

func profileName(profile *config.Profile) string {
	if profile == nil || profile.Name == "" {
		return config.DefaultProfile
	}
	return profile.Name
}

func (r FileSystemResult) Human() string {
	lines := []string{
		"File system: " + r.FileSystemName,
		"Status: " + r.Status,
	}
	if r.TenantID != "" {
		lines = append(lines, "Tenant ID: "+r.TenantID)
	}
	if r.CloudProvider != "" || r.RegionCode != "" {
		lines = append(lines, "Location: "+strings.TrimSpace(r.CloudProvider+" "+r.RegionCode))
	}
	if r.CredentialsStored {
		lines = append(lines, "Credentials: stored in ~/.tdc/credentials")
	}
	return strings.Join(lines, "\n")
}

func (r DeleteResult) Human() string {
	lines := []string{
		"File system: " + r.FileSystemName,
		"Status: " + r.Status,
	}
	if r.TenantID != "" {
		lines = append(lines, "Tenant ID: "+r.TenantID)
	}
	if r.RemoteDeletionState != "" {
		lines = append(lines, "Remote deletion state: "+r.RemoteDeletionState)
	}
	if r.CredentialsRemoved {
		lines = append(lines, "Credentials: removed from ~/.tdc/credentials")
	}
	return strings.Join(lines, "\n")
}

func (r CheckResult) Human() string {
	var out strings.Builder
	_, _ = fmt.Fprintf(&out, "tdc fs check: %s\n", r.Status)
	writer := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "CHECK\tSTATUS\tMESSAGE")
	for _, check := range r.Checks {
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\n", check.Name, check.Status, check.Message)
	}
	_ = writer.Flush()
	return strings.TrimRight(out.String(), "\n")
}
