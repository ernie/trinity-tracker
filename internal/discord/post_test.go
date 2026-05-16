package discord

import (
	"context"
	"encoding/json"
	"errors"
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

// PostWebhookForID appends ?wait=true and parses the returned
// message id so callers can later edit it.
func TestPostWebhookForID_ReturnsMessageID(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"1234567890","type":0,"content":""}`))
	}))
	defer srv.Close()

	id, err := PostWebhookForID(context.Background(), srv.URL, Embed{Title: "hi"})
	if err != nil {
		t.Fatalf("PostWebhookForID: %v", err)
	}
	if id != "1234567890" {
		t.Errorf("id: got %q, want %q", id, "1234567890")
	}
	if gotQuery != "wait=true" {
		t.Errorf("server saw query %q, want %q", gotQuery, "wait=true")
	}
}

// EditWebhookMessage PATCHes the right URL with the embed payload.
func TestEditWebhookMessage_PatchesCorrectPath(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Embeds []Embed `json:"embeds"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := EditWebhookMessage(context.Background(), srv.URL, "9999", Embed{Title: "updated"}); err != nil {
		t.Fatalf("EditWebhookMessage: %v", err)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method: got %q, want PATCH", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/messages/9999") {
		t.Errorf("path: got %q, want suffix /messages/9999", gotPath)
	}
	if len(gotBody.Embeds) != 1 || gotBody.Embeds[0].Title != "updated" {
		t.Errorf("server saw wrong payload: %+v", gotBody)
	}
}

// 404 from Discord means the message was deleted — surface as
// ErrMessageNotFound so callers can clear their cached id.
func TestEditWebhookMessage_NotFoundReturnsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Unknown Message"}`))
	}))
	defer srv.Close()

	err := EditWebhookMessage(context.Background(), srv.URL, "deleted", Embed{})
	if !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("expected ErrMessageNotFound, got %v", err)
	}
}
