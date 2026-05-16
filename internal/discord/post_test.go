package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostWebhook_Success(t *testing.T) {
	var got struct {
		Embeds []Embed `json:"embeds"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected JSON content-type, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("body not JSON: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := PostWebhook(context.Background(), srv.URL, Embed{Title: "test"}); err != nil {
		t.Fatalf("PostWebhook: %v", err)
	}
	if len(got.Embeds) != 1 || got.Embeds[0].Title != "test" {
		t.Errorf("server saw wrong payload: %+v", got)
	}
}

func TestPostWebhook_NonOKSurfaceBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"Invalid Webhook Token"}`))
	}))
	defer srv.Close()

	err := PostWebhook(context.Background(), srv.URL, Embed{})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !strings.Contains(err.Error(), "Invalid Webhook Token") {
		t.Errorf("error %q should surface response body for diagnostics", err)
	}
}
