package notify

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
)

const (
	activeColor   = 0x2ECC71 // green
	inactiveColor = 0x95A5A6 // muted gray
)

// Q3 team enum values used by PlayerStatus.Team.
const (
	teamFree      = 0
	teamRed       = 1
	teamBlue      = 2
	teamSpectator = 3
)

// rosterCap caps how many players we list per roster field. 16 is
// comfortably more than any Q3 server runs per side in practice and
// keeps the ANSI block from wrapping in a Discord embed cell.
const rosterCap = 16

// FFA two-column layout: column cell wide enough for a typical Q3
// name plus a " (bot)" suffix without crowding the next column.
const (
	ffaColumns     = 2
	ffaColumnWidth = 18
)

// buildActiveEmbed renders the "going active" snapshot for s.
func buildActiveEmbed(s *domain.ServerStatus, publicURL string, mapMeta map[string]string) discord.Embed {
	embed := discord.Embed{
		Title:       "🟢  " + serverDisplay(s) + " is active",
		Description: describeMatch(s, mapMeta),
		Color:       activeColor,
		URL:         serverLink(publicURL, s.ServerID),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	red, blue, free, specs := bucketRoster(s.Players)
	// Team mode is whenever the engine has put anyone on Red or Blue.
	// Warmup edge case: a TDM server with everyone still on team 0
	// falls into the FFA layout, which is fine — once the match
	// starts, the refresh PATCH will pick up the new buckets.
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

// buildInactiveEmbed renders the "going inactive" notice for s.
// No roster — the server is empty by definition.
func buildInactiveEmbed(s *domain.ServerStatus, publicURL string, mapMeta map[string]string) discord.Embed {
	desc := "Server is empty"
	if s.Map != "" {
		desc = "Empty on " + mapDisplayName(s.Map, mapMeta)
	}
	return discord.Embed{
		Title:       "⚪  " + serverDisplay(s) + " is idle",
		Description: desc,
		Color:       inactiveColor,
		URL:         serverLink(publicURL, s.ServerID),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// serverDisplay renders the canonical identity the UI elsewhere
// also shows, e.g. "local / dm-hub". Documented at
// internal/domain/server.go:6-7.
func serverDisplay(s *domain.ServerStatus) string {
	return s.Source + " / " + s.Key
}

// serverLink points the embed at the server-list page. There is no
// per-server detail page in the UI; the list shows live cards for
// every server so a viewer can find the right one there.
func serverLink(publicURL string, _ int64) string {
	if publicURL == "" {
		return ""
	}
	return publicURL + "/servers"
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

// bucketRoster sorts players by team into red / blue / free / spec
// buckets. Each bucket is then sorted humans-first, name-ascending
// (case-insensitive, Q3-color-stripped) — see sortRosterBucket.
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
		return strings.ToLower(discord.StripQ3Colors(discord.DisplayName(ps[i].Name, ps[i].IsVR))) <
			strings.ToLower(discord.StripQ3Colors(discord.DisplayName(ps[j].Name, ps[j].IsVR)))
	})
}

// teamRosterField renders a single team's column as an inline ANSI
// code block. Red and Blue are both inline so Discord lays them out
// side-by-side; the spectators field that may follow is non-inline
// so it gets its own row underneath.
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

// ffaRosterField packs the FFA roster into a two-column ANSI block.
// Vertical space is the bottleneck on a fuller server, so we trade
// width for height.
func ffaRosterField(ps []domain.PlayerStatus) discord.Field {
	var b strings.Builder
	b.WriteString("```ansi\n")
	writeRosterColumns(&b, ps, ffaColumns, ffaColumnWidth)
	b.WriteString("```")
	return discord.Field{Name: "Players", Value: b.String()}
}

// spectatorsField goes on its own row below the play roster — they
// aren't part of the action and deserve visual separation.
func spectatorsField(ps []domain.PlayerStatus) discord.Field {
	var b strings.Builder
	b.WriteString("```ansi\n")
	writeRosterColumns(&b, ps, ffaColumns, ffaColumnWidth)
	b.WriteString("```")
	return discord.Field{Name: "Spectators", Value: b.String()}
}

// writeRosterLines emits one player per line. Used by team columns
// where the field itself is already narrow (inline).
func writeRosterLines(b *strings.Builder, ps []domain.PlayerStatus) {
	emitted := 0
	for _, p := range ps {
		if emitted >= rosterCap {
			break
		}
		b.WriteString(discord.Q3ToANSIDiscord(discord.DisplayName(p.Name, p.IsVR)))
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

// writeRosterColumns emits players in a fixed-width grid. Padding
// is computed on the color-stripped name length since ANSI escapes
// contribute bytes but no display width.
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
		b.WriteString(discord.Q3ToANSIDiscord(name))
		b.WriteString(suffix)
		endOfRow := (i+1)%cols == 0
		endOfList := i == n-1
		if endOfRow || endOfList {
			b.WriteByte('\n')
			continue
		}
		visible := utf8.RuneCountInString(discord.StripQ3Colors(name)) + utf8.RuneCountInString(suffix)
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
