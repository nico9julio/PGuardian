package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type DetectedTelegramChat struct {
	ChatID string `json:"chatId"`
	Title  string `json:"title"`
	Type   string `json:"type"`
}

type TelegramClient struct {
	httpClient *http.Client
}

func NewTelegramClient() *TelegramClient {
	return &TelegramClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          4,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 12 * time.Second,
			},
		},
	}
}

func (t *TelegramClient) SendMessage(ctx context.Context, botToken, chatID, htmlText string) error {
	if strings.TrimSpace(botToken) == "" || strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("missing telegram bot token or chat ID")
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", strings.TrimSpace(botToken))
	payload := map[string]any{
		"chat_id":                  strings.TrimSpace(chatID),
		"text":                     htmlText,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("telegram API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	return nil
}

// DetectChats calls getUpdates so users who just messaged their bot can pick their Chat ID in 1 click.
func (t *TelegramClient) DetectChats(ctx context.Context, botToken string) ([]DetectedTelegramChat, error) {
	if strings.TrimSpace(botToken) == "" {
		return nil, fmt.Errorf("bot token is required")
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=30", strings.TrimSpace(botToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram getUpdates returned HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		OK     bool `json:"ok"`
		Result []struct {
			Message *struct {
				Chat struct {
					ID        int64  `json:"id"`
					Type      string `json:"type"`
					Title     string `json:"title"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
					LastName  string `json:"last_name"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&parsed); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var chats []DetectedTelegramChat
	for _, item := range parsed.Result {
		if item.Message == nil {
			continue
		}
		c := item.Message.Chat
		idStr := strconv.FormatInt(c.ID, 10)
		if seen[idStr] {
			continue
		}
		seen[idStr] = true
		label := strings.TrimSpace(c.Title)
		if label == "" {
			label = strings.TrimSpace(c.FirstName + " " + c.LastName)
		}
		if c.Username != "" {
			label += " (@" + c.Username + ")"
		}
		chats = append(chats, DetectedTelegramChat{
			ChatID: idStr,
			Title:  strings.TrimSpace(label),
			Type:   c.Type,
		})
	}
	return chats, nil
}
