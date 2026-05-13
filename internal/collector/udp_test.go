package collector

import (
	"strings"
	"testing"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// TestParseObjStatus exercises the gametype-dispatched parser. The
// clientNum=0 cases guard against confusing a real carrier with "none".
func TestParseObjStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		gameType string
		wantObj  *domain.ObjStatus
		wantTail map[int]int
	}{
		{
			name:     "empty input",
			input:    "",
			gameType: "ctf",
			wantObj:  nil,
		},
		{
			name:     "unknown gametype returns nil",
			input:    "0:-1,0:-1",
			gameType: "ffa",
			wantObj:  nil,
		},
		{
			name:     "ctf well-formed (flags at base)",
			input:    "0:-1,0:-1",
			gameType: "ctf",
			wantObj: &domain.ObjStatus{
				Mode:        "ctf",
				Red:         0,
				RedCarrier:  -1,
				Blue:        0,
				BlueCarrier: -1,
			},
		},
		{
			name:     "ctf clientNum=0 carries red flag",
			input:    "2:0,0:-1",
			gameType: "ctf",
			wantObj: &domain.ObjStatus{
				Mode:        "ctf",
				Red:         2,
				RedCarrier:  0,
				Blue:        0,
				BlueCarrier: -1,
			},
		},
		{
			name:     "ctf clientNum=0 carries blue flag",
			input:    "0:-1,2:0",
			gameType: "ctf",
			wantObj: &domain.ObjStatus{
				Mode:        "ctf",
				Red:         0,
				RedCarrier:  -1,
				Blue:        2,
				BlueCarrier: 0,
			},
		},
		{
			name:     "1fctf neutral held by clientNum=0",
			input:    "2:0",
			gameType: "1fctf",
			wantObj: &domain.ObjStatus{
				Mode:           "1fctf",
				Neutral:        2,
				NeutralCarrier: 0,
				RedCarrier:     -1,
				BlueCarrier:    -1,
			},
		},
		{
			name:     "1fctf neutral at base normalizes carrier to -1",
			input:    "0:5",
			gameType: "1fctf",
			wantObj: &domain.ObjStatus{
				Mode:           "1fctf",
				Neutral:        0,
				NeutralCarrier: -1,
				RedCarrier:     -1,
				BlueCarrier:    -1,
			},
		},
		{
			name:     "overload both obelisks alive",
			input:    "2500,2500",
			gameType: "overload",
			wantObj: &domain.ObjStatus{
				Mode:          "overload",
				RedObeliskHP:  2500,
				BlueObeliskHP: 2500,
			},
		},
		{
			name:     "overload destroyed serializes 0",
			input:    "0,1200",
			gameType: "overload",
			wantObj: &domain.ObjStatus{
				Mode:          "overload",
				RedObeliskHP:  0,
				BlueObeliskHP: 1200,
			},
		},
		{
			name:     "harvester scoreboard",
			input:    "7,3",
			gameType: "harvester",
			wantObj: &domain.ObjStatus{
				Mode:       "harvester",
				RedSkulls:  7,
				BlueSkulls: 3,
			},
		},
		{
			name:     "harvester with pipe tail (cumulative + carry)",
			input:    "5,2|3:1,7:2",
			gameType: "harvester",
			wantObj: &domain.ObjStatus{
				Mode:       "harvester",
				RedSkulls:  5,
				BlueSkulls: 2,
			},
			wantTail: map[int]int{3: 1, 7: 2},
		},
		{
			name:     "malformed colon-only segment",
			input:    ":",
			gameType: "ctf",
			wantObj:  nil,
		},
		{
			name:     "malformed alpha segment",
			input:    "abc",
			gameType: "ctf",
			wantObj:  nil,
		},
		{
			name:     "ctf single segment (missing comma)",
			input:    "1:",
			gameType: "ctf",
			wantObj:  nil,
		},
		{
			name:     "ctf trailing extra fragment",
			input:    "1:2,3",
			gameType: "ctf",
			wantObj:  nil,
		},
		{
			name:     "harvester with empty pipe tail",
			input:    "1:2|",
			gameType: "harvester",
			wantObj:  nil, // "1:2" is not a valid harvester team section
		},
		{
			name:     "well-formed overload with malformed tail",
			input:    "1200,1500|abc:def",
			gameType: "overload",
			wantObj: &domain.ObjStatus{
				Mode:          "overload",
				RedObeliskHP:  1200,
				BlueObeliskHP: 1500,
			},
			wantTail: nil, // tail parser drops all malformed entries
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotObj, gotTail := parseObjStatus(tt.input, tt.gameType)
			if !objStatusEqual(gotObj, tt.wantObj) {
				t.Errorf("obj mismatch:\n  got:  %+v\n  want: %+v", gotObj, tt.wantObj)
			}
			if !tailEqual(gotTail, tt.wantTail) {
				t.Errorf("tail mismatch:\n  got:  %v\n  want: %v", gotTail, tt.wantTail)
			}
		})
	}
}

// TestParseClientCountTail directly exercises the pipe-tail parser
// edge cases the dispatcher doesn't cover on its own.
func TestParseClientCountTail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[int]int
	}{
		{name: "empty", input: "", want: nil},
		{name: "single pair", input: "3:1", want: map[int]int{3: 1}},
		{name: "multiple pairs", input: "0:1,2:3,4:5", want: map[int]int{0: 1, 2: 3, 4: 5}},
		{name: "clientNum=0 carries", input: "0:2", want: map[int]int{0: 2}},
		{name: "all-malformed yields nil", input: "abc,def:ghi,::", want: nil},
		{name: "mixed valid + malformed", input: "1:2,bad,3:4", want: map[int]int{1: 2, 3: 4}},
		{name: "trailing comma", input: "1:2,", want: map[int]int{1: 2}},
		{name: "no colon", input: "1234", want: nil},
		{
			name:  "long tail near buffer limit",
			input: buildLongTail(64), // 64 pairs ~ 4-5 bytes each, well under MAX_INFO_VALUE
			want:  expectedLongTail(64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClientCountTail(tt.input)
			if !tailEqual(got, tt.want) {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

// buildLongTail produces "0:0,1:1,...,n-1:n-1" — exercises the parser
// against a realistically packed tail without crossing MAX_INFO_VALUE.
func buildLongTail(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(itoa(i))
		b.WriteByte(':')
		b.WriteString(itoa(i))
	}
	return b.String()
}

func expectedLongTail(n int) map[int]int {
	m := make(map[int]int, n)
	for i := 0; i < n; i++ {
		m[i] = i
	}
	return m
}

// itoa avoids pulling strconv into the helper just for test sugar.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func objStatusEqual(a, b *domain.ObjStatus) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func tailEqual(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
