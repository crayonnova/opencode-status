package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchFromOpenRouter_FreeOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"minimax/minimax-m3:free","name":"MiniMax M3 (free)","pricing":{"prompt":"0","completion":"0"}},
			{"id":"anthropic/claude-sonnet-4-5","name":"Sonnet 4.5","pricing":{"prompt":"0.000003","completion":"0.000015"}}
		]}`))
	}))
	defer srv.Close()

	f := New("http://example.com", srv.URL)
	got, err := f.FetchFromOpenRouter(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 free model, got %d", len(got))
	}
	if !got[0].IsFree {
		t.Errorf("expected IsFree=true, got false")
	}
	if got[0].ID != "minimax/minimax-m3:free" {
		t.Errorf("wrong id: %s", got[0].ID)
	}
	if got[0].Provider != "minimax" {
		t.Errorf("wrong provider: %s", got[0].Provider)
	}
}

func TestFetchFromOpenRouter_IncludePaid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"a/free:free","name":"Free","pricing":{"prompt":"0","completion":"0"}},
			{"id":"b/paid","name":"Paid","pricing":{"prompt":"0.001","completion":"0.002"}}
		]}`))
	}))
	defer srv.Close()

	f := New("http://example.com", srv.URL)
	got, err := f.FetchFromOpenRouter(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFetchFromModelsDev_FreeByCost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"nvidia":{"models":{
			"nemotron-free":{"name":"Nemotron Free","cost":{"input":0,"output":0}},
			"nemotron-paid":{"name":"Nemotron Paid","cost":{"input":0.001,"output":0.002}}
		}}}`))
	}))
	defer srv.Close()

	f := New(srv.URL, "http://example.com")
	got, err := f.FetchFromModelsDev(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "nvidia/nemotron-free" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestFetchFromOpenRouter_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	f := New("http://example.com", srv.URL)
	_, err := f.FetchFromOpenRouter(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}
