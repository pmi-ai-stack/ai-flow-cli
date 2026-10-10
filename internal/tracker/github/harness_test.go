package github

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// fixtureResponse is one recorded HTTP response.
type fixtureResponse struct {
	status int
	file   string // testdata file; empty for no body
	header http.Header
}

// fixtureTransport answers only pre-registered "METHOD requestURI"
// calls — any other request fails the test, which is how "no write
// happened" is asserted — and records each request body it received.
type fixtureTransport struct {
	t         *testing.T
	responses map[string]fixtureResponse
	calls     []string
	bodies    map[string]string
}

func newFixtureTransport(t *testing.T) *fixtureTransport {
	t.Helper()
	return &fixtureTransport{t: t, responses: map[string]fixtureResponse{}, bodies: map[string]string{}}
}

func (ft *fixtureTransport) on(method, requestURI string, status int, file string) *fixtureTransport {
	ft.responses[method+" "+requestURI] = fixtureResponse{status: status, file: file}
	return ft
}

func (ft *fixtureTransport) onWithHeader(method, requestURI string, status int, file string, h http.Header) *fixtureTransport {
	ft.responses[method+" "+requestURI] = fixtureResponse{status: status, file: file, header: h}
	return ft
}

func (ft *fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.t.Helper()
	key := req.Method + " " + req.URL.RequestURI()
	ft.calls = append(ft.calls, key)
	if req.Body != nil {
		data, _ := io.ReadAll(req.Body)
		ft.bodies[key] = string(data)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		ft.t.Errorf("%s: Authorization = %q", key, got)
	}
	resp, ok := ft.responses[key]
	if !ok {
		ft.t.Fatalf("fixtureTransport: unexpected request %s", key)
		return nil, nil
	}
	body := io.NopCloser(bytes.NewReader(nil))
	if resp.file != "" {
		data, err := os.ReadFile(filepath.Join("testdata", resp.file))
		if err != nil {
			ft.t.Fatalf("read testdata/%s: %v", resp.file, err)
		}
		body = io.NopCloser(bytes.NewReader(data))
	}
	h := resp.header
	if h == nil {
		h = make(http.Header)
	}
	return &http.Response{StatusCode: resp.status, Body: body, Header: h}, nil
}

// testTrackerConfig is the github tracker configuration every test
// builds a Client against: key prefix CLI names cli/cli, whose issues
// the recorded fixtures are.
func testTrackerConfig(commentLimit int) config.Tracker {
	return config.Tracker{
		Provider:     config.TrackerGitHub,
		BaseURL:      "https://api.github.com",
		Statuses:     config.TrackerStatuses{Todo: []string{"open", "triage"}, InProgress: "in progress", InReview: "in review"},
		BugTypes:     []string{"Bug", "Defect"},
		Repositories: map[string]string{"CLI": "cli/cli"},
		CommentLimit: commentLimit,
	}
}

func newTestClient(t *testing.T, ft *fixtureTransport, commentLimit int) *Client {
	t.Helper()
	c := New(testTrackerConfig(commentLimit), "test-token", ft, nil)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}
