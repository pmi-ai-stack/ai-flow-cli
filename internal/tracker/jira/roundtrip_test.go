package jira

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
	file   string // empty: no body
}

// fixtureTransport is a private http.RoundTripper standing in for a
// real Jira instance in every test in this package (07-determinism.md:
// tests run offline against testdata/ fixtures). Every request it
// handles must be pre-registered by exact "METHOD requestURI" — an
// unregistered request fails the test immediately, which is what makes
// this the assertion mechanism for "no HTTP call happened" cases
// (transitions_test.go's already-there short-circuit,
// comments_test.go's marker-edit-not-duplicate).
type fixtureTransport struct {
	t         *testing.T
	responses map[string]fixtureResponse
}

func newFixtureTransport(t *testing.T) *fixtureTransport {
	t.Helper()
	return &fixtureTransport{t: t, responses: map[string]fixtureResponse{}}
}

// on registers the response for one "METHOD requestURI" call.
// requestURI is the path plus (if any) the exact query string the
// client sends — every call site in this package builds its own query
// deterministically, so the registered string is not a guess.
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

	return &http.Response{
		StatusCode: resp.status,
		Body:       body,
		Header:     make(http.Header),
	}, nil
}
