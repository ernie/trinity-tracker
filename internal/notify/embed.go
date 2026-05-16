package notify

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
)

const (
	activeColor   = 0x2ECC71 // green
	inactiveColor = 0x95A5A6 // muted gray
)

// rosterCap caps how many players we list in the active embed's
// roster field. 16 is comfortably more than any Q3 server runs in
// practice and keeps the ANSI block from wrapping in a Discord
// embed cell.
const rosterCap = 16

// buildActiveEmbed renders the "going active" snapshot for s.
func buildActiveEmbed(s *domain.ServerStatus, publicURL string, mapMeta map[string]string) discord.Embed {
	embed := discord.Embed{
		Title:       "🟢  " + serverDisplay(s) + " is active",
		Description: describeMatch(s, mapMeta),
		Color:       activeColor,
		URL:         serverLink(publicURL, s.ServerID),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	humans, bots := splitRoster(s.Players)
	if len(humans) > 0 || len(bots) > 0 {
		embed.Fields = append(embed.Fields, rosterField(humans, bots))
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

func splitRoster(players []domain.PlayerStatus) (humans, bots []domain.PlayerStatus) {
	humans = make([]domain.PlayerStatus, 0, len(players))
	bots = make([]domain.PlayerStatus, 0, len(players))
	for _, p := range players {
		if p.IsBot {
			bots = append(bots, p)
			continue
		}
		humans = append(humans, p)
	}
	sort.SliceStable(humans, func(i, j int) bool { return humans[i].Score > humans[j].Score })
	sort.SliceStable(bots, func(i, j int) bool { return bots[i].Score > bots[j].Score })
	return
}

// rosterField builds the ANSI code-block listing humans first then
// bots. Score column right-padded to 4 chars (typical Q3 scores fit
// in 3 digits + leading minus); names stay Q3-colored via the
// Discord-flavored ANSI translation.
func rosterField(humans, bots []domain.PlayerStatus) discord.Field {
	var b strings.Builder
	b.WriteString("```ansi\n")
	emit := func(p domain.PlayerStatus, suffix string) {
		fmt.Fprintf(&b, "%4d  %s%s\n", p.Score, discord.Q3ToANSIDiscord(p.Name), suffix)
	}
	emitted := 0
	for _, p := range humans {
		if emitted >= rosterCap {
			break
		}
		emit(p, "")
		emitted++
	}
	for _, p := range bots {
		if emitted >= rosterCap {
			break
		}
		emit(p, "  (bot)")
		emitted++
	}
	total := len(humans) + len(bots)
	if total > rosterCap {
		fmt.Fprintf(&b, "… +%d more\n", total-rosterCap)
	}
	b.WriteString("```")
	return discord.Field{Name: "Roster", Value: b.String()}
}
