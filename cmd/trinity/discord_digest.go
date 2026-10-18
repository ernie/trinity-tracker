package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/q3color"
)

// digestCategory renders one leaderboard category in the Discord embed
// (Title) or the CLI table (CLILabel). Keep the registry in sync with
// internal/api.validCategories and internal/config.validDigestCategories.
// Headline follows "<name> · " in the author bar: "🥇 NilClass · most frags".
type digestCategory struct {
	Title    string // Discord embed field name (with emoji)
	CLILabel string // CLI column header (plain, all-caps)
	Headline string // phrase for the author bar ("most frags", "best K/D", ...)
	Format   func(domain.LeaderboardEntry) string
}

func fmtInt(n int64) string  { return fmt.Sprintf("%d", n) }
func fmtKD(r float64) string { return fmt.Sprintf("%.2f", r) }

var digestCategoryRegistry = map[string]digestCategory{
	"frags":              {Title: "🔥 Frags", CLILabel: "FRAGS", Headline: "most frags", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.TotalFrags) }},
	"deaths":             {Title: "🪦 Deaths", CLILabel: "DEATHS", Headline: "most deaths", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.TotalDeaths) }},
	"kd_ratio":           {Title: "⚖️ K/D ratio", CLILabel: "K/D", Headline: "best K/D", Format: func(e domain.LeaderboardEntry) string { return fmtKD(e.KDRatio) }},
	"matches":            {Title: "🎮 Matches", CLILabel: "MATCHES", Headline: "most matches", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.CompletedMatches) }},
	"victories":          {Title: "🏆 Victories", CLILabel: "VICTORIES", Headline: "most victories", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Victories) }},
	"captures":           {Title: "🚩 Captures", CLILabel: "CAPTURES", Headline: "most flag captures", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Captures) }},
	"flag_returns":       {Title: "🔁 Flag returns", CLILabel: "RETURNS", Headline: "most flag returns", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.FlagReturns) }},
	"assists":            {Title: "🤝 Assists", CLILabel: "ASSISTS", Headline: "most assists", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Assists) }},
	"defends":            {Title: "🛡️ Defense", CLILabel: "DEFENSE", Headline: "most defense medals", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Defends) }},
	"impressives":        {Title: "⚡ Impressives", CLILabel: "IMPRESSIVES", Headline: "most impressives", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Impressives) }},
	"excellents":         {Title: "💎 Excellents", CLILabel: "EXCELLENTS", Headline: "most excellents", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Excellents) }},
	"humiliations":       {Title: "😂 Humiliations", CLILabel: "HUMILIATIONS", Headline: "most humiliations", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.Humiliations) }},
	"skulls_delivered":   {Title: "💀 Skulls delivered", CLILabel: "SKULLS", Headline: "most skulls delivered", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.SkullsDelivered) }},
	"obelisks_destroyed": {Title: "💥 Obelisks destroyed", CLILabel: "OBELISKS", Headline: "most obelisks destroyed", Format: func(e domain.LeaderboardEntry) string { return fmtInt(e.ObelisksDestroyed) }},
}

// defaultDigestCategories follows the leaderboard page's order for all
// game types, paired so each two-column row belongs together.
var defaultDigestCategories = []string{
	"matches", "kd_ratio",
	"frags", "deaths",
	"victories", "humiliations",
	"excellents", "impressives",
	"captures", "flag_returns",
	"assists", "defends",
	"skulls_delivered", "obelisks_destroyed",
}

const trinityEmbedColor = 0x2A9D8F

// renderDigestEmbed builds the digest's one embed. results is keyed by
// API category; topN caps entries per field; footerURL becomes the
// title link, so a click opens the same snapshot. Portraits are served
// from publicBase and checked on disk under staticDir. An empty
// publicBase skips the leader callout.
func renderDigestEmbed(results map[string]*domain.LeaderboardResponse, categories []string, topN int, footerURL, publicBase, staticDir string, now time.Time) discord.Embed {
	// Discord sizes an embed to fit its footer, and U+2800 (braille
	// blank) is invisible but still counts, so 60 of them widen the
	// embed to its desktop maximum while still fitting on mobile.
	const embedMinWidthPad = "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀"

	period := ""
	for _, cat := range categories {
		if r := results[cat]; r != nil && r.Period != "" {
			period = r.Period
			break
		}
	}

	embed := discord.Embed{
		Title: digestTitle(period),
		URL:   footerURL,
		Color: trinityEmbedColor,
		Footer: &discord.Footer{
			Text: "Generated " + now.UTC().Format("2006-01-02 15:04 UTC") + " " + embedMinWidthPad,
		},
	}

	// The leader of the first non-empty category takes the author slot.
	// author.name is plain text, so it uses CleanName. A custom model
	// missing on disk gets the generic silhouette as its icon.
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
			author := &discord.Author{Name: name}
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

	// Every response shares as_of and period, so any one gives the bounds.
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

	// Discord packs up to three inline fields per row. Each pair gets a
	// blank third field, so every field is a third of the embed wide; an
	// odd last field gets two blanks so it keeps that width.
	realCount := 0
	for _, cat := range categories {
		spec, ok := digestCategoryRegistry[cat]
		if !ok {
			continue
		}
		embed.Fields = append(embed.Fields, discord.Field{
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

// blankInlineField uses zero-width spaces because Discord requires a
// field's name and value to be non-empty.
func blankInlineField() discord.Field {
	return discord.Field{Name: "​", Value: "​", Inline: true}
}

// renderCategoryField renders a field as a Discord ```ansi block of
// "<rank>. <name>   <value>" rows, with the value column aligned on the
// widest visible name. An empty leaderboard gets a placeholder so the
// embed keeps its shape.
func renderCategoryField(r *domain.LeaderboardResponse, spec digestCategory, topN int) string {
	if r == nil || len(r.Entries) == 0 {
		return "_(no activity)_"
	}
	n := topN
	if n > len(r.Entries) {
		n = len(r.Entries)
	}

	type rowParts struct {
		prefix string
		value  string
	}
	rows := make([]rowParts, n)
	maxPrefixWidth := 0
	for i := 0; i < n; i++ {
		e := r.Entries[i]
		name := discord.DisplayName(e.Player.Name, e.Player.IsVR)
		prefix := fmt.Sprintf("%d. %s", e.Rank, q3color.ToANSIDiscord(name))
		if w := q3color.VisibleWidth(prefix); w > maxPrefixWidth {
			maxPrefixWidth = w
		}
		rows[i] = rowParts{prefix: prefix, value: spec.Format(e)}
	}

	var b strings.Builder
	b.WriteString("```ansi\n")
	for _, row := range rows {
		padding := maxPrefixWidth - q3color.VisibleWidth(row.prefix)
		if padding < 0 {
			padding = 0
		}
		fmt.Fprintf(&b, "%s%s   %s\n", row.prefix, strings.Repeat(" ", padding), row.value)
	}
	b.WriteString("```")
	return b.String()
}

// playerVerificationBadge is one cell wide for every state so columns
// stay aligned.
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

// playerPlatformBadge is two cells wide for both platforms so columns
// stay aligned.
func playerPlatformBadge(isVR bool) string {
	if isVR {
		return "🥽"
	}
	return "🖥️"
}

// portraitExists stats the portrait on disk, since the digest runs on
// the hub host, so a custom model we lack doesn't post a broken icon.
// An empty staticDir skips the check.
func portraitExists(staticDir, urlPath string) bool {
	if staticDir == "" {
		return true
	}
	full := filepath.Join(staticDir, filepath.FromSlash(strings.TrimPrefix(urlPath, "/")))
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

// portraitPath maps a Q3 model string ("doom/default", "*james",
// "sarge") to the site's portrait path. Keep it in step with
// getPortraitPath in web/src/components/PlayerPortrait.tsx.
func portraitPath(model string) string {
	if model == "" {
		return ""
	}
	// Team Arena models carry a leading asterisk.
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

func digestTitle(p string) string {
	switch p {
	case "day":
		return "Today's leaderboard"
	case "month":
		return "This month's leaderboard"
	case "year":
		return "This year's leaderboard"
	case "all":
		return "All-time leaderboard"
	}
	return "This week's leaderboard"
}

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
