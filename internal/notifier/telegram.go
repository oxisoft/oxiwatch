package notifier

import (
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Telegram delivers notifications to a Telegram chat via a bot.
type Telegram struct {
	bot    *tgbotapi.BotAPI
	chatID int64
}

func NewTelegram(botToken, chatID string) (*Telegram, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID %q: %w", chatID, err)
	}

	return &Telegram{bot: bot, chatID: id}, nil
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Send(msg Message) error {
	m := tgbotapi.NewMessage(t.chatID, msg.HTML)
	m.ParseMode = tgbotapi.ModeHTML

	_, err := t.bot.Send(m)
	return err
}
