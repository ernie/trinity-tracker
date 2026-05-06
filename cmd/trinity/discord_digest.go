package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// Discord webhook embed shapes — only the fields we use. See
// https://discord.com/developers/docs/resources/channel#embed-object
// for the full schema.

type discordEmbed struct {
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	URL         string            `json:"url,omitempty"`
	Color       int               `json:"color,omitempty"`
	Fields      []discordField    `json:"fields,omitempty"`
	Footer      *discordFooter    `json:"footer,omitempty"`
	Thumbnail   *discordThumbnail `json:"thumbnail,omitempty"`
	Author      *discordAuthor    `json:"author,omitempty"`
}

// discordAuthor renders as a small icon + name line at the very top
// of the embed, above the title. The icon is smaller than a thumbnail
// (~24×24) and sits beside the name rather than above it — closest
// Discord-native equivalent of a labeled portrait.
type discordAuthor struct {
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

// discordThumbnail is the small image that appears in the top-right
// corner of an embed. Used here for the headline player's portrait —
// gives the digest a visual focal point without requiring per-row
// images (which Discord embeds don't support inline).
type discordThumbnail struct {
	URL string `json:"url"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

// digestCategory describes how to render one leaderboard category for
// either the Discord embed (Title with emoji) or the CLI table
// (CLILabel, plain). The registry covers all 12 API categories so
// operators can swap any of them in via discord.digest_categories
// or `trinity leaderboard --category=...`.
//
// Headline is the prepositional phrase used in the author bar when
// this category produces the embed's headlining player, e.g. "most
// frags" → "🥇 NilClass · most frags". Designed to drop in after
// "<name> · " naturally.
type digestCategory struct {
	Title    string // Discord embed field name (with emoji)
	CLILabel string // CLI column header (plain, all-caps)
	Headline string // phrase for the author bar ("most frags", "best K/D", ...)
	Format   func(domain.LeaderboardEntry) string
}

// fmtInt and fmtKD avoid pulling fmt into the closure's capture set
// at every callsite; same effect as inlining but the registry stays
// readable.
func fmtInt(n int64) string  { return fmt.Sprintf("%d", n) }
func fmtKD(r float64) string { return fmt.Sprintf("%.2f", r) }

var digestCategoryRegistry = map[string]digestCategory{
	"frags":        {Title: "🔥 Frags", CLILabel: "FRAGS", Headline: "most frags", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.TotalFrags) }},
	"deaths":       {Title: "💀 Deaths", CLILabel: "DEATHS", Headline: "most deaths", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.TotalDeaths) }},
	"kd_ratio":     {Title: "⚖️ K/D Ratio", CLILabel: "K/D", Headline: "best K/D", Format: func(e domain.LeaderboardEntry) string { return fmtKD(e.KDRatio) }},
	"matches":      {Title: "🎮 Matches", CLILabel: "MATCHES", Headline: "most matches", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.CompletedMatches) }},
	"victories":    {Title: "🏆 Wins", CLILabel: "WINS", Headline: "most wins", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Victories) }},
	"captures":     {Title: "🚩 Captures", CLILabel: "CAPTURES", Headline: "most flag captures", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Captures) }},
	"flag_returns": {Title: "🔁 Flag Returns", CLILabel: "RETURNS", Headline: "most flag returns", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.FlagReturns) }},
	"assists":      {Title: "🤝 Assists", CLILabel: "ASSISTS", Headline: "most assists", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Assists) }},
	"defends":      {Title: "🛡️ Defends", CLILabel: "DEFENDS", Headline: "most defends", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Defends) }},
	"impressives":  {Title: "⚡ Impressives", CLILabel: "IMPRESSIVES", Headline: "most impressives", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Impressives) }},
	"excellents":   {Title: "💎 Excellents", CLILabel: "EXCELLENTS", Headline: "most excellents", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Excellents) }},
	"humiliations": {Title: "😂 Humiliations", CLILabel: "HUMILIATIONS", Headline: "most humiliations", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Humiliations) }},
}

// defaultDigestCategories is the order / selection used when
// discord.digest_categories is unset. Three thematic rows of three;
// Discord's inline-field layout renders this as a 3×3 grid on desktop.
var defaultDigestCategories = []string{
	"frags", "kd_ratio", "victories",
	"captures", "assists", "defends",
	"impressives", "excellents", "humiliations",
}

// trinityEmbedColor is a muted teal, matches the website's accent.
const trinityEmbedColor = 0x2A9D8F

// renderDigestEmbed builds the single Discord embed shown by the
// weekly digest. results is the per-category leaderboard responses
// (key = API category name); categories is the ordered list to render
// (typically defaultDigestCategories or config override). topN caps
// how many entries appear per field. footerURL is embedded as the
// title's hyperlink so clicking the title reproduces the snapshot.
// publicBase (e.g. "https://trinity.run") is the host portraits are
// served from. staticDir is the local filesystem root those portraits
// live under (cfg.Server.StaticDir) — used to skip a 404-bound icon
// URL when the player's custom model isn't on disk. Pass "" for
// publicBase or staticDir to skip the leader callout entirely.
func renderDigestEmbed(results map[string]*domain.LeaderboardResponse, categories []string, topN int, footerURL, publicBase, staticDir string, now time.Time) discordEmbed {
	// embedMinWidthPad is a run of U+2800 BRAILLE PATTERN BLANK
	// characters appended to the footer text. Discord's layout engine
	// reserves the full footer width when computing the embed's
	// minimum size, so a long invisible-but-counted footer string
	// pushes the embed out to its column-max width. The image-as-
	// spacer technique is folklore — Discord ignores image
	// dimensions entirely (see discord-api-docs#5300, discord.js
	// #1468). U+2800 renders as zero visible width across modern
	// Discord clients but still counts for layout, unlike U+3000
	// which can render as a visible gap on some themes. 60 chars
	// is enough to hit max width on desktop without overflowing on
	// narrow mobile viewports.
	const embedMinWidthPad = "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀"

	embed := discordEmbed{
		Title: "This Week's Leaderboard",
		URL:   footerURL,
		Color: trinityEmbedColor,
		Footer: &discordFooter{
			Text: "Generated " + now.UTC().Format("2006-01-02 15:04 UTC") + " " + embedMinWidthPad,
		},
	}

	// Leader callout: the #1 player from the first non-empty
	// configured category gets their name + portrait + the category's
	// headline phrase in the embed's `author` slot. Discord renders
	// this as `[icon] name` at the very top of the embed — closest
	// native equivalent of a labeled portrait. The headline phrase
	// (e.g. "most frags") explains *why* this player is the headline,
	// since Discord doesn't let us style or annotate the icon.
	//
	// Discord's author.name is plain text — no markdown, no ANSI —
	// so we use the player's CleanName (stripped of Q3 color codes).
	// A 🥇 emoji + the headline phrase stand in for the missing
	// formatting: "🥇 NilClass · most frags".
	//
	// Filesystem fallback: if the player's custom model isn't on
	// disk, we point IconURL at the generic silhouette so Discord
	// doesn't render a broken image. The name + profile link still
	// appear, so the leader callout is meaningful even without the
	// player's actual portrait.
	if publicBase != "" {
		for _, cat := range categories {
			r := results[cat]
			if r == nil || len(r.Entries) == 0 {
				continue
			}
			spec, hasSpec := digestCategoryRegistry[cat]
			leader := r.Entries[0].Player
			name := leader.CleanName
			if hasSpec && spec.Headline != "" {
				name = fmt.Sprintf("🥇 %s · %s", leader.CleanName, spec.Headline)
			}
			author := &discordAuthor{Name: name}
			if leader.ID != 0 {
				author.URL = fmt.Sprintf("%s/players/%d",
					strings.TrimSuffix(publicBase, "/"), leader.ID)
			}
			base := strings.TrimSuffix(publicBase, "/")
			author.IconURL = base + "/assets/discord/default-portrait.png"
			if path := portraitPath(leader.Model); path != "" && portraitExists(staticDir, path) {
				author.IconURL = base + path
			}
			embed.Author = author
			break
		}
	}

	// Pull the period bounds from the first response that has them.
	// All responses use the same as_of/period so any will do.
	for _, cat := range categories {
		r := results[cat]
		if r == nil || r.PeriodStart == nil || r.PeriodEnd == nil {
			continue
		}
		embed.Description = fmt.Sprintf("%s · %s → %s",
			humanPeriod(r.Period),
			r.PeriodStart.UTC().Format("2006-01-02 15:04"),
			r.PeriodEnd.UTC().Format("2006-01-02 15:04 UTC"))
		break
	}

	// Layout: 2 visible fields per row at 1/3 embed width each, with
	// invisible 3rd-slot spacers. Combined with the embed-widening
	// image (set below), this gives ~200px per cell — wide enough
	// that 12-char player names + scores don't wrap inside the
	// ```ansi``` block.
	//
	// Mechanics: Discord packs inline fields up to 3/row. After
	// every pair we append a zero-width-space inline field, so the
	// row reads [F, F, S] (S invisible). When the total real-field
	// count is odd, the trailing field would otherwise render
	// full-width on its own row — to keep its width consistent with
	// the others we add TWO trailing spacers instead of one,
	// producing [F, S, S] = 3 inline slots, F at 1/3 width.
	realCount := 0
	for _, cat := range categories {
		spec, ok := digestCategoryRegistry[cat]
		if !ok {
			// Unknown category — config validator should have caught
			// this, but skip gracefully rather than panicking.
			continue
		}
		embed.Fields = append(embed.Fields, discordField{
			Name:   spec.Title,
			Value:  renderCategoryField(results[cat], spec, topN),
			Inline: true,
		})
		realCount++
		if realCount%2 == 0 {
			embed.Fields = append(embed.Fields, blankInlineField())
		}
	}
	if realCount%2 == 1 {
		embed.Fields = append(embed.Fields, blankInlineField())
		embed.Fields = append(embed.Fields, blankInlineField())
	}

	return embed
}

// blankInlineField returns the empty zero-width-space inline field
// used to pad rows in the 2-per-row layout. Pulled into a helper so
// the layout logic reads cleanly and the ZWS character has one
// canonical site.
func blankInlineField() discordField {
	return discordField{Name: "​", Value: "​", Inline: true}
}

// renderCategoryField produces one field's body as a Discord ```ansi
// code block. Empty leaderboards (e.g. no CTF in the period) collapse
// to a single italic placeholder so the embed shape is stable
// week-to-week.
//
// Layout per row (left-aligned name, left-aligned score):
//
//   <rank>. <verif badge> <platform badge> <name>   <value>
//
// Both columns (name and value) are left-aligned at fixed positions
// computed from the widest row in this field. ansiVisibleWidth keeps
// the math honest in the face of ANSI codes and emoji widths.
func renderCategoryField(r *domain.LeaderboardResponse, spec digestCategory, topN int) string {
	if r == nil || len(r.Entries) == 0 {
		return "_(no activity)_"
	}
	n := topN
	if n > len(r.Entries) {
		n = len(r.Entries)
	}

	// First pass: build the prefix (rank + colored name) for each row
	// and find the widest. We pad to that on emit. (Badges were
	// dropped from the embed to buy back name-column width — a
	// 12-char name + 4-cell badge prefix wraps in a 1/3-width Discord
	// cell; without badges we comfortably fit ~16 chars.)
	type rowParts struct {
		prefix string
		value  string
	}
	rows := make([]rowParts, n)
	maxPrefixWidth := 0
	for i := 0; i < n; i++ {
		e := r.Entries[i]
		name := stripVRPrefix(e.Player.Name)
		prefix := fmt.Sprintf("%d. %s", e.Rank, q3ToANSIDiscord(name))
		if w := ansiVisibleWidth(prefix); w > maxPrefixWidth {
			maxPrefixWidth = w
		}
		rows[i] = rowParts{prefix: prefix, value: spec.Format(e)}
	}

	var b strings.Builder
	b.WriteString("```ansi\n")
	for _, row := range rows {
		padding := maxPrefixWidth - ansiVisibleWidth(row.prefix)
		if padding < 0 {
			padding = 0
		}
		// Three-space gutter between the name column and the value
		// column — wider than typical for breathing room since the
		// values are visually different (numbers/decimals).
		fmt.Fprintf(&b, "%s%s   %s\n", row.prefix, strings.Repeat(" ", padding), row.value)
	}
	b.WriteString("```")
	return b.String()
}

// playerVerificationBadge returns a single-cell glyph with an ANSI
// background indicating the player's account status. Always emitted
// (even for unverified) so columns stay aligned across rows.
//
//   admin:      ★ on yellow bg, dark fg
//   verified:   ✓ on green bg, dark fg
//   unverified: ? on dark bg, white fg
func playerVerificationBadge(verified, admin bool) string {
	switch {
	case admin:
		return "\x1b[43m\x1b[30m★\x1b[0m"
	case verified:
		return "\x1b[42m\x1b[30m✓\x1b[0m"
	default:
		return "\x1b[40m\x1b[37m?\x1b[0m"
	}
}

// playerPlatformBadge returns a 2-cell emoji indicating the player's
// platform. Always emitted so the column width stays consistent
// regardless of mix.
//
//   VR:    🥽 (goggles)
//   flat:  🖥️ (desktop computer)
func playerPlatformBadge(isVR bool) string {
	if isVR {
		return "🥽"
	}
	return "🖥️"
}

// stripVRPrefix removes the conventional "[VR] " prefix that bots
// of trinity-engine prepend to VR users' names. We use the platform
// badge instead so the prefix is redundant — and stripping it keeps
// names single-cased and aligned with the website's display.
func stripVRPrefix(name string) string {
	return strings.TrimPrefix(name, "[VR] ")
}

// portraitExists checks whether the URL path's portrait file is
// present on disk. The digest runs on the hub host so we can stat
// the local copy directly — faster than HEADing the public URL,
// and avoids posting a Discord embed with a broken-image icon when
// a player uses a custom model we don't have. Returns true when
// staticDir is empty (skip-the-check fallback).
func portraitExists(staticDir, urlPath string) bool {
	if staticDir == "" {
		return true
	}
	full := filepath.Join(staticDir, filepath.FromSlash(strings.TrimPrefix(urlPath, "/")))
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

// portraitPath turns a Q3 model string ("doom/default", "*james",
// "sarge") into the URL path the website serves portraits at:
// /assets/portraits/{head}/icon_{skin}.png. Mirrors
// web/src/components/PlayerPortrait.tsx::getPortraitPath. Returns ""
// for an empty model — caller falls back to no portrait icon.
func portraitPath(model string) string {
	if model == "" {
		return ""
	}
	// Strip Team Arena asterisk prefix (e.g. "*james" → "james").
	if strings.HasPrefix(model, "*") {
		model = model[1:]
	}
	parts := strings.SplitN(model, "/", 2)
	head := strings.ToLower(parts[0])
	skin := "default"
	if len(parts) == 2 && parts[1] != "" {
		skin = strings.ToLower(parts[1])
	}
	if head == "" {
		return ""
	}
	return fmt.Sprintf("/assets/portraits/%s/icon_%s.png", head, skin)
}

// humanPeriod turns the API's period enum into a phrase suitable for
// the embed description.
func humanPeriod(p string) string {
	switch p {
	case "day":
		return "Past 24 hours"
	case "week":
		return "Past 7 days"
	case "month":
		return "Past 30 days"
	case "year":
		return "Past year"
	case "all":
		return "All time"
	}
	return p
}

// postWebhook sends the embed to a Discord webhook URL. Discord
// returns 204 on success (no body); we treat 2xx as ok and surface
// the body on anything else so cron-emailed errors are diagnostic.
func postWebhook(ctx context.Context, url string, embed discordEmbed) error {
	body, err := json.Marshal(discordWebhookPayload{Embeds: []discordEmbed{embed}})
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "trinity-discord-digest/1.0")

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
