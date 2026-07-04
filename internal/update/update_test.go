package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/version"
)

func TestCheckReportsAvailableRelease(t *testing.T) {
	server := fakeReleaseServer(t, releaseFixture{
		tag: "v0.2.0",
		assets: map[string][]byte{
			"tdc_darwin_arm64.tar.gz": []byte("not downloaded by check"),
		},
	})
	t.Setenv("TDC_RELEASE_API_BASE_URL", server.URL)

	result, err := Check(context.Background(), version.Info{
		Version:       "0.1.0",
		OS:            "darwin",
		Arch:          "arm64",
		InstallSource: "archive",
	}, CheckOptions{})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "0.2.0" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ArtifactName != "tdc_darwin_arm64.tar.gz" {
		t.Fatalf("unexpected artifact name %q", result.ArtifactName)
	}
}

func TestApplyDryRunPlansOwnedInstall(t *testing.T) {
	server := fakeReleaseServer(t, releaseFixture{
		tag: "v0.2.0",
		assets: map[string][]byte{
			"tdc_linux_amd64.tar.gz": []byte("not downloaded by dry-run"),
		},
	})
	current := filepath.Join(t.TempDir(), "tdc")
	if err := os.WriteFile(current, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(context.Background(), version.Info{
		Version:       "0.1.0",
		OS:            "linux",
		Arch:          "amd64",
		InstallSource: "archive",
	}, ApplyOptions{
		DryRun:            true,
		ReleaseAPIBaseURL: server.URL,
		ExecutablePath:    current,
	})
	if err != nil {
		t.Fatalf("Apply dry-run failed: %v", err)
	}
	if !result.DryRun || result.TargetVersion != "0.2.0" {
		t.Fatalf("unexpected result: %+v", result)
	}
	wantTarget, err := filepath.EvalSymlinks(current)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetPath != wantTarget {
		t.Fatalf("target path: want %q, got %q", wantTarget, result.TargetPath)
	}
}

func TestApplyRefusesUnknownInstall(t *testing.T) {
	current := filepath.Join(t.TempDir(), "tdc")
	if err := os.WriteFile(current, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Apply(context.Background(), version.Info{
		Version:       "0.1.0",
		OS:            "linux",
		Arch:          "amd64",
		InstallSource: "local",
	}, ApplyOptions{
		DryRun:         true,
		ExecutablePath: current,
	})
	if err == nil {
		t.Fatal("expected unknown install to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "not owned by tdc") {
		t.Fatalf("unexpected error %q", got)
	}
}

func TestApplyReplacesOwnedUnixBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("running Windows executables cannot be replaced in-process")
	}

	archiveBytes := tarGzBinary(t, "tdc", "#!/bin/sh\necho 'tdc 0.2.0 (test, now, linux/amd64)'\n")
	server := fakeReleaseServer(t, releaseFixture{
		tag: "v0.2.0",
		assets: map[string][]byte{
			artifactForRuntime(t): archiveBytes,
		},
	})
	current := filepath.Join(t.TempDir(), "tdc")
	if err := os.WriteFile(current, []byte("#!/bin/sh\necho current\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(context.Background(), version.Info{
		Version:       "0.1.0",
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		InstallSource: "archive",
	}, ApplyOptions{
		Yes:               true,
		ReleaseAPIBaseURL: server.URL,
		ExecutablePath:    current,
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Updated {
		t.Fatalf("expected update result, got %+v", result)
	}
	updated, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "0.2.0") {
		t.Fatalf("binary was not replaced:\n%s", string(updated))
	}
}

func TestChecksumForGoReleaserLine(t *testing.T) {
	got, err := checksumFor([]byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  tdc_linux_amd64.tar.gz\n"), "tdc_linux_amd64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected checksum %q", got)
	}
}

type releaseFixture struct {
	tag    string
	assets map[string][]byte
}

func fakeReleaseServer(t *testing.T, fixture releaseFixture) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	checksums := checksumsFor(fixture.assets)
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		writeReleaseJSON(t, w, server.URL, fixture, checksums)
	})
	mux.HandleFunc("/releases/tags/"+fixture.tag, func(w http.ResponseWriter, r *http.Request) {
		writeReleaseJSON(t, w, server.URL, fixture, checksums)
	})
	mux.HandleFunc("/assets/tdc_checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	for name, data := range fixture.assets {
		name := name
		data := data
		mux.HandleFunc("/assets/"+name, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(data)
		})
	}
	t.Cleanup(server.Close)
	return server
}

func writeReleaseJSON(t *testing.T, w http.ResponseWriter, baseURL string, fixture releaseFixture, checksums string) {
	t.Helper()
	type asset struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}
	response := struct {
		TagName string  `json:"tag_name"`
		Name    string  `json:"name"`
		HTMLURL string  `json:"html_url"`
		Assets  []asset `json:"assets"`
	}{
		TagName: fixture.tag,
		Name:    fixture.tag,
		HTMLURL: baseURL + "/release/" + fixture.tag,
	}
	for name := range fixture.assets {
		response.Assets = append(response.Assets, asset{Name: name, BrowserDownloadURL: baseURL + "/assets/" + name})
	}
	response.Assets = append(response.Assets, asset{Name: checksumAssetName, BrowserDownloadURL: baseURL + "/assets/" + checksumAssetName})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Fatal(err)
	}
	_ = checksums
}

func checksumsFor(assets map[string][]byte) string {
	var b strings.Builder
	for name, data := range assets {
		sum := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return b.String()
}

func tarGzBinary(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func artifactForRuntime(t *testing.T) string {
	t.Helper()
	name, err := artifactName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	return name
}
