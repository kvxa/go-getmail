package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type healthAccount struct {
	Name     string `json:"name"`
	State    int    `json:"state"`
	StateName string `json:"state_name"`
	Messages uint64 `json:"messages_total"`
	Ready    bool   `json:"ready"`
}

type healthResponse struct {
	Status   string          `json:"status"`
	Ready    bool            `json:"ready"`
	Accounts []healthAccount `json:"accounts"`
}

func accountReady(state fetchState) bool {
	return state == watchingState || state == handlingState || state == connectedState
}

func buildHealth(cfg *config) healthResponse {
	accounts := make([]healthAccount, 0, len(cfg.Accounts))
	readyCount := 0
	for _, c := range cfg.Accounts {
		ready := accountReady(c.state)
		if ready {
			readyCount++
		}
		accounts = append(accounts, healthAccount{
			Name:      c.Name,
			State:     int(c.state),
			StateName: stateName(c.state),
			Messages:  c.total,
			Ready:     ready,
		})
	}

	allReady := len(cfg.Accounts) > 0 && readyCount == len(cfg.Accounts)
	status := "ok"
	if !allReady {
		status = "degraded"
	}
	if len(cfg.Accounts) == 0 {
		status = "no_accounts"
	}

	return healthResponse{
		Status:   status,
		Ready:    allReady,
		Accounts: accounts,
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func newHealthMux(ctx context.Context, cfg *config) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "shutting_down",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "shutting_down",
			})
			return
		}
		health := buildHealth(cfg)
		if !health.Ready {
			writeJSON(w, http.StatusServiceUnavailable, health)
			return
		}
		writeJSON(w, http.StatusOK, health)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			health := buildHealth(cfg)
			health.Status = "shutting_down"
			health.Ready = false
			writeJSON(w, http.StatusServiceUnavailable, health)
			return
		}
		health := buildHealth(cfg)
		statusCode := http.StatusOK
		if !health.Ready {
			statusCode = http.StatusServiceUnavailable
		}
		writeJSON(w, statusCode, health)
	})

	return mux
}
