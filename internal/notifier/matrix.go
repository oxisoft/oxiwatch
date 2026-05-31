package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Matrix delivers notifications to a Matrix room via the client-server API.
// It mirrors the proven request:
//
//	POST {homeserver}/_matrix/client/r0/rooms/{roomID}/send/m.room.message?access_token=...
//	{"msgtype":"m.text","body":"..."}
//
// and additionally sends a formatted (HTML) body so the message renders with
// the same markup as the other channels.
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
	endpoint := fmt.Sprintf("%s/_matrix/client/r0/rooms/%s/send/m.room.message",
		m.homeserver, url.PathEscape(m.roomID))

	payload := map[string]string{
		"msgtype": "m.text",
		"body":    msg.Text,
	}
	if msg.HTML != "" {
		payload["format"] = "org.matrix.custom.html"
		// In HTML a newline is just whitespace, so convert the layout newlines
		// into <br> or the whole message renders on a single line. The
		// plain-text body above keeps its newlines for non-HTML clients.
		payload["formatted_body"] = strings.ReplaceAll(msg.HTML, "\n", "<br>\n")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
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
