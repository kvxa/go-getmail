go-getmail
==========
This Go program is a simple tool to retrieve and forward e-mails between IMAP servers.

[![Build Status](https://travis-ci.org/mback2k/go-getmail.svg?branch=master)](https://travis-ci.org/mback2k/go-getmail)
[![GoDoc](https://godoc.org/github.com/mback2k/go-getmail?status.svg)](https://godoc.org/github.com/mback2k/go-getmail)
[![Go Report Card](https://goreportcard.com/badge/github.com/mback2k/go-getmail)](https://goreportcard.com/report/github.com/mback2k/go-getmail)

Dependencies
------------
Special thanks to [@emersion](https://github.com/emersion) for creating and providing
the following Go libraries that are the main building blocks of this program:

- https://github.com/emersion/go-imap
- https://github.com/emersion/go-imap-idle

Additional dependencies are the following awesome Go libraries:

- https://github.com/spf13/viper

Installation
------------
You basically have two options to install this Go program package:

1. If you have Go installed and configured on your PATH, just do the following go get inside your GOPATH to get the latest version:

```
go get -u github.com/mback2k/go-getmail
```

2. If you do not have Go installed and just want to use a released binary,
then you can just go ahead and download a pre-compiled Linux amd64 binary from the [Github releases](https://github.com/mback2k/go-getmail/releases).

Finally put the go-getmail binary onto your PATH and make sure it is executable.

Configuration
-------------
The following YAML file is an example configuration with one transfer to be handled:

```
DeleteSource: true
ArchiveMailbox: Archive
ReconnectDelay: 30
HandleDelay: 5

Notify:
  FailureThreshold: 3
  CooldownSeconds: 1800
  DingTalk:
    WebhookUrl: https://oapi.dingtalk.com/robot/send?access_token=your-access-token
    Secret: your-signing-secret
  Telegram:
    BotToken: your-bot-token
    ChatId: your-chat-id
    ApiEndpoint: https://api.telegram.org

Accounts:

- Name: Test account
  HandleDelay: 30
  Source:
    IMAP:
      Server: imap-source.example.com:993
      Username: your-imap-source-username
      Password: your-imap-source-username
      Mailbox: your-imap-source-mailbox
  Target:
    IMAP:
      Server: imap-target.example.com:993
      Username: your-imap-target-username
      Password: your-imap-target-username
      Mailbox: your-imap-target-mailbox
```

You can have multiple accounts handled by repeating the `- Name: ...` section.
Set `DeleteSource` to `false` to move messages to `ArchiveMailbox` in the
source mailbox after they have been copied to the target mailbox. These options
can be overridden with environment variables, for example `DELETE_SOURCE=false`
or `ARCHIVE_MAILBOX=Archive` when running in Docker.
`ReconnectDelay` controls how many seconds to wait before reconnecting after an
IMAP IDLE connection closes, and can be overridden with `RECONNECT_DELAY`.
`HandleDelay` waits the given seconds after a mailbox update before copying
messages, so server-side filter rules on the source account can finish first.
The global default is `5` seconds and can be overridden with `HANDLE_DELAY`.
Each account may set its own `HandleDelay`; omit it to inherit the global value,
or set `0` to handle updates immediately. Additional mailbox updates during the
wait reset the timer. Connection failure, recovery, and message handling failure
notifications can be sent to DingTalk and Telegram robots. DingTalk uses
`Notify.DingTalk.WebhookUrl` and `Notify.DingTalk.Secret`. Telegram uses
`Notify.Telegram.BotToken`, `Notify.Telegram.ChatId`, and optional
`Notify.Telegram.ApiEndpoint` for a custom Bot API endpoint. These options can
be overridden with `DINGTALK_WEBHOOK_URL`, `DINGTALK_SECRET`,
`TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `TELEGRAM_API_ENDPOINT`,
`NOTIFY_FAILURE_THRESHOLD`, and `NOTIFY_COOLDOWN_SECONDS`.

Save this file in one of the following locations and run `./go-getmail`:

- /etc/go-getmail/go-getmail.yaml
- $HOME/.go-getmail.yaml
- $PWD/go-getmail.yaml

License
-------
Copyright (C) 2019  Marc Hoersken <info@marc-hoersken.de>

This software is licensed as described in the file LICENSE, which
you should have received as part of this software distribution.

All trademarks are the property of their respective owners.
