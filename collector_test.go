package main

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCollectorMetrics(t *testing.T) {
	cfg := &config{
		Accounts: []*fetchConfig{
			{Name: "acc", state: watchingState, total: 9},
		},
	}
	reg := prometheus.NewRegistry()
	if err := reg.Register(NewCollector(cfg)); err != nil {
		t.Fatal(err)
	}

	expected := `
# HELP mail_account_messages_total Number of processed messages.
# TYPE mail_account_messages_total counter
mail_account_messages_total{name="acc"} 9
# HELP mail_account_state State of mail accounts as numeric value.
# TYPE mail_account_state gauge
mail_account_state{name="acc"} 4
# HELP mail_account_state_info State of mail accounts labeled by state name.
# TYPE mail_account_state_info gauge
mail_account_state_info{name="acc",state="connected"} 0
mail_account_state_info{name="acc",state="connecting"} 0
mail_account_state_info{name="acc",state="handling"} 0
mail_account_state_info{name="acc",state="initial"} 0
mail_account_state_info{name="acc",state="shutdown"} 0
mail_account_state_info{name="acc",state="watching"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected),
		"mail_account_messages_total",
		"mail_account_state",
		"mail_account_state_info",
	); err != nil {
		t.Fatal(err)
	}
}
