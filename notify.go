package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	if config == nil || config.DingTalk == nil || config.DingTalk.WebhookUrl == "" {
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

	webhookURL, err := n.signedWebhookURL()
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

func (n *notifier) signedWebhookURL() (string, error) {
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

func singleLineError(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join(strings.Fields(err.Error()), " ")
}
