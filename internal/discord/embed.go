// Package discord holds shared types and helpers for posting messages
// to a Discord webhook, for the leaderboard digest and the
// server-activity notifier.
package discord

// Webhook embed shapes, with the fields Trinity uses. The full schema:
// https://discord.com/developers/docs/resources/channel#embed-object

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

// Author renders as a small icon and name line above the title,
// Discord's closest equivalent of a labeled portrait.
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

// Thumbnail is the small image in the embed's top-right corner.
type Thumbnail struct {
	URL string `json:"url"`
}

type WebhookPayload struct {
	Embeds []Embed `json:"embeds"`
}
