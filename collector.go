package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	labels               = []string{"name"}
	stateLabels          = []string{"name", "state"}
	accountState         = prometheus.NewDesc("mail_account_state", "State of mail accounts as numeric value.", labels, nil)
	accountStateInfo     = prometheus.NewDesc("mail_account_state_info", "State of mail accounts labeled by state name.", stateLabels, nil)
	accountMessagesTotal = prometheus.NewDesc("mail_account_messages_total", "Number of processed messages.", labels, nil)
)

var accountStates = []fetchState{
	initialState,
	connectingState,
	connectedState,
	watchingState,
	handlingState,
	shutdownState,
}

// Collector implements a prometheus.Collector.
type Collector struct {
	config *config
}

func NewCollector(config *config) *Collector {
	cc := &Collector{config: config}
	return cc
}

func (cc *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

func (cc *Collector) Collect(ch chan<- prometheus.Metric) {
	for _, c := range cc.config.Accounts {
		ch <- prometheus.MustNewConstMetric(
			accountState,
			prometheus.GaugeValue,
			float64(c.state),
			c.Name,
		)
		for _, state := range accountStates {
			value := 0.0
			if c.state == state {
				value = 1.0
			}
			ch <- prometheus.MustNewConstMetric(
				accountStateInfo,
				prometheus.GaugeValue,
				value,
				c.Name,
				stateName(state),
			)
		}
		ch <- prometheus.MustNewConstMetric(
			accountMessagesTotal,
			prometheus.CounterValue,
			float64(c.total),
			c.Name,
		)
	}
}
