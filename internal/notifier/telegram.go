package notifier

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/oxisoft/oxiwatch/internal/parser"
)

type Telegram struct {
	bot        *tgbotapi.BotAPI
	chatID     int64
	serverName string
	serverInfo string
}

func NewTelegram(botToken, chatID, serverName string) (*Telegram, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID %q: %w", chatID, err)
	}

	t := &Telegram{
		bot:        bot,
		chatID:     id,
		serverName: serverName,
	}
	t.serverInfo = t.buildServerInfo()

	return t, nil
}

func (t *Telegram) buildServerInfo() string {
	ipv4 := getPublicIP("https://api.ipify.org")
	ipv6 := getPublicIP("https://api6.ipify.org")

	info := t.serverName
	if ipv4 != "" || ipv6 != "" {
		var ips []string
		if ipv4 != "" {
			ips = append(ips, ipv4)
		}
		if ipv6 != "" {
			ips = append(ips, ipv6)
		}
		info = fmt.Sprintf("%s (%s)", t.serverName, strings.Join(ips, ", "))
	}
	return info
}

func getPublicIP(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(body))
}

func (t *Telegram) SendLoginAlert(event *parser.SSHEvent, country, city string) error {
	location := formatLocation(event.IP, country, city)

	msg := fmt.Sprintf(`🔐 <b>SSH Login Alert</b>
🖥️ Server: %s

👤 User: %s
📅 Time: %s
🔓 Method: %s
🌐 IP: %s
📍 Location: %s`,
		escapeHTML(t.serverInfo),
		escapeHTML(event.Username),
		event.Timestamp.Format("2006-01-02 15:04:05"),
		event.Method,
		escapeHTML(event.IP),
		escapeHTML(location),
	)

	return t.send(msg)
}

func (t *Telegram) SendDailyReport(report string) error {
	return t.send(report)
}

func (t *Telegram) SendTestMessage() error {
	msg := fmt.Sprintf(`✅ <b>OxiWatch Test Message</b>
🖥️ Server: %s
📅 Time: %s

Connection successful!`,
		escapeHTML(t.serverInfo),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	return t.send(msg)
}

func (t *Telegram) SendStartupMessage(version string) error {
	msg := fmt.Sprintf(`🟢 <b>OxiWatch Started</b>
🖥️ Server: %s
📅 Time: %s
📦 Version: %s`,
		escapeHTML(t.serverInfo),
		time.Now().Format("2006-01-02 15:04:05"),
		escapeHTML(version),
	)
	return t.send(msg)
}

func (t *Telegram) SendShutdownMessage() error {
	msg := fmt.Sprintf(`🔴 <b>OxiWatch Stopped</b>
🖥️ Server: %s
📅 Time: %s`,
		escapeHTML(t.serverInfo),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	return t.send(msg)
}

func (t *Telegram) send(text string) error {
	msg := tgbotapi.NewMessage(t.chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML

	_, err := t.bot.Send(msg)
	return err
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

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
