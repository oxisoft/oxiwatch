package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

const telegramAPIURL = "https://api.telegram.org/bot%s/sendMessage"

type Telegram struct {
	botToken   string
	chatID     string
	serverName string
	client     *http.Client
}

type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func NewTelegram(botToken, chatID, serverName string) *Telegram {
	return &Telegram{
		botToken:   botToken,
		chatID:     chatID,
		serverName: serverName,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (t *Telegram) SendLoginAlert(event *parser.SSHEvent, country, city string) error {
	location := formatLocation(event.IP, country, city)

	msg := fmt.Sprintf(`🔐 *SSH Login Alert*
🖥️ Server: %s

👤 User: %s
📅 Time: %s
🔓 Method: %s
🌐 IP: %s
📍 Location: %s`,
		escapeMarkdown(t.serverName),
		escapeMarkdown(event.Username),
		event.Timestamp.Format("2006-01-02 15:04:05"),
		event.Method,
		escapeMarkdown(event.IP),
		escapeMarkdown(location),
	)

	return t.send(msg)
}

func (t *Telegram) SendDailyReport(report string) error {
	return t.send(report)
}

func (t *Telegram) SendTestMessage() error {
	msg := fmt.Sprintf(`✅ *OxiWatch Test Message*
🖥️ Server: %s
📅 Time: %s

Connection successful\!`,
		escapeMarkdown(t.serverName),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	return t.send(msg)
}

func (t *Telegram) send(text string) error {
	url := fmt.Sprintf(telegramAPIURL, t.botToken)

	payload := telegramMessage{
		ChatID:    t.chatID,
		Text:      text,
		ParseMode: "MarkdownV2",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram API error: %s (status %d)", result["description"], resp.StatusCode)
	}

	return nil
}

func formatLocation(ip, country, city string) string {
	if country == "" && city == "" {
		return ip
	}
	if city != "" && country != "" {
		return fmt.Sprintf("%s, %s", city, country)
	}
	if country != "" {
		return country
	}
	return city
}

func escapeMarkdown(s string) string {
	chars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := s
	for _, c := range chars {
		result = replaceAll(result, c, "\\"+c)
	}
	return result
}

func replaceAll(s, old, new string) string {
	var result bytes.Buffer
	for i := 0; i < len(s); i++ {
		if string(s[i]) == old {
			result.WriteString(new)
		} else {
			result.WriteByte(s[i])
		}
	}
	return result.String()
}
