package jira

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

func TestDownloadAttachments(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/secure/attachment/10001/mock.png", http.StatusOK, "attachment-mock.png").
		on(http.MethodGet, "/secure/attachment/10002/notes.txt", http.StatusOK, "attachment-notes.txt")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)

	issue := &tracker.Issue{
		Key: "TEST-1",
		Attachments: []tracker.Attachment{
			{ID: "10001", Filename: "mock.png", URL: "https://example.atlassian.net/secure/attachment/10001/mock.png"},
			{ID: "10002", Filename: "notes.txt", URL: "https://example.atlassian.net/secure/attachment/10002/notes.txt"},
		},
	}

	dir := t.TempDir()
	written, err := c.DownloadAttachments(context.Background(), issue, dir)
	if err != nil {
		t.Fatalf("DownloadAttachments: %v", err)
	}
	if len(written) != 2 || written[0] != "mock.png" || written[1] != "notes.txt" {
		t.Fatalf("unexpected written filenames: %v", written)
	}

	gotPNG, err := os.ReadFile(filepath.Join(dir, "mock.png"))
	if err != nil {
		t.Fatalf("read written mock.png: %v", err)
	}
	wantPNG, err := os.ReadFile(filepath.Join("testdata", "attachment-mock.png"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if string(gotPNG) != string(wantPNG) {
		t.Fatalf("mock.png bytes mismatch: got %q want %q", gotPNG, wantPNG)
	}
}
