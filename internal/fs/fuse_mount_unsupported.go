//go:build windows

package fs

import (
	"context"

	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/apperr"
)

func (s Service) mountFUSEForeground(ctx context.Context, inputs mountInputs, remote apifs.StatusResponse, checks []MountRuntimeCheck) (MountResult, error) {
	return MountResult{}, apperr.New("fs.fuse_unsupported", "runtime", 1, "tdc fs FUSE mount is not supported on Windows; explicitly use --driver webdav if a WebDAV mount is available")
}
