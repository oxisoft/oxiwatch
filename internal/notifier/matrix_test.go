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
		gotPath = r.RequestURI
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

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}

	// Path must contain the path-escaped room id.
	escaped := url.PathEscape(roomID)
	if !strings.Contains(gotPath, escaped) {
		t.Errorf("path %q does not contain escaped room id %q", gotPath, escaped)
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
