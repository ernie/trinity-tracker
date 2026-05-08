package assets

import (
	"strings"
	"testing"
)

func TestParseArenaText_OfficialBaseq3Format(t *testing.T) {
	input := `{
map     "q3dm17"
bots    "anarki angel keel"
longname "The Longest Yard"
fraglimit 30
type    "ffa tourney"
}

{
map     "q3dm6"
longname "The Camping Grounds"
type    "single ffa team"
fraglimit 20
}`

	got, err := ParseArenaText(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseArenaText: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Map != "q3dm17" {
		t.Errorf("entry 0 map: want q3dm17, got %q", got[0].Map)
	}
	if got[0].LongName != "The Longest Yard" {
		t.Errorf("entry 0 longname: want %q, got %q", "The Longest Yard", got[0].LongName)
	}
	if got[0].Type != "ffa tourney" {
		t.Errorf("entry 0 type: want %q, got %q", "ffa tourney", got[0].Type)
	}
	if got[0].FragLimit != 30 {
		t.Errorf("entry 0 fraglimit: want 30, got %d", got[0].FragLimit)
	}
	if got[1].Map != "q3dm6" {
		t.Errorf("entry 1 map: want q3dm6, got %q", got[1].Map)
	}
}

func TestParseArenaText_CommunityFormatWithAuthor(t *testing.T) {
	input := `{
map "mIKEctf3"
longname "My iron lung"
type "cctf ctf oneflag harvester overload"
mod "threewave"
author "mIKE"
quote "...3...2...1... Capture the flaaag"
}`

	got, err := ParseArenaText(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseArenaText: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Author != "mIKE" {
		t.Errorf("author: want mIKE, got %q", got[0].Author)
	}
}

func TestParseArenaText_HandlesEmptyAndUnknownKeys(t *testing.T) {
	input := `

{
	map "test1"
	longname "Test One"
	unknown_key "ignored"
}
`
	got, err := ParseArenaText(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Map != "test1" || got[0].LongName != "Test One" {
		t.Fatalf("got: %+v", got)
	}
}
