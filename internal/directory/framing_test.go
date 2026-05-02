package directory

import "testing"

func TestParseOOB(t *testing.T) {
	cases := []struct {
		name      string
		input     []byte
		wantCmd   string
		wantRest  string
		wantOK    bool
	}{
		{
			name:     "heartbeat with tag",
			input:    []byte(OOBHeader + "heartbeat QuakeArena-1\n"),
			wantCmd:  "heartbeat",
			wantRest: "QuakeArena-1",
			wantOK:   true,
		},
		{
			name:     "infoResponse uses newline as separator",
			input:    []byte(OOBHeader + "infoResponse\n\\challenge\\abc\\protocol\\68"),
			wantCmd:  "infoResponse",
			wantRest: "\\challenge\\abc\\protocol\\68",
			wantOK:   true,
		},
		{
			name:     "getservers with protocol",
			input:    []byte(OOBHeader + "getservers 68 empty full\n"),
			wantCmd:  "getservers",
			wantRest: "68 empty full",
			wantOK:   true,
		},
		{
			name:     "command only, no args",
			input:    []byte(OOBHeader + "ping"),
			wantCmd:  "ping",
			wantRest: "",
			wantOK:   true,
		},
		{
			name:   "missing OOB header",
			input:  []byte("heartbeat QuakeArena-1\n"),
			wantOK: false,
		},
		{
			name:   "too short",
			input:  []byte("\xff\xff"),
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, rest, ok := parseOOB(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if rest != tc.wantRest {
				t.Errorf("rest = %q, want %q", rest, tc.wantRest)
			}
		})
	}
}

func TestParseInfostring(t *testing.T) {
	in := "\\challenge\\abc123\\Protocol\\68\\hostname\\Test Server\\sv_maxclients\\16"
	got := parseInfostring(in)
	if got["challenge"] != "abc123" {
		t.Errorf("challenge = %q", got["challenge"])
	}
	if got["protocol"] != "68" {
		t.Errorf("protocol = %q (key should be lowercased)", got["protocol"])
	}
	if got["hostname"] != "Test Server" {
		t.Errorf("hostname = %q", got["hostname"])
	}
	if got["sv_maxclients"] != "16" {
		t.Errorf("sv_maxclients = %q", got["sv_maxclients"])
	}
}

func TestParseInfostringEmpty(t *testing.T) {
	got := parseInfostring("")
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestFormatGetinfo(t *testing.T) {
	got := string(formatGetinfo("xyz"))
	want := OOBHeader + "getinfo xyz\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
