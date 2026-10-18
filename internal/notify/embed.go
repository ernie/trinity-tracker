package notify

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/q3color"
)

const (
	activeColor   = 0x2ECC71 // green
	inactiveColor = 0x95A5A6 // muted gray
)

// Team values in PlayerStatus.Team.
const (
	teamFree      = 0
	teamRed       = 1
	teamBlue      = 2
	teamSpectator = 3
)

// rosterCap is more players per side than servers run, and keeps the
// ANSI block from wrapping in a Discord embed cell.
const rosterCap = 16

// A column fits a typical name plus " (bot)".
const (
	ffaColumns     = 2
	ffaColumnWidth = 18
)

// embedMinWidthPad widens the embed to its maximum so the two-column
// roster doesn't wrap, the same U+2800 padding the digest embed uses.
const embedMinWidthPad = "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀"

func buildActiveEmbed(s *domain.ServerStatus, publicURL string, mapMeta map[string]string) discord.Embed {
	title := "🟢  " + serverDisplay(s) + " is active"
	url := serverLink(publicURL, s.ServerID)
	// With a live tap open the title links to the browser player. Each
	// refresh re-checks, so the link reverts when the tap drops.
	if link := liveLink(publicURL, s); link != "" {
		title += " (click to watch)"
		url = link
	}
	embed := discord.Embed{
		Title:       title,
		Description: describeMatch(s, mapMeta) + embedMinWidthPad,
		Color:       activeColor,
		URL:         url,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	red, blue, free, specs := bucketRoster(s.Players)
	// Team layout whenever anyone is on red or blue. A TDM warmup with
	// everyone on team 0 shows the FFA layout until a refresh after the
	// match starts.
	if len(red) > 0 || len(blue) > 0 {
		embed.Fields = append(embed.Fields,
			teamRosterField("Red", red),
			teamRosterField("Blue", blue),
		)
	} else if len(free) > 0 {
		embed.Fields = append(embed.Fields, ffaRosterField(free))
	}
	if len(specs) > 0 {
		embed.Fields = append(embed.Fields, spectatorsField(specs))
	}
	return embed
}

// buildInactiveEmbed has no roster or link, since the server is empty.
func buildInactiveEmbed(s *domain.ServerStatus, _ string, mapMeta map[string]string) discord.Embed {
	desc := "Server is empty"
	if s.Map != "" {
		desc = "Empty on " + mapDisplayName(s.Map, mapMeta)
	}
	return discord.Embed{
		Title:       "⚪  " + serverDisplay(s) + " is idle",
		Description: desc,
		Color:       inactiveColor,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// serverDisplay matches the UI's "<source> / <key>".
func serverDisplay(s *domain.ServerStatus) string {
	return s.Source + " / " + s.Key
}

// serverLink points at the server list, which has a live card for
// every server.
func serverLink(publicURL string, _ int64) string {
	if publicURL == "" {
		return ""
	}
	return publicURL + "/servers"
}

// liveLink is the browser player URL for s, the site's /tv/:source/:key
// route, or "" with no live tap or no public URL. Source and key are
// already limited to [A-Za-z0-9._-], so they need no escaping.
func liveLink(publicURL string, s *domain.ServerStatus) string {
	if publicURL == "" || !s.IsLive {
		return ""
	}
	return publicURL + "/tv/" + s.Source + "/" + s.Key
}

func describeMatch(s *domain.ServerStatus, mapMeta map[string]string) string {
	var parts []string
	if s.Map != "" {
		parts = append(parts, "**Map:** "+mapDisplayName(s.Map, mapMeta))
	}
	if s.GameType != "" {
		parts = append(parts, "**Mode:** "+s.GameType)
	}
	if s.TeamScores != nil {
		parts = append(parts, fmt.Sprintf("**Score:** Red %d – %d Blue", s.TeamScores.RedScore, s.TeamScores.BlueScore))
	}
	humans, bots := countRoster(s.Players)
	parts = append(parts, fmt.Sprintf("**Players:** %s · %s", pluralize(humans, "human"), pluralize(bots, "bot")))
	return strings.Join(parts, "\n")
}

func mapDisplayName(short string, mapMeta map[string]string) string {
	if long, ok := mapMeta[short]; ok && long != "" && long != short {
		return fmt.Sprintf("%s (%s)", long, short)
	}
	return short
}

func pluralize(n int, base string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, base)
	}
	return fmt.Sprintf("%d %ss", n, base)
}

func countRoster(players []domain.PlayerStatus) (humans, bots int) {
	for _, p := range players {
		if p.IsBot {
			bots++
		} else {
			humans++
		}
	}
	return
}

// bucketRoster splits players into red, blue, free and spectator
// buckets, each sorted humans first, then by name.
func bucketRoster(players []domain.PlayerStatus) (red, blue, free, specs []domain.PlayerStatus) {
	for _, p := range players {
		switch p.Team {
		case teamRed:
			red = append(red, p)
		case teamBlue:
			blue = append(blue, p)
		case teamSpectator:
			specs = append(specs, p)
		default: // teamFree (0) and any unexpected value land here.
			free = append(free, p)
		}
	}
	sortRosterBucket(red)
	sortRosterBucket(blue)
	sortRosterBucket(free)
	sortRosterBucket(specs)
	return
}

func sortRosterBucket(ps []domain.PlayerStatus) {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].IsBot != ps[j].IsBot {
			return !ps[i].IsBot
		}
		return strings.ToLower(q3color.Strip(discord.DisplayName(ps[i].Name, ps[i].IsVR))) <
			strings.ToLower(q3color.Strip(discord.DisplayName(ps[j].Name, ps[j].IsVR)))
	})
}

// teamRosterField renders one team as an inline ANSI block, so red and
// blue sit side by side and the spectators field below gets its own row.
func teamRosterField(name string, ps []domain.PlayerStatus) discord.Field {
	var b strings.Builder
	b.WriteString("```ansi\n")
	if len(ps) == 0 {
		b.WriteString("(empty)\n")
	} else {
		writeRosterLines(&b, ps)
	}
	b.WriteString("```")
	return discord.Field{Name: name, Value: b.String(), Inline: true}
}

// ffaRosterField uses two columns, since height runs out first on a
// full server.
func ffaRosterField(ps []domain.PlayerStatus) discord.Field {
	var b strings.Builder
	b.WriteString("```ansi\n")
	writeRosterColumns(&b, ps, ffaColumns, ffaColumnWidth)
	b.WriteString("```")
	return discord.Field{Name: "Players", Value: b.String()}
}

func spectatorsField(ps []domain.PlayerStatus) discord.Field {
	var b strings.Builder
	b.WriteString("```ansi\n")
	writeRosterColumns(&b, ps, ffaColumns, ffaColumnWidth)
	b.WriteString("```")
	return discord.Field{Name: "Spectators", Value: b.String()}
}

func writeRosterLines(b *strings.Builder, ps []domain.PlayerStatus) {
	emitted := 0
	for _, p := range ps {
		if emitted >= rosterCap {
			break
		}
		b.WriteString(q3color.ToANSIDiscord(discord.DisplayName(p.Name, p.IsVR)))
		if p.IsBot {
			b.WriteString("  (bot)")
		}
		b.WriteByte('\n')
		emitted++
	}
	if len(ps) > rosterCap {
		fmt.Fprintf(b, "… +%d more\n", len(ps)-rosterCap)
	}
}

// writeRosterColumns pads by the color-stripped name length, since ANSI
// escapes add bytes but no width.
func writeRosterColumns(b *strings.Builder, ps []domain.PlayerStatus, cols, cellWidth int) {
	n := len(ps)
	if n > rosterCap {
		n = rosterCap
	}
	for i := 0; i < n; i++ {
		name := discord.DisplayName(ps[i].Name, ps[i].IsVR)
		suffix := ""
		if ps[i].IsBot {
			suffix = " (bot)"
		}
		b.WriteString(q3color.ToANSIDiscord(name))
		b.WriteString(suffix)
		endOfRow := (i+1)%cols == 0
		endOfList := i == n-1
		if endOfRow || endOfList {
			b.WriteByte('\n')
			continue
		}
		visible := utf8.RuneCountInString(q3color.Strip(name)) + utf8.RuneCountInString(suffix)
		pad := cellWidth - visible
		if pad < 1 {
			pad = 1
		}
		b.WriteString(strings.Repeat(" ", pad))
	}
	if len(ps) > rosterCap {
		fmt.Fprintf(b, "… +%d more\n", len(ps)-rosterCap)
	}
}
