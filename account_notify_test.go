package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotifyConnectionFailureThresholdAndCooldown(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	n := newNotifier(&configNotify{
		FailureThreshold: 2,
		CooldownSeconds:  60,
		Telegram: &configNotifyTelegram{
			BotToken:    "TOKEN",
			ChatId:      "1",
			ApiEndpoint: srv.URL,
		},
	})
	n.httpClient = srv.Client()

	c := &fetchConfig{
		Name:           "acc",
		ReconnectDelay: 3,
		ctx:            context.Background(),
		notifier:       n,
	}

	c.notifyConnectionFailure(errString("fail-1"))
	if hits.Load() != 0 || c.alerting {
		t.Fatal("should not notify before threshold")
	}
	c.notifyConnectionFailure(errString("fail-2"))
	if hits.Load() != 1 || !c.alerting {
		t.Fatalf("hits=%d alerting=%v", hits.Load(), c.alerting)
	}
	c.notifyConnectionFailure(errString("fail-3"))
	if hits.Load() != 1 {
		t.Fatal("cooldown should suppress")
	}

	c.notifyConnectionRecovered()
	if c.failureCount != 0 || c.alerting {
		t.Fatal("recovered should reset counters")
	}
	if hits.Load() != 2 {
		t.Fatalf("recover notify hits=%d", hits.Load())
	}

	// no alert history means recover is silent after clean state
	c.notifyConnectionRecovered()
	if hits.Load() != 2 {
		t.Fatal("clean recover should not notify")
	}
}

func TestNotifyMessageHandlingFailureCooldown(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		data, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		if text, _ := body["text"].(string); text == "" {
			t.Fatal("empty text")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	n := newNotifier(&configNotify{
		CooldownSeconds: 60,
		Telegram: &configNotifyTelegram{
			BotToken:    "TOKEN",
			ChatId:      "1",
			ApiEndpoint: srv.URL,
		},
	})
	n.httpClient = srv.Client()
	c := &fetchConfig{
		Name:           "acc",
		ReconnectDelay: 1,
		ctx:            context.Background(),
		notifier:       n,
	}

	c.notifyMessageHandlingFailure(errString("boom"))
	c.notifyMessageHandlingFailure(errString("boom"))
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}

	c.lastHandleWarn = time.Now().Add(-2 * time.Minute)
	c.notifyMessageHandlingFailure(errString("boom"))
	if hits.Load() != 2 {
		t.Fatalf("after cooldown hits=%d", hits.Load())
	}
}

type errString string

func (e errString) Error() string { return string(e) }
