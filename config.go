/*
	go-getmail - Retrieve and forward e-mails between IMAP servers.
	Copyright (C) 2019  Marc Hoersken <info@marc-hoersken.de>

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

type configLogging struct {
	Level string
}

type configMetrics struct {
	ListenAddress string
}

type configRollbar struct {
	AccessToken string
	Environment string
}

type configBroker struct {
	URL      string
	ClientID string
	Username string
	Password string
}

type configNotify struct {
	FailureThreshold int
	CooldownSeconds  int
	DingTalk         *configNotifyDingTalk
	Telegram         *configNotifyTelegram
}

type configNotifyDingTalk struct {
	WebhookUrl string
	Secret     string
}

type configNotifyTelegram struct {
	BotToken    string
	ChatId      string
	ApiEndpoint string
}

type config struct {
	DeleteSource   bool
	ArchiveMailbox string
	ReconnectDelay int
	HandleDelay    int
	Accounts       []*fetchConfig

	Logging *configLogging
	Metrics *configMetrics
	Rollbar *configRollbar
	Notify  *configNotify

	Broker *configBroker
}

func loadConfig() (*config, error) {
	configFile := ""
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	return loadConfigFrom(configFile)
}

func loadConfigFrom(configFile string) (*config, error) {
	vpr := viper.New()
	vpr.SetConfigName("go-getmail")
	vpr.SetDefault("DeleteSource", true)
	vpr.SetDefault("ArchiveMailbox", "Archive")
	vpr.SetDefault("ReconnectDelay", 30)
	vpr.SetDefault("HandleDelay", 5)
	vpr.SetDefault("Notify.FailureThreshold", 3)
	vpr.SetDefault("Notify.CooldownSeconds", 1800)
	vpr.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vpr.AutomaticEnv()
	vpr.BindEnv("DeleteSource", "DELETE_SOURCE")
	vpr.BindEnv("ArchiveMailbox", "ARCHIVE_MAILBOX")
	vpr.BindEnv("ReconnectDelay", "RECONNECT_DELAY")
	vpr.BindEnv("HandleDelay", "HANDLE_DELAY")
	vpr.BindEnv("Notify.FailureThreshold", "NOTIFY_FAILURE_THRESHOLD")
	vpr.BindEnv("Notify.CooldownSeconds", "NOTIFY_COOLDOWN_SECONDS")
	vpr.BindEnv("Notify.DingTalk.WebhookUrl", "DINGTALK_WEBHOOK_URL")
	vpr.BindEnv("Notify.DingTalk.Secret", "DINGTALK_SECRET")
	vpr.BindEnv("Notify.Telegram.BotToken", "TELEGRAM_BOT_TOKEN")
	vpr.BindEnv("Notify.Telegram.ChatId", "TELEGRAM_CHAT_ID")
	vpr.BindEnv("Notify.Telegram.ApiEndpoint", "TELEGRAM_API_ENDPOINT")
	vpr.AddConfigPath("/etc/go-getmail/")
	vpr.AddConfigPath("$HOME/.go-getmail")
	vpr.AddConfigPath(".")
	if configFile != "" {
		vpr.SetConfigFile(configFile)
	}

	err := vpr.ReadInConfig()
	if err != nil {
		return nil, err
	}

	var cfg config
	err = vpr.UnmarshalExact(&cfg)
	if err != nil {
		return nil, err
	}
	normalizeNotifyConfig(vpr, &cfg)
	for _, account := range cfg.Accounts {
		account.DeleteSource = cfg.DeleteSource
		account.ArchiveMailbox = cfg.ArchiveMailbox
		account.ReconnectDelay = cfg.ReconnectDelay
		if account.HandleDelay != nil {
			account.handleDelay = *account.HandleDelay
		} else {
			account.handleDelay = cfg.HandleDelay
		}
	}
	return &cfg, nil
}

func normalizeNotifyConfig(vpr *viper.Viper, cfg *config) {
	dingTalkWebhook := vpr.GetString("Notify.DingTalk.WebhookUrl")
	dingTalkSecret := vpr.GetString("Notify.DingTalk.Secret")
	telegramBotToken := vpr.GetString("Notify.Telegram.BotToken")
	telegramChatId := vpr.GetString("Notify.Telegram.ChatId")
	telegramApiEndpoint := vpr.GetString("Notify.Telegram.ApiEndpoint")

	if cfg.Notify == nil &&
		(dingTalkWebhook != "" || dingTalkSecret != "" ||
			telegramBotToken != "" || telegramChatId != "" || telegramApiEndpoint != "") {
		cfg.Notify = &configNotify{}
	}
	if cfg.Notify == nil {
		return
	}

	cfg.Notify.FailureThreshold = vpr.GetInt("Notify.FailureThreshold")
	cfg.Notify.CooldownSeconds = vpr.GetInt("Notify.CooldownSeconds")

	if dingTalkWebhook != "" || dingTalkSecret != "" {
		cfg.Notify.DingTalk = &configNotifyDingTalk{
			WebhookUrl: dingTalkWebhook,
			Secret:     dingTalkSecret,
		}
	}
	if telegramBotToken != "" || telegramChatId != "" || telegramApiEndpoint != "" {
		cfg.Notify.Telegram = &configNotifyTelegram{
			BotToken:    telegramBotToken,
			ChatId:      telegramChatId,
			ApiEndpoint: telegramApiEndpoint,
		}
	}
}
