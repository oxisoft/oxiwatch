package notifier

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// defaultTelegramAPI is the public Telegram Bot API endpoint.
const defaultTelegramAPI = "https://api.telegram.org"

// Telegram delivers notifications to a Telegram chat via a bot. OxiWatch only
// ever sends messages, so this is a minimal client for the Bot API sendMessage
// method on net/http instead of a full bot library:
//
//	POST {api}/bot{token}/sendMessage
//	{"chat_id":123,"text":"...","parse_mode":"HTML"}
//
// Unlike the old library it does not call getMe at construction, so the daemon
// starts even when Telegram is unreachable at boot; a bad token shows up as a
// send error (and in `oxiwatch send-test`).
type Telegram struct {
	apiURL string
	token  string
	chatID int64
	client *http.Client
}

func NewTelegram(botToken, chatID string) (*Telegram, error) {
	return newTelegram(defaultTelegramAPI, botToken, chatID)
}

// newTelegram allows tests to point the client at a fake Bot API server.
func newTelegram(apiURL, botToken, chatID string) (*Telegram, error) {
	if botToken == "" {
		return nil, fmt.Errorf("telegram requires a bot token")
	}
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID %q: %w", chatID, err)
	}
	return &Telegram{
		apiURL: strings.TrimRight(apiURL, "/"),
		token:  botToken,
		chatID: id,
		client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (t *Telegram) Name() string { return "telegram" }

// telegramResponse is the envelope every Bot API method returns.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (t *Telegram) Send(msg Message) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":    t.chatID,
		"text":       msg.HTML,
		"parse_mode": "HTML",
	})
	if err != nil {
		return err
	}

	endpoint := t.apiURL + "/bot" + t.token + "/sendMessage"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return t.redact(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// net/http errors quote the full URL, which carries the bot token.
		return t.redact(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return t.redact(fmt.Errorf("telegram: reading response: %w", err))
	}

	var r telegramResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("telegram: HTTP %d, unreadable response: %s",
			resp.StatusCode, clip(string(raw), 200))
	}
	if !r.OK {
		code := r.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		if r.Parameters != nil && r.Parameters.RetryAfter > 0 {
			return fmt.Errorf("telegram: error %d: %s (retry after %ds)",
				code, r.Description, r.Parameters.RetryAfter)
		}
		return fmt.Errorf("telegram: error %d: %s", code, r.Description)
	}
	return nil
}

// redact removes the bot token from an error message so it never reaches the
// journal or a notification about a failed notification.
func (t *Telegram) redact(err error) error {
	s := err.Error()
	if !strings.Contains(s, t.token) {
		return err
	}
	return errors.New("telegram: " + strings.ReplaceAll(s, t.token, "<redacted>"))
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
