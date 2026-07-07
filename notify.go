package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type notifier struct {
	config     *configNotify
	httpClient *http.Client
}

func newNotifier(config *configNotify) *notifier {
	if config == nil || (!hasDingTalk(config) && !hasTelegram(config)) {
		return nil
	}

	return &notifier{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *notifier) failureThreshold() int {
	if n == nil || n.config == nil || n.config.FailureThreshold < 1 {
		return 3
	}
	return n.config.FailureThreshold
}

func (n *notifier) cooldown() time.Duration {
	if n == nil || n.config == nil || n.config.CooldownSeconds < 1 {
		return 30 * time.Minute
	}
	return time.Duration(n.config.CooldownSeconds) * time.Second
}

func (n *notifier) notify(ctx context.Context, subject, message string) error {
	if n == nil {
		return nil
	}

	var errs []error
	successes := 0
	if hasDingTalk(n.config) {
		if err := n.notifyDingTalk(ctx, subject, message); err != nil {
			errs = append(errs, fmt.Errorf("dingtalk: %w", err))
		} else {
			successes++
		}
	}
	if hasTelegram(n.config) {
		if err := n.notifyTelegram(ctx, subject, message); err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", err))
		} else {
			successes++
		}
	}
	if successes > 0 {
		return nil
	}

	return errors.Join(errs...)
}

func hasDingTalk(config *configNotify) bool {
	return config != nil && config.DingTalk != nil && config.DingTalk.WebhookUrl != ""
}

func hasTelegram(config *configNotify) bool {
	return config != nil && config.Telegram != nil &&
		config.Telegram.BotToken != "" && config.Telegram.ChatId != ""
}

func (n *notifier) notifyDingTalk(ctx context.Context, subject, message string) error {
	webhookURL, err := n.signedDingTalkWebhookURL()
	if err != nil {
		return err
	}

	body := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": subject + "\n\n" + message,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ErrorCode    int    `json:"errcode"`
		ErrorMessage string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	if result.ErrorCode != 0 {
		return fmt.Errorf("dingtalk errcode=%d errmsg=%s", result.ErrorCode, result.ErrorMessage)
	}

	return nil
}

func (n *notifier) signedDingTalkWebhookURL() (string, error) {
	webhookURL, err := url.Parse(n.config.DingTalk.WebhookUrl)
	if err != nil {
		return "", err
	}

	secret := n.config.DingTalk.Secret
	if secret == "" {
		return webhookURL.String(), nil
	}

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	signText := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signText))
	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	query := webhookURL.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", sign)
	webhookURL.RawQuery = query.Encode()

	return webhookURL.String(), nil
}

func (n *notifier) notifyTelegram(ctx context.Context, subject, message string) error {
	endpoint := n.config.Telegram.ApiEndpoint
	if endpoint == "" {
		endpoint = "https://api.telegram.org"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	apiURL := endpoint + "/bot" + n.config.Telegram.BotToken + "/sendMessage"
	body := map[string]any{
		"chat_id": n.config.Telegram.ChatId,
		"text":    subject + "\n\n" + message,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram ok=false error_code=%d description=%s", result.ErrorCode, result.Description)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func singleLineError(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join(strings.Fields(err.Error()), " ")
}
