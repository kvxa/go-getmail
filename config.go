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

type config struct {
	DeleteSource   bool
	ArchiveMailbox string
	ReconnectDelay int
	Accounts       []*fetchConfig

	Logging *configLogging
	Metrics *configMetrics
	Rollbar *configRollbar

	Broker *configBroker
}

func loadConfig() (*config, error) {
	vpr := viper.GetViper()
	vpr.SetConfigName("go-getmail")
	vpr.SetDefault("DeleteSource", true)
	vpr.SetDefault("ArchiveMailbox", "Archive")
	vpr.SetDefault("ReconnectDelay", 30)
	vpr.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vpr.AutomaticEnv()
	vpr.BindEnv("DeleteSource", "DELETE_SOURCE")
	vpr.BindEnv("ArchiveMailbox", "ARCHIVE_MAILBOX")
	vpr.BindEnv("ReconnectDelay", "RECONNECT_DELAY")
	vpr.AddConfigPath("/etc/go-getmail/")
	vpr.AddConfigPath("$HOME/.go-getmail")
	vpr.AddConfigPath(".")
	if len(os.Args) > 1 {
		vpr.SetConfigFile(os.Args[1])
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
	for _, account := range cfg.Accounts {
		account.DeleteSource = cfg.DeleteSource
		account.ArchiveMailbox = cfg.ArchiveMailbox
		account.ReconnectDelay = cfg.ReconnectDelay
	}
	return &cfg, nil
}
