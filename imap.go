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
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"

	imap "github.com/emersion/go-imap"
	idle "github.com/emersion/go-imap-idle"
	client "github.com/emersion/go-imap/client"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"

	"github.com/mback2k/go-modernauth"
	"github.com/mback2k/go-modernauth/hassmqtt"
)

// FetchServer contains the IMAP credentials.
type FetchServer struct {
	Server   string
	Username string
	Password string
	Provider string
	Mailbox  string

	config   *fetchConfig
	imapconn *client.Client
	tokensrc oauth2.TokenSource
}

type fetchSource struct {
	FetchServer `mapstructure:"IMAP"`

	idleconn *client.Client
	idle     *idle.Client
	updates  chan client.Update
}

type fetchTarget struct {
	FetchServer `mapstructure:"IMAP"`
}

type fetchState int

const (
	initialState    = (fetchState)(0 << 0)
	connectingState = (fetchState)(1 << 0)
	connectedState  = (fetchState)(1 << 1)
	watchingState   = (fetchState)(1 << 2)
	handlingState   = (fetchState)(1 << 3)
	shutdownState   = (fetchState)(1 << 4)
)

func stateName(state fetchState) string {
	switch state {
	case initialState:
		return "initial"
	case connectingState:
		return "connecting"
	case connectedState:
		return "connected"
	case watchingState:
		return "watching"
	case handlingState:
		return "handling"
	case shutdownState:
		return "shutdown"
	default:
		return "unknown"
	}
}

func shouldHandleBootstrap(messages uint32) bool {
	return messages > 0
}

type fetchConfig struct {
	Name           string
	DeleteSource   bool
	ArchiveMailbox string
	ReconnectDelay int
	HandleDelay    *int
	Source         fetchSource
	Target         fetchTarget

	state       fetchState
	total       uint64
	ctx         context.Context
	handleDelay int

	notifier        *notifier
	failureCount    int
	alerting        bool
	lastConnectWarn time.Time
	lastHandleWarn  time.Time
	mqttopts        *mqtt.ClientOptions
	mqttlock        *sync.Mutex
}

type messageHandlingError struct {
	err error
}

func (e messageHandlingError) Error() string {
	return e.err.Error()
}

func (e messageHandlingError) Unwrap() error {
	return e.err
}

func (s *FetchServer) open() (*client.Client, error) {
	if s.Provider != "" {
		if s.tokensrc == nil {
			backend := hassmqtt.NewHassMqttAuthBackend(s.config.ctx, s.Username, s.config.mqttopts, s.config.mqttlock)
			s.tokensrc = modernauth.NewDeviceAuthTokenSource(s.config.ctx, s.Provider, backend)
		}
		tok, err := s.tokensrc.Token()
		if err != nil {
			return nil, err
		}
		s.Password = tok.AccessToken
	}
	con, err := client.DialTLS(s.Server, nil)
	if err != nil {
		return nil, err
	}
	if s.Provider != "" {
		err = con.Authenticate(modernauth.NewXoauth2Client(s.Username, s.Password))
	} else {
		err = con.Login(s.Username, s.Password)
	}
	if err != nil {
		return nil, err
	}
	return con, nil
}

func (s *FetchServer) openIMAP() error {
	con, err := s.open()
	if err != nil {
		return err
	}
	s.imapconn = con
	return nil
}

func (s *fetchSource) openIDLE() error {
	con, err := s.open()
	if err != nil {
		return err
	}
	s.idleconn = con
	return nil
}

func (s *FetchServer) selectIMAP() (*client.MailboxUpdate, error) {
	status, err := s.imapconn.Select(s.Mailbox, false)
	update := &client.MailboxUpdate{Mailbox: status}
	return update, err
}

func (s *fetchSource) selectIDLE() (*client.MailboxUpdate, error) {
	status, err := s.idleconn.Select(s.Mailbox, true)
	update := &client.MailboxUpdate{Mailbox: status}
	return update, err
}

func (s *fetchSource) initIDLE() error {
	update, err := s.selectIDLE()
	if err != nil {
		return err
	}
	updates := make(chan client.Update, 32)
	updates <- update

	s.idle = idle.NewClient(s.idleconn)
	s.idleconn.Updates = updates
	s.updates = updates
	return nil
}

func (c *fetchConfig) init() error {
	c.Source.config = c
	c.Target.config = c
	c.state = connectingState
	err := c.Source.openIMAP()
	if err != nil {
		return err
	}
	err = c.Source.closeIMAP()
	if err != nil {
		return err
	}
	err = c.Target.openIMAP()
	if err != nil {
		return err
	}
	err = c.Target.closeIMAP()
	if err != nil {
		return err
	}
	err = c.Source.openIDLE()
	if err != nil {
		return err
	}
	err = c.Source.initIDLE()
	if err != nil {
		return err
	}
	c.state = connectedState
	return err
}

func (s *FetchServer) closeIMAP() error {
	if s.imapconn == nil {
		return nil
	}
	defer func() {
		s.imapconn = nil
	}()
	err := s.imapconn.Logout()
	if err != nil {
		return err
	}
	return nil
}

func (s *fetchSource) closeIDLE() error {
	if s.idleconn == nil {
		return nil
	}
	defer func() {
		s.idleconn = nil
	}()
	err := s.idleconn.Logout()
	if err != nil {
		return err
	}
	return nil
}

func (c *fetchConfig) close() error {
	c.state = shutdownState
	var firstErr error
	if err := c.Source.closeIDLE(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.Source.closeIMAP(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.Target.closeIMAP(); err != nil && firstErr == nil {
		firstErr = err
	}
	c.state = initialState
	return firstErr
}

func (c *fetchConfig) watch() error {
	defer func(c *fetchConfig, s fetchState) {
		c.state = s
	}(c, c.state)
	c.state = watchingState

	c.log().Info("Begin idling")

	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	errors := make(chan error, 1)
	go func() {
		errors <- c.Source.idle.IdleWithFallback(ctx.Done(), 0)
	}()

	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	defer stopTimer()

	scheduleHandle := func() {
		delay := time.Duration(c.handleDelay) * time.Second
		if timer == nil {
			c.log().Infof("Waiting %s before handling", delay)
			timer = time.NewTimer(delay)
			timerC = timer.C
			return
		}
		c.log().Infof("Resetting handle delay to %s", delay)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
	}

	bootstrap := true
	pending := false
	handling := false

	runHandle := func() error {
		for {
			handling = true
			err := c.handle()
			handling = false
			if err != nil {
				return err
			}
			if !pending {
				return nil
			}
			pending = false
			c.log().Info("Pending mailbox update after handle")
			if c.handleDelay > 0 {
				scheduleHandle()
				return nil
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			c.log().Info("Shutdown requested, stopping idle watch")
			return ctx.Err()
		case update := <-c.Source.updates:
			c.log().Infof("New update: %#v", update)
			mailboxUpdate, ok := update.(*client.MailboxUpdate)
			if !ok {
				continue
			}
			if bootstrap {
				bootstrap = false
				var messages uint32
				if mailboxUpdate.Mailbox != nil {
					messages = mailboxUpdate.Mailbox.Messages
				}
				if !shouldHandleBootstrap(messages) {
					c.log().Info("Skipping bootstrap handle for empty mailbox")
					continue
				}
				c.log().Infof("Bootstrap mailbox has %d messages, scheduling handle", messages)
			}
			if handling {
				pending = true
				c.log().Info("Mailbox update during handle, marked pending")
				continue
			}
			if c.handleDelay <= 0 {
				if err := runHandle(); err != nil {
					return messageHandlingError{err: err}
				}
				continue
			}
			scheduleHandle()
		case <-timerC:
			stopTimer()
			if handling {
				pending = true
				continue
			}
			if err := runHandle(); err != nil {
				return messageHandlingError{err: err}
			}
		case err := <-errors:
			c.log().Warnf("Not idling anymore: %v", err)
			return err
		}
	}
}

func (c *fetchConfig) handle() error {
	defer func(c *fetchConfig, s fetchState) {
		c.state = s
	}(c, c.state)
	c.state = handlingState

	c.log().Info("Begin handling")

	err := c.Source.openIMAP()
	if err != nil {
		c.log().Warnf("Source connection failed: %v", err)
		return err
	}
	defer c.Source.closeIMAP()

	err = c.Target.openIMAP()
	if err != nil {
		c.log().Warnf("Target connection failed: %v", err)
		return err
	}
	defer c.Target.closeIMAP()

	messages := make(chan *imap.Message, 100)
	processed := make(chan uint32, 100)

	var g errgroup.Group
	g.Go(func() error {
		return c.Source.fetchMessages(messages)
	})
	g.Go(func() error {
		return c.Target.storeMessages(messages, processed)
	})
	g.Go(func() error {
		return c.Source.cleanMessages(processed)
	})

	err = g.Wait()
	if err != nil {
		c.log().Warnf("Message handling failed: %v", err)
		return err
	}

	if c.DeleteSource {
		err = c.Source.imapconn.Expunge(nil)
		if err != nil {
			c.log().Warnf("Message expunge failed: %v", err)
			return err
		}
	}

	c.log().Info("Message handling finished")
	return nil
}

func (s *fetchSource) fetchMessages(messages chan *imap.Message) error {
	update, err := s.selectIMAP()
	if err != nil {
		close(messages)
		return err
	}

	if update.Mailbox.Messages < 1 {
		close(messages)
		return nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, update.Mailbox.Messages)

	return s.imapconn.Fetch(seqset, []imap.FetchItem{
		"UID", "FLAGS", "INTERNALDATE", "BODY[]"}, messages)
}

func (t *fetchTarget) storeMessages(messages <-chan *imap.Message, processed chan<- uint32) error {
	defer close(processed)

	section, err := imap.ParseBodySectionName("BODY[]")
	if err != nil {
		return err
	}

	update, err := t.selectIMAP()
	if err != nil {
		return err
	}

	for msg := range messages {
		t.config.log().Infof("Handling message: %d", msg.Uid)

		deleted := false
		flags := []string{}
		for _, flag := range msg.Flags {
			switch flag {
			case imap.DeletedFlag:
				deleted = true
			case imap.RecentFlag:
				continue
			case imap.SeenFlag:
				continue
			default:
				flags = append(flags, flag)
			}
		}
		if deleted {
			t.config.log().Infof("Ignoring message: %d", msg.Uid)
			continue
		}

		t.config.log().Infof("Storing message: %d", msg.Uid)

		now := time.Now()
		body := msg.GetBody(section)
		err := t.imapconn.Append(update.Mailbox.Name, flags, now, body)
		if err != nil {
			return err
		}

		t.config.total++
		processed <- msg.Uid
	}

	return nil
}

func (s *fetchSource) cleanMessages(processed <-chan uint32) error {
	seqset := new(imap.SeqSet)
	for uid := range processed {
		seqset.AddNum(uid)
	}

	if seqset.Empty() {
		return nil
	}

	if !s.config.DeleteSource {
		s.config.log().Infof("Archiving source messages to %s", s.config.ArchiveMailbox)
		err := s.imapconn.UidMove(seqset, s.config.ArchiveMailbox)
		if err == nil {
			return nil
		}
		if !isMissingMailboxError(err) {
			return err
		}

		s.config.log().Warnf("Archive mailbox missing, creating %s", s.config.ArchiveMailbox)
		err = s.imapconn.Create(s.config.ArchiveMailbox)
		if err != nil && !isMailboxExistsError(err) {
			return err
		}
		return s.imapconn.UidMove(seqset, s.config.ArchiveMailbox)
	}

	s.config.log().Info("Deleting source messages")
	return s.imapconn.UidStore(seqset, imap.AddFlags,
		[]interface{}{imap.DeletedFlag}, nil)
}

func isMissingMailboxError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "not exist") ||
		strings.Contains(msg, "unknown mailbox") ||
		strings.Contains(msg, "unknown folder")
}

func isMailboxExistsError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "mailbox exists") ||
		strings.Contains(msg, "folder exists")
}

func (c *fetchConfig) run() error {
	defer c.close()

	for {
		select {
		case <-c.ctx.Done():
			c.log().Info("Account stopped")
			return c.ctx.Err()
		default:
		}

		err := c.init()
		if err != nil {
			if c.ctx.Err() != nil {
				return c.ctx.Err()
			}
			c.log().Error(err)
			c.notifyConnectionFailure(err)
			c.close()
			c.waitReconnect()
			continue
		}
		c.notifyConnectionRecovered()

		err = c.watch()
		if err != nil {
			if c.ctx.Err() != nil {
				return c.ctx.Err()
			}
			c.log().Error(err)
			var handlingErr messageHandlingError
			if errors.As(err, &handlingErr) {
				c.notifyMessageHandlingFailure(handlingErr.err)
			}
		}
		c.close()
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		c.waitReconnect()
	}
}

func (c *fetchConfig) waitReconnect() {
	delay := time.Duration(c.ReconnectDelay) * time.Second
	if delay < time.Second {
		delay = time.Second
	}
	c.log().Infof("Reconnecting in %s", delay)

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-c.ctx.Done():
	case <-timer.C:
	}
}

func (c *fetchConfig) log() *log.Entry {
	return log.WithFields(log.Fields{
		"name":       c.Name,
		"state":      c.state,
		"state_name": stateName(c.state),
	})
}
