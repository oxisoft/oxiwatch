package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMatrixSend(t *testing.T) {
	const (
		roomID = "!abc:example.org"
		token  = "secret-token-123"
	)

	var (
		gotMethod string
		gotPath   string
		gotRawURL string
		gotAuth   string
		gotBody   map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotRawURL = r.RequestURI
		gotAuth = r.Header.Get("Authorization")

		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"event_id":"$x"}`))
	}))
	defer srv.Close()

	m, err := NewMatrix(srv.URL, roomID, token)
	if err != nil {
		t.Fatalf("NewMatrix() error: %v", err)
	}

	msg := message("Subject", "<b>hi</b>\nsecond line")
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}

	// Path must be the spec v3 send endpoint with the escaped room id and a
	// non-empty transaction id.
	prefix := "/_matrix/client/v3/rooms/" + url.PathEscape(roomID) + "/send/m.room.message/"
	if !strings.HasPrefix(gotPath, prefix) {
		t.Errorf("path %q does not start with %q", gotPath, prefix)
	}
	if txn := strings.TrimPrefix(gotPath, prefix); txn == "" || strings.Contains(txn, "/") {
		t.Errorf("transaction id %q in path %q is empty or malformed", txn, gotPath)
	}

	// Authorization header must carry the bearer token.
	if gotAuth != "Bearer "+token {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer "+token)
	}

	// The token must NOT appear anywhere in the URL.
	if strings.Contains(gotRawURL, token) {
		t.Errorf("token leaked into URL: %q", gotRawURL)
	}

	// JSON body assertions.
	if gotBody["msgtype"] != "m.text" {
		t.Errorf("msgtype = %v, want m.text", gotBody["msgtype"])
	}
	if _, ok := gotBody["body"]; !ok {
		t.Errorf("body field missing in payload: %v", gotBody)
	}
	if gotBody["format"] != "org.matrix.custom.html" {
		t.Errorf("format = %v, want org.matrix.custom.html", gotBody["format"])
	}
	formatted, ok := gotBody["formatted_body"].(string)
	if !ok {
		t.Fatalf("formatted_body missing or not a string: %v", gotBody["formatted_body"])
	}
	if !strings.Contains(formatted, "<br>") {
		t.Errorf("formatted_body %q does not contain <br> for newline", formatted)
	}
	if !strings.HasPrefix(formatted, "<hr>") {
		t.Errorf("formatted_body %q should start with <hr> separator", formatted)
	}
}

func TestMatrixSendNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errcode":"M_FORBIDDEN"}`))
	}))
	defer srv.Close()

	m, err := NewMatrix(srv.URL, "!r:s", "tok")
	if err != nil {
		t.Fatalf("NewMatrix() error: %v", err)
	}

	err = m.Send(message("s", "<b>x</b>"))
	if err == nil {
		t.Fatal("Send() error = nil, want error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not mention status 403", err.Error())
	}
}

func TestNewMatrixValidation(t *testing.T) {
	tests := []struct {
		name                      string
		homeserver, roomID, token string
		wantErr                   bool
	}{
		{"valid", "https://h", "!r:s", "tok", false},
		{"missing homeserver", "", "!r:s", "tok", true},
		{"missing room", "https://h", "", "tok", true},
		{"missing token", "https://h", "!r:s", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMatrix(tt.homeserver, tt.roomID, tt.token)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewMatrixTrimsTrailingSlash(t *testing.T) {
	m, err := NewMatrix("https://h/", "!r:s", "tok")
	if err != nil {
		t.Fatalf("NewMatrix() error: %v", err)
	}
	if strings.HasSuffix(m.homeserver, "/") {
		t.Errorf("homeserver %q should have trailing slash trimmed", m.homeserver)
	}
}

func TestMatrixName(t *testing.T) {
	m, err := NewMatrix("https://h", "!r:s", "tok")
	if err != nil {
		t.Fatalf("NewMatrix() error: %v", err)
	}
	if m.Name() != "matrix" {
		t.Errorf("Name() = %q, want matrix", m.Name())
	}
}

func TestMatrixSendDistinctTxnIDs(t *testing.T) {
	const prefix = "/_matrix/client/v3/rooms/%21r:s/send/m.room.message/"
	seen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.EscapedPath()
		if r.Method != http.MethodPut || !strings.HasPrefix(p, prefix) {
			t.Errorf("unexpected request %s %s", r.Method, p)
		}
		seen[strings.TrimPrefix(p, prefix)] = true
		w.Write([]byte(`{"event_id":"$x"}`))
	}))
	defer srv.Close()

	m, err := NewMatrix(srv.URL, "!r:s", "tok")
	if err != nil {
		t.Fatalf("NewMatrix() error: %v", err)
	}
	const n = 5
	for i := 0; i < n; i++ {
		if err := m.Send(message("s", "x")); err != nil {
			t.Fatalf("Send() error: %v", err)
		}
	}
	if len(seen) != n {
		t.Errorf("got %d distinct transaction ids for %d messages: %v", len(seen), n, seen)
	}
	for id := range seen {
		if id == "" {
			t.Error("empty transaction id")
		}
	}
}
