// Package discord holds shared types and helpers for posting messages
// to a Discord webhook. Used by the weekly leaderboard digest command
// and by the event-driven server-activity notifier.
package discord

// Webhook embed shapes — only the fields we use. See
// https://discord.com/developers/docs/resources/channel#embed-object
// for the full schema.

type Embed struct {
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url,omitempty"`
	Color       int        `json:"color,omitempty"`
	Fields      []Field    `json:"fields,omitempty"`
	Footer      *Footer    `json:"footer,omitempty"`
	Thumbnail   *Thumbnail `json:"thumbnail,omitempty"`
	Author      *Author    `json:"author,omitempty"`
	Timestamp   string     `json:"timestamp,omitempty"`
}

// Author renders as a small icon + name line at the very top of the
// embed, above the title. The icon is smaller than a thumbnail
// (~24×24) and sits beside the name rather than above it — closest
// Discord-native equivalent of a labeled portrait.
type Author struct {
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type Footer struct {
	Text string `json:"text"`
}

// Thumbnail is the small image that appears in the top-right corner
// of an embed.
type Thumbnail struct {
	URL string `json:"url"`
}

type WebhookPayload struct {
	Embeds []Embed `json:"embeds"`
}
