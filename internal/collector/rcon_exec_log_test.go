package collector

import (
	"testing"
)

func TestRconExecRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMatch bool
		wantAddr  string
		wantGUID  string
		wantCmd   string
	}{
		{
			name:      "human client with guid",
			input:     "RconExec: 71.215.215.232:27964 E452452E5415AE05617C0DA045C3098D status",
			wantMatch: true,
			wantAddr:  "71.215.215.232:27964",
			wantGUID:  "E452452E5415AE05617C0DA045C3098D",
			wantCmd:   "status",
		},
		{
			name:      "external rcon, no matched client",
			input:     "RconExec: 10.0.0.5:54321 - kick badname",
			wantMatch: true,
			wantAddr:  "10.0.0.5:54321",
			wantGUID:  "-",
			wantCmd:   "kick badname",
		},
		{
			name:      "command with spaces and quotes",
			input:     `RconExec: 1.2.3.4:1234 ABCDEF0123 say hello "quoted text"`,
			wantMatch: true,
			wantAddr:  "1.2.3.4:1234",
			wantGUID:  "ABCDEF0123",
			wantCmd:   `say hello "quoted text"`,
		},
		{
			name:      "ipv6 source",
			input:     "RconExec: [2001:db8::1]:27960 - status",
			wantMatch: true,
			wantAddr:  "[2001:db8::1]:27960",
			wantGUID:  "-",
			wantCmd:   "status",
		},
		{
			name:      "missing command field",
			input:     "RconExec: 1.2.3.4:1234 -",
			wantMatch: false,
		},
		{
			name:      "missing guid field",
			input:     "RconExec: 1.2.3.4:1234",
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := rconExecRegex.FindStringSubmatch(tt.input)
			if (m != nil) != tt.wantMatch {
				t.Fatalf("match=%v, want %v", m != nil, tt.wantMatch)
			}
			if !tt.wantMatch {
				return
			}
			if m[1] != tt.wantAddr {
				t.Errorf("addr = %q, want %q", m[1], tt.wantAddr)
			}
			if m[2] != tt.wantGUID {
				t.Errorf("guid = %q, want %q", m[2], tt.wantGUID)
			}
			if m[3] != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", m[3], tt.wantCmd)
			}
		})
	}
}

func TestRconDeniedRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMatch bool
		wantAddr  string
	}{
		{
			name:      "typical denied",
			input:     "RconDenied: 203.0.113.99:51234",
			wantMatch: true,
			wantAddr:  "203.0.113.99:51234",
		},
		{
			name:      "ipv6 denied",
			input:     "RconDenied: [2001:db8::1]:27960",
			wantMatch: true,
			wantAddr:  "[2001:db8::1]:27960",
		},
		{
			name:      "trailing junk rejected",
			input:     "RconDenied: 1.2.3.4:1234 extra",
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := rconDeniedRegex.FindStringSubmatch(tt.input)
			if (m != nil) != tt.wantMatch {
				t.Fatalf("match=%v, want %v", m != nil, tt.wantMatch)
			}
			if !tt.wantMatch {
				return
			}
			if m[1] != tt.wantAddr {
				t.Errorf("addr = %q, want %q", m[1], tt.wantAddr)
			}
		})
	}
}
