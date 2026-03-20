package alert

import (
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
)

type Alerter struct {
	TelegramBotToken string
	TelegramChatID   string
	AlertEmail       string
}

func New(botToken, chatID, email string) *Alerter {
	return &Alerter{
		TelegramBotToken: botToken,
		TelegramChatID:   chatID,
		AlertEmail:       email,
	}
}

func (a *Alerter) Enabled() bool {
	return a.TelegramBotToken != "" || a.AlertEmail != ""
}

func (a *Alerter) NodeOffline(nodeName, ip, pool string) {
	msg := fmt.Sprintf("🔴 节点离线: %s\nIP: %s\n池: %s", nodeName, ip, pool)
	a.send(msg)
}

func (a *Alerter) NodeOnline(nodeName, ip, pool string) {
	msg := fmt.Sprintf("🟢 节点恢复: %s\nIP: %s\n池: %s", nodeName, ip, pool)
	a.send(msg)
}

func (a *Alerter) send(msg string) {
	if a.TelegramBotToken != "" && a.TelegramChatID != "" {
		go a.sendTelegram(msg)
	}
	if a.AlertEmail != "" {
		go a.sendEmail(msg)
	}
}

func (a *Alerter) sendTelegram(msg string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", a.TelegramBotToken)
	values := url.Values{
		"chat_id": {a.TelegramChatID},
		"text":    {msg},
	}
	resp, err := http.PostForm(apiURL, values)
	if err != nil {
		log.Printf("[alert] telegram error: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[alert] telegram status: %d", resp.StatusCode)
	}
}

func (a *Alerter) sendEmail(msg string) {
	// Simple email via local sendmail / SMTP on localhost:25
	subject := "AirPool Alert"
	body := fmt.Sprintf("Subject: %s\r\nFrom: airpool@localhost\r\nTo: %s\r\n\r\n%s",
		subject, a.AlertEmail, msg)
	err := smtp.SendMail("localhost:25", nil, "airpool@localhost",
		[]string{a.AlertEmail}, []byte(body))
	if err != nil {
		// Fall back to log if SMTP not available
		log.Printf("[alert] email to %s failed: %v\n  message: %s", a.AlertEmail, err, strings.ReplaceAll(msg, "\n", " "))
	}
}
