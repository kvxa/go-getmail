package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go-getmail.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigHandleDelayDefaultsAndOverrides(t *testing.T) {
	path := writeTempConfig(t, `
DeleteSource: false
ArchiveMailbox: Kept
ReconnectDelay: 12
HandleDelay: 7

Accounts:
- Name: inherit
  Source:
    IMAP:
      Server: src.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: dst.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
- Name: override
  HandleDelay: 30
  Source:
    IMAP:
      Server: src.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: dst.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
- Name: immediate
  HandleDelay: 0
  Source:
    IMAP:
      Server: src.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: dst.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
`)

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HandleDelay != 7 {
		t.Fatalf("global HandleDelay=%d", cfg.HandleDelay)
	}
	if cfg.DeleteSource {
		t.Fatal("DeleteSource should be false")
	}
	if len(cfg.Accounts) != 3 {
		t.Fatalf("accounts=%d", len(cfg.Accounts))
	}
	if cfg.Accounts[0].handleDelay != 7 {
		t.Fatalf("inherit handleDelay=%d", cfg.Accounts[0].handleDelay)
	}
	if cfg.Accounts[1].handleDelay != 30 {
		t.Fatalf("override handleDelay=%d", cfg.Accounts[1].handleDelay)
	}
	if cfg.Accounts[2].handleDelay != 0 {
		t.Fatalf("immediate handleDelay=%d", cfg.Accounts[2].handleDelay)
	}
	if cfg.Accounts[0].ArchiveMailbox != "Kept" || cfg.Accounts[0].ReconnectDelay != 12 {
		t.Fatalf("global fields not applied: %+v", cfg.Accounts[0])
	}
}

func TestLoadConfigDefaultHandleDelayIsFive(t *testing.T) {
	path := writeTempConfig(t, `
Accounts:
- Name: only
  Source:
    IMAP:
      Server: src.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: dst.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
`)
	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HandleDelay != 5 {
		t.Fatalf("default HandleDelay=%d want 5", cfg.HandleDelay)
	}
	if cfg.Accounts[0].handleDelay != 5 {
		t.Fatalf("account handleDelay=%d want 5", cfg.Accounts[0].handleDelay)
	}
	if !cfg.DeleteSource {
		t.Fatal("DeleteSource default should be true")
	}
	if cfg.ReconnectDelay != 30 {
		t.Fatalf("ReconnectDelay default=%d", cfg.ReconnectDelay)
	}
}

func TestLoadConfigEnvHandleDelay(t *testing.T) {
	path := writeTempConfig(t, `
Accounts:
- Name: only
  Source:
    IMAP:
      Server: src.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: dst.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
`)
	t.Setenv("HANDLE_DELAY", "11")
	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HandleDelay != 11 {
		t.Fatalf("env HandleDelay=%d", cfg.HandleDelay)
	}
	if cfg.Accounts[0].handleDelay != 11 {
		t.Fatalf("account handleDelay=%d", cfg.Accounts[0].handleDelay)
	}
}

func TestLoadConfigNotifyFromEnv(t *testing.T) {
	path := writeTempConfig(t, `
Accounts:
- Name: only
  Source:
    IMAP:
      Server: src.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: dst.example.com:993
      Username: u
      Password: p
      Mailbox: INBOX
`)
	t.Setenv("DINGTALK_WEBHOOK_URL", "https://example.com/ding")
	t.Setenv("DINGTALK_SECRET", "sec")
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "123")
	t.Setenv("NOTIFY_FAILURE_THRESHOLD", "4")
	t.Setenv("NOTIFY_COOLDOWN_SECONDS", "60")

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notify == nil || cfg.Notify.DingTalk == nil || cfg.Notify.Telegram == nil {
		t.Fatalf("notify not populated: %+v", cfg.Notify)
	}
	if cfg.Notify.FailureThreshold != 4 || cfg.Notify.CooldownSeconds != 60 {
		t.Fatalf("notify thresholds: %+v", cfg.Notify)
	}
	if cfg.Notify.DingTalk.WebhookUrl != "https://example.com/ding" {
		t.Fatal("dingtalk webhook")
	}
	if cfg.Notify.Telegram.BotToken != "token" || cfg.Notify.Telegram.ChatId != "123" {
		t.Fatal("telegram credentials")
	}
}
