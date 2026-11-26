package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// txnCounter makes transaction ids unique even when two messages are sent in
// the same nanosecond.
var txnCounter atomic.Uint64

// newTxnID returns a transaction id unique to this process run. Matrix
// homeservers deduplicate retried PUTs with the same id per access token, so
// a network retry can never post the same alert twice.
func newTxnID() string {
	return fmt.Sprintf("oxiwatch-%d-%d", time.Now().UnixNano(), txnCounter.Add(1))
}

// Matrix delivers notifications to a Matrix room via the client-server API.
// It uses the spec v3 send endpoint with a fresh transaction id per message:
//
//	PUT {homeserver}/_matrix/client/v3/rooms/{roomID}/send/m.room.message/{txnId}
//	Authorization: Bearer <access token>
//	{"msgtype":"m.text","body":"...","format":"org.matrix.custom.html","formatted_body":"..."}
//
// The formatted (HTML) body makes the message render with the same markup as
// the other channels. Events are sent unencrypted, so the room must not have
// end-to-end encryption enabled.
type Matrix struct {
	homeserver string
	roomID     string
	token      string
	client     *http.Client
}

func NewMatrix(homeserver, roomID, token string) (*Matrix, error) {
	if homeserver == "" || roomID == "" || token == "" {
		return nil, fmt.Errorf("matrix requires homeserver, room id and access token")
	}
	return &Matrix{
		homeserver: strings.TrimRight(homeserver, "/"),
		roomID:     roomID,
		token:      token,
		client:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (m *Matrix) Name() string { return "matrix" }

func (m *Matrix) Send(msg Message) error {
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.homeserver, url.PathEscape(m.roomID), url.PathEscape(newTxnID()))

	payload := map[string]string{
		"msgtype": "m.text",
		"body":    msg.Text,
	}
	if msg.HTML != "" {
		payload["format"] = "org.matrix.custom.html"
		// In HTML a newline is just whitespace, so convert the layout newlines
		// into <br> or the whole message renders on a single line. The
		// plain-text body above keeps its newlines for non-HTML clients.
		// Prepend a horizontal rule so consecutive bot messages are clearly
		// separated in clients that group messages from the same sender.
		payload["formatted_body"] = "<hr>\n" + strings.ReplaceAll(msg.HTML, "\n", "<br>\n")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Pass the token as a header, not a URL query param, so it can't leak into
	// error messages, server access logs, or intermediary proxies.
	req.Header.Set("Authorization", "Bearer "+m.token)

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
