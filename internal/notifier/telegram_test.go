package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testBotToken = "123456:ABC-secret-token"

// fakeBotAPI is a minimal stand-in for the Telegram Bot API.
type fakeBotAPI struct {
	method, path, contentType string
	body                      map[string]any
	status                    int
	reply                     string
}

func (f *fakeBotAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.method = r.Method
		f.path = r.URL.Path
		f.contentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &f.body)
		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		w.Write([]byte(f.reply))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTelegramSend(t *testing.T) {
	f := &fakeBotAPI{reply: `{"ok":true,"result":{"message_id":7}}`}
	srv := f.server(t)

	tg, err := newTelegram(srv.URL+"/", testBotToken, "-1001234567890")
	if err != nil {
		t.Fatalf("newTelegram() error: %v", err)
	}
	if tg.Name() != "telegram" {
		t.Errorf("Name() = %q", tg.Name())
	}

	msg := message("Subject", "<b>SSH login</b>\nuser <code>oxi</code> &amp; more")
	if err := tg.Send(msg); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if f.method != http.MethodPost {
		t.Errorf("method = %q, want POST", f.method)
	}
	if want := "/bot" + testBotToken + "/sendMessage"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if f.contentType != "application/json" {
		t.Errorf("content-type = %q", f.contentType)
	}
	// JSON numbers decode as float64; the chat id must be sent as a number.
	if id, ok := f.body["chat_id"].(float64); !ok || int64(id) != -1001234567890 {
		t.Errorf("chat_id = %#v, want number -1001234567890", f.body["chat_id"])
	}
	if f.body["text"] != msg.HTML {
		t.Errorf("text = %q, want %q", f.body["text"], msg.HTML)
	}
	if f.body["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %#v, want HTML", f.body["parse_mode"])
	}
}

func TestTelegramSendAPIError(t *testing.T) {
	f := &fakeBotAPI{
		status: http.StatusBadRequest,
		reply:  `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
	}
	tg, _ := newTelegram(f.server(t).URL, testBotToken, "42")

	err := tg.Send(message("s", "hi"))
	if err == nil {
		t.Fatal("Send() error = nil, want API error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("error = %q, want code and description", err)
	}
}

func TestTelegramSendRateLimited(t *testing.T) {
	f := &fakeBotAPI{
		status: http.StatusTooManyRequests,
		reply:  `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 5","parameters":{"retry_after":5}}`,
	}
	tg, _ := newTelegram(f.server(t).URL, testBotToken, "42")

	err := tg.Send(message("s", "hi"))
	if err == nil || !strings.Contains(err.Error(), "retry after 5s") {
		t.Errorf("error = %v, want retry-after hint", err)
	}
}

func TestTelegramSendUnreadableResponse(t *testing.T) {
	f := &fakeBotAPI{status: http.StatusBadGateway, reply: "<html>bad gateway</html>"}
	tg, _ := newTelegram(f.server(t).URL, testBotToken, "42")

	err := tg.Send(message("s", "hi"))
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("error = %v, want HTTP 502", err)
	}
}

func TestTelegramSendErrorHidesToken(t *testing.T) {
	// A closed server makes net/http fail with an error that quotes the URL.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	tg, _ := newTelegram(url, testBotToken, "42")
	err := tg.Send(message("s", "hi"))
	if err == nil {
		t.Fatal("Send() error = nil, want connection error")
	}
	if strings.Contains(err.Error(), testBotToken) {
		t.Errorf("error leaks the bot token: %q", err)
	}
	if !strings.Contains(err.Error(), "<redacted>") {
		t.Errorf("error = %q, want redacted URL", err)
	}
}

func TestNewTelegramValidation(t *testing.T) {
	if _, err := NewTelegram("", "42"); err == nil {
		t.Error("empty token: error = nil")
	}
	if _, err := NewTelegram(testBotToken, "not-a-number"); err == nil {
		t.Error("bad chat id: error = nil")
	}
	tg, err := NewTelegram(testBotToken, "42")
	if err != nil {
		t.Fatalf("NewTelegram() error: %v", err)
	}
	if tg.apiURL != defaultTelegramAPI || tg.chatID != 42 {
		t.Errorf("got apiURL=%q chatID=%d", tg.apiURL, tg.chatID)
	}
}
