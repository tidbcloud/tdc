package fs

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/authz"
	"golang.org/x/net/webdav"
)

func TestRemoteWebDAVFSReadWriteListStat(t *testing.T) {
	var wrote string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && r.URL.Query().Get("stat") == "1":
			_ = json.NewEncoder(w).Encode(apifs.StatMetadataResponse{IsDir: true})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && r.URL.Query().Get("list") == "1":
			_ = json.NewEncoder(w).Encode(apifs.ListResponse{Entries: []apifs.FileInfo{{Name: "README.md", Size: 5}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md" && r.URL.Query().Get("stat") == "1":
			_ = json.NewEncoder(w).Encode(apifs.StatMetadataResponse{Size: 5, ContentType: "text/plain"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md" && r.URL.RawQuery == "":
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/new.txt" && r.URL.Query().Get("stat") == "1":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/new.txt":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read write body: %v", err)
			}
			wrote = string(body)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	client, err := testService(t.TempDir(), server.URL).dataClient(dataProfile(), authz.FSMount, "test mount")
	if err != nil {
		t.Fatalf("dataClient failed: %v", err)
	}
	fsys := newRemoteWebDAVFS(client, "/workspace", false)
	info, err := fsys.Stat(context.Background(), "/README.md")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Name() != "README.md" || info.Size() != 5 || info.IsDir() {
		t.Fatalf("unexpected info: %#v", info)
	}
	dir, err := fsys.OpenFile(context.Background(), "/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile dir failed: %v", err)
	}
	entries, err := dir.Readdir(-1)
	if err != nil {
		t.Fatalf("Readdir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "README.md" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	file, err := fsys.OpenFile(context.Background(), "/README.md", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile read failed: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected data %q", data)
	}
	props, ok := file.(webdav.DeadPropsHolder)
	if !ok {
		t.Fatal("expected WebDAV file to hold dead properties")
	}
	propName := xml.Name{Space: "urn:tdc-test", Local: "color"}
	_, err = props.Patch([]webdav.Proppatch{{Props: []webdav.Property{{XMLName: propName, InnerXML: []byte("blue")}}}})
	if err != nil {
		t.Fatalf("Patch dead prop failed: %v", err)
	}
	deadProps, err := props.DeadProps()
	if err != nil {
		t.Fatalf("DeadProps failed: %v", err)
	}
	if string(deadProps[propName].InnerXML) != "blue" {
		t.Fatalf("unexpected dead prop value %#v", deadProps[propName])
	}
	writable, err := fsys.OpenFile(context.Background(), "/new.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile write failed: %v", err)
	}
	if _, err := writable.Write([]byte("new data")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if wrote != "new data" {
		t.Fatalf("unexpected write body %q", wrote)
	}
}

func TestRemoteWebDAVFSReadOnlyRejectsWrites(t *testing.T) {
	client, err := testService(t.TempDir(), "https://fs.test").dataClient(dataProfile(), authz.FSMount, "test mount")
	if err != nil {
		t.Fatalf("dataClient failed: %v", err)
	}
	fsys := newRemoteWebDAVFS(client, "/", true)
	if err := fsys.Mkdir(context.Background(), "/newdir", 0o755); !os.IsPermission(err) {
		t.Fatalf("expected permission error, got %v", err)
	}
}
