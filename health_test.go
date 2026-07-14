package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	cfg := &config{
		Accounts: []*fetchConfig{
			{Name: "a", state: watchingState, total: 3},
			{Name: "b", state: handlingState, total: 1},
		},
	}
	mux := newHealthMux(context.Background(), cfg)

	t.Run("healthz ok", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("readyz ok when all ready", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var body healthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if !body.Ready || body.Status != "ok" || len(body.Accounts) != 2 {
			t.Fatalf("body=%+v", body)
		}
		if body.Accounts[0].StateName != "watching" || body.Accounts[1].StateName != "handling" {
			t.Fatalf("state names: %+v", body.Accounts)
		}
	})

	t.Run("readyz degraded", func(t *testing.T) {
		cfg.Accounts[1].state = connectingState
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var body healthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Ready || body.Status != "degraded" {
			t.Fatalf("body=%+v", body)
		}
	})

	t.Run("health detail", func(t *testing.T) {
		cfg.Accounts[0].state = watchingState
		cfg.Accounts[1].state = watchingState
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHealthzShuttingDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := &config{Accounts: []*fetchConfig{{Name: "a", state: watchingState}}}
	mux := newHealthMux(ctx, cfg)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuildHealthNoAccounts(t *testing.T) {
	health := buildHealth(&config{})
	if health.Ready || health.Status != "no_accounts" {
		t.Fatalf("%+v", health)
	}
}

func TestAccountReady(t *testing.T) {
	if !accountReady(watchingState) || !accountReady(handlingState) || !accountReady(connectedState) {
		t.Fatal("ready states")
	}
	if accountReady(initialState) || accountReady(connectingState) || accountReady(shutdownState) {
		t.Fatal("not ready states")
	}
}
