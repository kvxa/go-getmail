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
	"context"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/heroku/rollrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rollbar/rollbar-go"
	"github.com/rollbar/rollbar-go/errors"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Logging != nil && cfg.Logging.Level != "" {
		l, err := log.ParseLevel(cfg.Logging.Level)
		if err != nil {
			log.Fatal(err)
		}
		log.SetLevel(l)
	}

	if cfg.Rollbar != nil && cfg.Rollbar.AccessToken != "" {
		rollbar.SetStackTracer(errors.StackTracer)
		rollrus.SetupLogging(cfg.Rollbar.AccessToken, cfg.Rollbar.Environment)
		defer rollrus.ReportPanic(cfg.Rollbar.AccessToken, cfg.Rollbar.Environment)
		log.Warn("Errors will be reported to rollbar.com!")
	}

	mqttlock := &sync.Mutex{}
	mqttopts := mqtt.NewClientOptions()
	if cfg.Broker != nil {
		if cfg.Broker.ClientID == "" {
			cfg.Broker.ClientID = "go-getmail"
		}
		mqttopts.AddBroker(cfg.Broker.URL)
		mqttopts.SetClientID(cfg.Broker.ClientID)
		mqttopts.SetUsername(cfg.Broker.Username)
		mqttopts.SetPassword(cfg.Broker.Password)
	}

	runtime.GC()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Metrics != nil && cfg.Metrics.ListenAddress != "" {
		mux := newHealthMux(ctx, cfg)
		cc := NewCollector(cfg)
		prometheus.MustRegister(cc)
		mux.Handle("/metrics", promhttp.Handler())
		go func() {
			log.Infof("HTTP metrics/health listening on %s", cfg.Metrics.ListenAddress)
			if err := http.ListenAndServe(cfg.Metrics.ListenAddress, mux); err != nil {
				log.Errorf("HTTP server stopped: %v", err)
			}
		}()
	}

	notify := newNotifier(cfg.Notify)
	var wg sync.WaitGroup
	for _, c := range cfg.Accounts {
		c.ctx = ctx
		c.notifier = notify
		c.mqttopts = mqttopts
		c.mqttlock = mqttlock
		c.log().Infof("%s --> %s", c.Source.Server, c.Target.Server)
		wg.Add(1)
		go func(c *fetchConfig) {
			defer wg.Done()
			if err := c.run(); err != nil && ctx.Err() == nil {
				c.log().Error(err)
			}
		}(c)
	}

	<-ctx.Done()
	log.Info("Shutdown signal received, waiting for accounts to stop")
	stop()
	wg.Wait()
	log.Info("Shutdown complete")
}
