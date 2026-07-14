package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewNotifierNilWithoutChannels(t *testing.T) {
	if newNotifier(nil) != nil {
		t.Fatal("nil config")
	}
	if newNotifier(&configNotify{}) != nil {
		t.Fatal("empty notify")
	}
}

func TestNotifierDefaults(t *testing.T) {
	n := &notifier{config: &configNotify{}}
	if n.failureThreshold() != 3 {
		t.Fatalf("threshold=%d", n.failureThreshold())
	}
	if n.cooldown() != 30*time.Minute {
		t.Fatalf("cooldown=%s", n.cooldown())
	}
	n.config.FailureThreshold = 5
	n.config.CooldownSeconds = 10
	if n.failureThreshold() != 5 || n.cooldown() != 10*time.Second {
		t.Fatal("custom values")
	}
}

func TestNotifyDingTalkAndTelegram(t *testing.T) {
	var dingBody map[string]any
	var tgBody map[string]any
	var dingQuery url.Values

	ding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dingQuery = r.URL.Query()
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &dingBody)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer ding.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/botTOKEN/sendMessage") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &tgBody)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer tg.Close()

	n := newNotifier(&configNotify{
		DingTalk: &configNotifyDingTalk{
			WebhookUrl: ding.URL + "/robot",
			Secret:     "secret",
		},
		Telegram: &configNotifyTelegram{
			BotToken:    "TOKEN",
			ChatId:      "42",
			ApiEndpoint: tg.URL,
		},
	})
	if n == nil {
		t.Fatal("notifier nil")
	}
	n.httpClient = &http.Client{Timeout: 3 * time.Second}

	if err := n.notify(context.Background(), "subj", "body"); err != nil {
		t.Fatal(err)
	}
	if dingBody["msgtype"] != "text" {
		t.Fatalf("ding body=%v", dingBody)
	}
	if dingQuery.Get("timestamp") == "" || dingQuery.Get("sign") == "" {
		t.Fatalf("ding query missing sign: %v", dingQuery)
	}
	if tgBody["chat_id"] != "42" {
		t.Fatalf("tg body=%v", tgBody)
	}
	text, _ := tgBody["text"].(string)
	if !strings.Contains(text, "subj") || !strings.Contains(text, "body") {
		t.Fatalf("tg text=%q", text)
	}
}

func TestNotifyPartialSuccess(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer okSrv.Close()

	n := newNotifier(&configNotify{
		DingTalk: &configNotifyDingTalk{WebhookUrl: "http://127.0.0.1:1/fail"},
		Telegram: &configNotifyTelegram{
			BotToken:    "TOKEN",
			ChatId:      "1",
			ApiEndpoint: okSrv.URL,
		},
	})
	n.httpClient = &http.Client{Timeout: time.Second}
	if err := n.notify(context.Background(), "s", "m"); err != nil {
		t.Fatalf("partial success should return nil: %v", err)
	}
}

func TestSingleLineError(t *testing.T) {
	if singleLineError(nil) != "" {
		t.Fatal("nil")
	}
	err := errors.New("line1\n  line2\tline3")
	if got := singleLineError(err); got != "line1 line2 line3" {
		t.Fatalf("got=%q", got)
	}
}
