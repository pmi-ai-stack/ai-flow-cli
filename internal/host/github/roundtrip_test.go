package github

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// fixtureResponse is one recorded HTTP response: a status code and an
// optional testdata file supplying the body.
type fixtureResponse struct {
	status int
	file   string
}

// fixtureTransport is a private http.RoundTripper standing in for a
// real GitHub instance — same shape as internal/tracker/jira's own
// fixtureTransport (07-determinism.md: tests run offline against
// testdata/ fixtures). An unregistered request fails the test
// immediately, which is the mechanism reuse-vs-create tests rely on to
// prove "no POST happened".
type fixtureTransport struct {
	t         *testing.T
	responses map[string]fixtureResponse
}

func newFixtureTransport(t *testing.T) *fixtureTransport {
	t.Helper()
	return &fixtureTransport{t: t, responses: map[string]fixtureResponse{}}
}

func (ft *fixtureTransport) on(method, requestURI string, status int, file string) *fixtureTransport {
	ft.responses[method+" "+requestURI] = fixtureResponse{status: status, file: file}
	return ft
}

func (ft *fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.t.Helper()
	key := req.Method + " " + req.URL.RequestURI()
	resp, ok := ft.responses[key]
	if !ok {
		ft.t.Fatalf("fixtureTransport: unexpected request %s", key)
		return nil, nil
	}

	body := io.NopCloser(bytes.NewReader(nil))
	if resp.file != "" {
		data, err := os.ReadFile(filepath.Join("testdata", resp.file))
		if err != nil {
			ft.t.Fatalf("fixtureTransport: read testdata/%s: %v", resp.file, err)
		}
		body = io.NopCloser(bytes.NewReader(data))
	}
	return &http.Response{StatusCode: resp.status, Body: body, Header: make(http.Header)}, nil
}
