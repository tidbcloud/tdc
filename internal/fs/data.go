package fs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Icemap/tdc/internal/api"
	apifs "github.com/Icemap/tdc/internal/api/fs"
	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/config"
)

type CopyFileOptions struct {
	Profile       *config.Profile
	FromLocal     string
	ToLocal       string
	FromRemote    string
	ToRemote      string
	Overwrite     bool
	CreateParents bool
}

type ReadFileOptions struct {
	Profile *config.Profile
	Path    string
}

type ListFilesOptions struct {
	Profile *config.Profile
	Path    string
}

type DescribeFileOptions struct {
	Profile *config.Profile
	Path    string
}

type MoveFileOptions struct {
	Profile    *config.Profile
	FromRemote string
	ToRemote   string
	Overwrite  bool
}

type DeleteFileOptions struct {
	Profile   *config.Profile
	Path      string
	Recursive bool
}

type CreateDirectoryOptions struct {
	Profile *config.Profile
	Path    string
	Mode    string
}

type SearchFileContentOptions struct {
	Profile *config.Profile
	Path    string
	Pattern string
	Limit   int32
}

type FindFilesOptions struct {
	Profile         *config.Profile
	Path            string
	FileNamePattern string
	ResourceType    string
	Tag             string
	Newer           string
	Older           string
	MinSizeBytes    int64
	MaxSizeBytes    int64
	Limit           int32
}

type FileOperationResult struct {
	Operation        string `json:"operation"`
	SourcePath       string `json:"source_path,omitempty"`
	TargetPath       string `json:"target_path,omitempty"`
	BytesTransferred int64  `json:"bytes_transferred,omitempty"`
	Revision         int64  `json:"revision,omitempty"`
	Status           string `json:"status"`
}

type FileEntry struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	IsDir     bool   `json:"is_dir"`
	Mtime     int64  `json:"mtime,omitempty"`
}

type ListFilesResult struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}

type DescribeFileResult struct {
	Path         string            `json:"path"`
	SizeBytes    int64             `json:"size_bytes"`
	IsDir        bool              `json:"is_dir"`
	Revision     int64             `json:"revision,omitempty"`
	Mtime        int64             `json:"mtime,omitempty"`
	Mode         int64             `json:"mode,omitempty"`
	HasMode      bool              `json:"has_mode,omitempty"`
	ResourceID   string            `json:"resource_id,omitempty"`
	Nlink        int64             `json:"nlink,omitempty"`
	ContentType  string            `json:"content_type,omitempty"`
	SemanticText string            `json:"semantic_text,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	Degraded     bool              `json:"degraded,omitempty"`
}

type SearchResult struct {
	Path      string   `json:"path"`
	Name      string   `json:"name,omitempty"`
	SizeBytes int64    `json:"size_bytes,omitempty"`
	Score     *float64 `json:"score,omitempty"`
}

type SearchFilesResult struct {
	Path    string         `json:"path"`
	Results []SearchResult `json:"results"`
}

func (s Service) CopyFile(ctx context.Context, opts CopyFileOptions) (FileOperationResult, error) {
	fromLocal := strings.TrimSpace(opts.FromLocal)
	toLocal := strings.TrimSpace(opts.ToLocal)
	fromRemote := strings.TrimSpace(opts.FromRemote)
	toRemote := strings.TrimSpace(opts.ToRemote)
	switch {
	case fromLocal != "" && toRemote != "" && fromRemote == "" && toLocal == "":
		client, err := s.dataClient(opts.Profile, authz.FSFileWrite, "write tdc fs file")
		if err != nil {
			return FileOperationResult{}, err
		}
		target, err := normalizeRemotePath(toRemote)
		if err != nil {
			return FileOperationResult{}, err
		}
		if err := ensureRemoteTargetCanWrite(ctx, client, target, opts.Overwrite); err != nil {
			return FileOperationResult{}, err
		}
		data, err := os.ReadFile(fromLocal)
		if err != nil {
			return FileOperationResult{}, apperr.Wrap("fs.read_local_file", "runtime", 1, fmt.Sprintf("read local file %q", fromLocal), err)
		}
		written, err := client.WriteFile(ctx, target, data)
		if err != nil {
			return FileOperationResult{}, err
		}
		return FileOperationResult{Operation: "copy_file", SourcePath: fromLocal, TargetPath: target, BytesTransferred: int64(len(data)), Revision: written.Revision, Status: "copied"}, nil
	case fromRemote != "" && toLocal != "" && fromLocal == "" && toRemote == "":
		client, err := s.dataClient(opts.Profile, authz.FSFileRead, "read tdc fs file")
		if err != nil {
			return FileOperationResult{}, err
		}
		source, err := normalizeRemotePath(fromRemote)
		if err != nil {
			return FileOperationResult{}, err
		}
		if err := ensureLocalTargetCanWrite(toLocal, opts.Overwrite, opts.CreateParents); err != nil {
			return FileOperationResult{}, err
		}
		data, err := client.ReadFile(ctx, source)
		if err != nil {
			return FileOperationResult{}, err
		}
		if err := os.WriteFile(toLocal, data, 0o644); err != nil {
			return FileOperationResult{}, apperr.Wrap("fs.write_local_file", "runtime", 1, fmt.Sprintf("write local file %q", toLocal), err)
		}
		return FileOperationResult{Operation: "copy_file", SourcePath: source, TargetPath: toLocal, BytesTransferred: int64(len(data)), Status: "copied"}, nil
	case fromRemote != "" && toRemote != "" && fromLocal == "" && toLocal == "":
		client, err := s.dataClient(opts.Profile, authz.FSFileWrite, "copy tdc fs file")
		if err != nil {
			return FileOperationResult{}, err
		}
		source, err := normalizeRemotePath(fromRemote)
		if err != nil {
			return FileOperationResult{}, err
		}
		target, err := normalizeRemotePath(toRemote)
		if err != nil {
			return FileOperationResult{}, err
		}
		if err := ensureRemoteTargetCanWrite(ctx, client, target, opts.Overwrite); err != nil {
			return FileOperationResult{}, err
		}
		if err := client.CopyRemote(ctx, source, target); err != nil {
			return FileOperationResult{}, err
		}
		return FileOperationResult{Operation: "copy_file", SourcePath: source, TargetPath: target, Status: "copied"}, nil
	default:
		return FileOperationResult{}, apperr.New("fs.invalid_copy_flags", "usage", 2, "copy-file requires exactly one source/target pair: --from-local with --to-remote, --from-remote with --to-local, or --from-remote with --to-remote")
	}
}

func (s Service) ReadFile(ctx context.Context, opts ReadFileOptions) ([]byte, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileRead, "read tdc fs file")
	if err != nil {
		return nil, err
	}
	remotePath, err := normalizeRemotePath(opts.Path)
	if err != nil {
		return nil, err
	}
	return client.ReadFile(ctx, remotePath)
}

func (s Service) ListFiles(ctx context.Context, opts ListFilesOptions) (ListFilesResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileRead, "list tdc fs files")
	if err != nil {
		return ListFilesResult{}, err
	}
	remotePath, err := normalizeRemotePath(defaultRemotePath(opts.Path))
	if err != nil {
		return ListFilesResult{}, err
	}
	response, err := client.List(ctx, remotePath)
	if err != nil {
		return ListFilesResult{}, err
	}
	entries := make([]FileEntry, 0, len(response.Entries))
	for _, entry := range response.Entries {
		entries = append(entries, FileEntry{Name: entry.Name, SizeBytes: entry.Size, IsDir: entry.IsDir, Mtime: entry.Mtime})
	}
	return ListFilesResult{Path: remotePath, Entries: entries}, nil
}

func (s Service) DescribeFile(ctx context.Context, opts DescribeFileOptions) (DescribeFileResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileRead, "describe tdc fs file")
	if err != nil {
		return DescribeFileResult{}, err
	}
	remotePath, err := normalizeRemotePath(opts.Path)
	if err != nil {
		return DescribeFileResult{}, err
	}
	metadata, err := client.StatMetadata(ctx, remotePath)
	if err == nil {
		return describeFromMetadata(remotePath, metadata), nil
	}
	if !shouldFallbackStat(err) {
		return DescribeFileResult{}, err
	}
	stat, statErr := client.Stat(ctx, remotePath)
	if statErr != nil {
		return DescribeFileResult{}, statErr
	}
	return describeFromStat(stat, true), nil
}

func (s Service) MoveFile(ctx context.Context, opts MoveFileOptions) (FileOperationResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileWrite, "move tdc fs file")
	if err != nil {
		return FileOperationResult{}, err
	}
	source, err := normalizeRemotePath(opts.FromRemote)
	if err != nil {
		return FileOperationResult{}, err
	}
	target, err := normalizeRemotePath(opts.ToRemote)
	if err != nil {
		return FileOperationResult{}, err
	}
	if err := ensureRemoteTargetCanWrite(ctx, client, target, opts.Overwrite); err != nil {
		return FileOperationResult{}, err
	}
	if err := client.Rename(ctx, source, target); err != nil {
		return FileOperationResult{}, err
	}
	return FileOperationResult{Operation: "move_file", SourcePath: source, TargetPath: target, Status: "moved"}, nil
}

func (s Service) DeleteFile(ctx context.Context, opts DeleteFileOptions) (FileOperationResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileWrite, "delete tdc fs file")
	if err != nil {
		return FileOperationResult{}, err
	}
	remotePath, err := normalizeRemotePath(opts.Path)
	if err != nil {
		return FileOperationResult{}, err
	}
	if err := client.DeleteFile(ctx, remotePath, opts.Recursive); err != nil {
		return FileOperationResult{}, err
	}
	return FileOperationResult{Operation: "delete_file", TargetPath: remotePath, Status: "deleted"}, nil
}

func (s Service) CreateDirectory(ctx context.Context, opts CreateDirectoryOptions) (FileOperationResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileWrite, "create tdc fs directory")
	if err != nil {
		return FileOperationResult{}, err
	}
	remotePath, err := normalizeRemotePath(opts.Path)
	if err != nil {
		return FileOperationResult{}, err
	}
	mode, err := parseMode(opts.Mode)
	if err != nil {
		return FileOperationResult{}, err
	}
	if err := client.Mkdir(ctx, remotePath, mode); err != nil {
		return FileOperationResult{}, err
	}
	return FileOperationResult{Operation: "create_directory", TargetPath: remotePath, Status: "created"}, nil
}

func (s Service) SearchFileContent(ctx context.Context, opts SearchFileContentOptions) (SearchFilesResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileRead, "search tdc fs file content")
	if err != nil {
		return SearchFilesResult{}, err
	}
	remotePath, err := normalizeRemotePath(defaultRemotePath(opts.Path))
	if err != nil {
		return SearchFilesResult{}, err
	}
	pattern := strings.TrimSpace(opts.Pattern)
	if pattern == "" {
		return SearchFilesResult{}, apperr.New("fs.missing_pattern", "usage", 2, "--pattern is required")
	}
	results, err := client.Grep(ctx, remotePath, pattern, opts.Limit)
	if err != nil {
		return SearchFilesResult{}, err
	}
	return SearchFilesResult{Path: remotePath, Results: searchResults(results)}, nil
}

func (s Service) FindFiles(ctx context.Context, opts FindFilesOptions) (SearchFilesResult, error) {
	client, err := s.dataClient(opts.Profile, authz.FSFileRead, "find tdc fs files")
	if err != nil {
		return SearchFilesResult{}, err
	}
	remotePath, err := normalizeRemotePath(defaultRemotePath(opts.Path))
	if err != nil {
		return SearchFilesResult{}, err
	}
	params := url.Values{}
	if opts.FileNamePattern != "" {
		params.Set("name", opts.FileNamePattern)
	}
	if opts.ResourceType != "" {
		params.Set("type", opts.ResourceType)
	}
	if opts.Tag != "" {
		params.Set("tag", opts.Tag)
	}
	if opts.Newer != "" {
		params.Set("newer", opts.Newer)
	}
	if opts.Older != "" {
		params.Set("older", opts.Older)
	}
	if opts.MinSizeBytes > 0 {
		params.Set("minsize", strconv.FormatInt(opts.MinSizeBytes, 10))
	}
	if opts.MaxSizeBytes > 0 {
		params.Set("maxsize", strconv.FormatInt(opts.MaxSizeBytes, 10))
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.FormatInt(int64(opts.Limit), 10))
	}
	results, err := client.Find(ctx, remotePath, params)
	if err != nil {
		return SearchFilesResult{}, err
	}
	return SearchFilesResult{Path: remotePath, Results: searchResults(results)}, nil
}

func (s Service) dataClient(profile *config.Profile, permission authz.Permission, action string) (*apifs.Client, error) {
	if profile == nil {
		return nil, apperr.New("fs.missing_profile", "config", 2, "active profile is required")
	}
	endpoint, err := s.resolveFS(profile)
	if err != nil {
		return nil, err
	}
	return s.bearerClient(profile, endpoint, permission, action)
}

func normalizeRemotePath(value string) (string, error) {
	if value == "" {
		return "", apperr.New("fs.missing_remote_path", "usage", 2, "remote path is required")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", apperr.New("fs.invalid_remote_path", "usage", 2, "remote path must not contain control characters")
		}
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	hasTrailingSlash := strings.HasSuffix(value, "/")
	cleaned := path.Clean(value)
	if cleaned == "." {
		cleaned = "/"
	}
	if hasTrailingSlash && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned, nil
}

func defaultRemotePath(value string) string {
	if strings.TrimSpace(value) == "" {
		return "/"
	}
	return value
}

func parseMode(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 8, 64)
	if err != nil || parsed < 0 {
		return 0, apperr.New("fs.invalid_mode", "usage", 2, "--mode must be an octal value such as 0755")
	}
	return parsed, nil
}

func ensureRemoteTargetCanWrite(ctx context.Context, client *apifs.Client, remotePath string, overwrite bool) error {
	if overwrite {
		return nil
	}
	_, err := client.Stat(ctx, remotePath)
	if err == nil {
		return apperr.New("fs.target_exists", "usage", 2, fmt.Sprintf("remote target %q already exists; pass --overwrite to replace it", remotePath))
	}
	if isNotFound(err) {
		return nil
	}
	return err
}

func ensureLocalTargetCanWrite(localPath string, overwrite, createParents bool) error {
	if strings.TrimSpace(localPath) == "" {
		return apperr.New("fs.missing_local_path", "usage", 2, "local path is required")
	}
	if createParents {
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return apperr.Wrap("fs.create_local_parent", "runtime", 1, fmt.Sprintf("create parent directories for %q", localPath), err)
		}
	}
	if overwrite {
		return nil
	}
	if _, err := os.Stat(localPath); err == nil {
		return apperr.New("fs.target_exists", "usage", 2, fmt.Sprintf("local target %q already exists; pass --overwrite to replace it", localPath))
	} else if !errors.Is(err, os.ErrNotExist) {
		return apperr.Wrap("fs.stat_local_file", "runtime", 1, fmt.Sprintf("stat local file %q", localPath), err)
	}
	return nil
}

func isNotFound(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func shouldFallbackStat(err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == "api.contract_gap" || apiErr.Code == "api.decode_response" || apiErr.StatusCode == http.StatusBadRequest
}

func describeFromMetadata(remotePath string, metadata apifs.StatMetadataResponse) DescribeFileResult {
	return DescribeFileResult{
		Path:         remotePath,
		SizeBytes:    metadata.Size,
		IsDir:        metadata.IsDir,
		Revision:     metadata.Revision,
		Mtime:        metadata.Mtime,
		ResourceID:   metadata.ResourceID,
		Nlink:        metadata.Nlink,
		ContentType:  metadata.ContentType,
		SemanticText: metadata.SemanticText,
		Tags:         metadata.Tags,
		Degraded:     metadata.Degraded,
	}
}

func describeFromStat(stat apifs.StatResponse, degraded bool) DescribeFileResult {
	return DescribeFileResult{
		Path:       stat.Path,
		SizeBytes:  stat.SizeBytes,
		IsDir:      stat.IsDir,
		Revision:   stat.Revision,
		Mtime:      stat.Mtime,
		Mode:       stat.Mode,
		HasMode:    stat.HasMode,
		ResourceID: stat.ResourceID,
		Nlink:      stat.Nlink,
		Degraded:   degraded,
	}
}

func searchResults(results []apifs.SearchResult) []SearchResult {
	out := make([]SearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, SearchResult{Path: result.Path, Name: result.Name, SizeBytes: result.SizeBytes, Score: result.Score})
	}
	return out
}

func (r FileOperationResult) Human() string {
	lines := []string{
		"Operation: " + r.Operation,
		"Status: " + r.Status,
	}
	if r.SourcePath != "" {
		lines = append(lines, "Source: "+r.SourcePath)
	}
	if r.TargetPath != "" {
		lines = append(lines, "Target: "+r.TargetPath)
	}
	if r.BytesTransferred > 0 {
		lines = append(lines, fmt.Sprintf("Bytes: %d", r.BytesTransferred))
	}
	return strings.Join(lines, "\n")
}

func (r ListFilesResult) Human() string {
	var out strings.Builder
	writer := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tTYPE\tSIZE\tMTIME")
	for _, entry := range r.Entries {
		kind := "file"
		if entry.IsDir {
			kind = "dir"
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%d\n", entry.Name, kind, entry.SizeBytes, entry.Mtime)
	}
	_ = writer.Flush()
	return strings.TrimRight(out.String(), "\n")
}

func (r DescribeFileResult) Human() string {
	lines := []string{
		"Path: " + r.Path,
		fmt.Sprintf("Size: %d", r.SizeBytes),
		fmt.Sprintf("Directory: %t", r.IsDir),
	}
	if r.Revision != 0 {
		lines = append(lines, fmt.Sprintf("Revision: %d", r.Revision))
	}
	if r.ContentType != "" {
		lines = append(lines, "Content type: "+r.ContentType)
	}
	return strings.Join(lines, "\n")
}

func (r SearchFilesResult) Human() string {
	var out strings.Builder
	writer := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "PATH\tNAME\tSIZE\tSCORE")
	for _, result := range r.Results {
		score := ""
		if result.Score != nil {
			score = fmt.Sprintf("%.4f", *result.Score)
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n", result.Path, result.Name, result.SizeBytes, score)
	}
	_ = writer.Flush()
	return strings.TrimRight(out.String(), "\n")
}
