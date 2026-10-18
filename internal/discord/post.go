package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrMessageNotFound is returned by EditWebhookMessage when Discord
// reports 404 for the target message, usually because someone deleted
// it from the channel. Callers clear their cached message ID.
var ErrMessageNotFound = errors.New("discord: webhook message not found")

// PostWebhook sends one or more embeds to a Discord webhook URL. A
// non-2xx error carries Discord's response body so cron-mailed
// failures say what went wrong. Use PostWebhookForID to edit the
// message later.
func PostWebhook(ctx context.Context, url string, embeds ...Embed) error {
	body, err := json.Marshal(WebhookPayload{Embeds: embeds})
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "trinity-discord/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// PostWebhookForID is PostWebhook with ?wait=true, which makes
// Discord return the created message. It returns the message's id
// for a later EditWebhookMessage.
func PostWebhookForID(ctx context.Context, url string, embed Embed) (string, error) {
	body, err := json.Marshal(WebhookPayload{Embeds: []Embed{embed}})
	if err != nil {
		return "", fmt.Errorf("marshal webhook payload: %w", err)
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url+sep+"wait=true", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "trinity-discord/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("webhook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode webhook response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("webhook response missing id field")
	}
	return parsed.ID, nil
}

// EditWebhookMessage PATCHes a webhook message posted with
// PostWebhookForID. It returns ErrMessageNotFound when the message
// was deleted.
func EditWebhookMessage(ctx context.Context, webhookURL, messageID string, embed Embed) error {
	body, err := json.Marshal(WebhookPayload{Embeds: []Embed{embed}})
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}
	editURL := strings.TrimRight(webhookURL, "/") + "/messages/" + messageID
	req, err := http.NewRequestWithContext(ctx, "PATCH", editURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "trinity-discord/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrMessageNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook edit returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
