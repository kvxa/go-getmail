package main

import (
	"fmt"
	"time"
)

func (c *fetchConfig) notifyConnectionFailure(err error) {
	c.failureCount++
	if c.notifier == nil || c.failureCount < c.notifier.failureThreshold() {
		return
	}
	if !c.lastConnectWarn.IsZero() && time.Since(c.lastConnectWarn) < c.notifier.cooldown() {
		return
	}

	subject := fmt.Sprintf("%s 连接失败", c.Name)
	message := fmt.Sprintf(
		"账号 %s 连接失败已达 %d 次，将在 %s 后重试。错误信息：%s",
		c.Name,
		c.failureCount,
		c.reconnectDelayText(),
		singleLineError(err),
	)
	if c.sendNotification(subject, message) {
		c.lastConnectWarn = time.Now()
		c.alerting = true
	}
}

func (c *fetchConfig) notifyConnectionRecovered() {
	if c.failureCount == 0 {
		return
	}

	previousFailures := c.failureCount
	shouldNotify := c.alerting
	c.failureCount = 0
	c.alerting = false

	if c.notifier == nil || !shouldNotify {
		return
	}

	subject := fmt.Sprintf("%s 连接已恢复", c.Name)
	message := fmt.Sprintf("账号 %s 连接已恢复，此前连接失败 %d 次。", c.Name, previousFailures)
	if c.sendNotification(subject, message) {
		c.lastConnectWarn = time.Now()
	}
}

func (c *fetchConfig) notifyMessageHandlingFailure(err error) {
	if c.notifier == nil {
		return
	}
	if !c.lastHandleWarn.IsZero() && time.Since(c.lastHandleWarn) < c.notifier.cooldown() {
		return
	}

	subject := fmt.Sprintf("%s 邮件处理失败", c.Name)
	message := fmt.Sprintf(
		"账号 %s 邮件处理失败，将在 %s 后重试。错误信息：%s",
		c.Name,
		c.reconnectDelayText(),
		singleLineError(err),
	)
	if c.sendNotification(subject, message) {
		c.lastHandleWarn = time.Now()
	}
}

func (c *fetchConfig) sendNotification(subject, message string) bool {
	if err := c.notifier.notify(c.ctx, subject, message); err != nil {
		c.log().Warnf("Notification failed: %v", err)
		return false
	}
	return true
}

func (c *fetchConfig) reconnectDelayText() string {
	delay := time.Duration(c.ReconnectDelay) * time.Second
	if delay < time.Second {
		delay = time.Second
	}
	return delay.String()
}
