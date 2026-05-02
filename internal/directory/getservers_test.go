package directory

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestParseGetservers(t *testing.T) {
	cases := []struct {
		in   string
		want queryFilters
	}{
		{
			in:   "68",
			want: queryFilters{protocol: 68},
		},
		{
			in:   "68 empty full",
			want: queryFilters{protocol: 68, includeEmpty: true, includeFull: true},
		},
		{
			in:   "68 ctf",
			want: queryFilters{protocol: 68, gametype: 4, gametypeSet: true},
		},
		{
			in:   "68 gametype=3",
			want: queryFilters{protocol: 68, gametype: 3, gametypeSet: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := parseGetservers(tc.in)
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseGetserversExt(t *testing.T) {
	got := parseGetserversExt("Quake3Arena 68 ipv6 empty")
	want := queryFilters{
		extended:     true,
		gamename:     "Quake3Arena",
		protocol:     68,
		wantV6:       true,
		includeEmpty: true,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMatchEntry(t *testing.T) {
	v4 := mustAddrPort(t, "10.0.0.1:27960")
	v6 := netip.AddrPortFrom(netip.MustParseAddr("2001:db8::1"), 27960)

	playing := regEntry{addr: v4, protocol: 68, gamename: "baseq3", clients: 2, maxClients: 16, gametype: 4}
	empty := regEntry{addr: v4, protocol: 68, gamename: "baseq3", clients: 0, maxClients: 16}
	full := regEntry{addr: v4, protocol: 68, gamename: "baseq3", clients: 16, maxClients: 16}
	v6Entry := regEntry{addr: v6, protocol: 68, gamename: "baseq3", clients: 1, maxClients: 16}

	t.Run("protocol mismatch rejects", func(t *testing.T) {
		f := queryFilters{protocol: 71}
		if matchEntry(playing, f) {
			t.Error("protocol mismatch should reject")
		}
	})
	t.Run("empty filtered out by default", func(t *testing.T) {
		f := queryFilters{protocol: 68}
		if matchEntry(empty, f) {
			t.Error("empty server should be excluded by default")
		}
	})
	t.Run("empty included when filter set", func(t *testing.T) {
		f := queryFilters{protocol: 68, includeEmpty: true}
		if !matchEntry(empty, f) {
			t.Error("empty server should be included with includeEmpty")
		}
	})
	t.Run("full filtered out by default", func(t *testing.T) {
		f := queryFilters{protocol: 68}
		if matchEntry(full, f) {
			t.Error("full server should be excluded by default")
		}
	})
	t.Run("v6 excluded from non-extended", func(t *testing.T) {
		f := queryFilters{protocol: 68}
		if matchEntry(v6Entry, f) {
			t.Error("v6 entry should not appear in legacy getservers")
		}
	})
	t.Run("v6 included in extended", func(t *testing.T) {
		f := queryFilters{protocol: 68, extended: true, gamename: "baseq3"}
		if !matchEntry(v6Entry, f) {
			t.Error("v6 entry should appear in getserversExt")
		}
	})
	t.Run("v6 only when ipv6 token set", func(t *testing.T) {
		f := queryFilters{protocol: 68, extended: true, gamename: "baseq3", wantV6: true}
		if !matchEntry(v6Entry, f) {
			t.Error("v6 entry should match ipv6 filter")
		}
		if matchEntry(playing, f) {
			t.Error("v4 entry should be excluded when only ipv6 requested")
		}
	})
	t.Run("gametype filter", func(t *testing.T) {
		f := queryFilters{protocol: 68, gametype: 3, gametypeSet: true}
		if matchEntry(playing, f) {
			t.Error("gametype 4 should not match filter 3")
		}
		f.gametype = 4
		if !matchEntry(playing, f) {
			t.Error("gametype 4 should match filter 4")
		}
	})
}

func TestBuildResponsesEOTAndChunking(t *testing.T) {
	// 200 v4 entries × 7 bytes = 1400 bytes of payload. With the OOB
	// header + keyword + EOT, we should land in 2 datagrams.
	entries := make([]regEntry, 200)
	for i := range entries {
		ip := netip.AddrFrom4([4]byte{10, 0, byte(i / 256), byte(i % 256)})
		entries[i] = regEntry{addr: netip.AddrPortFrom(ip, uint16(27960+i))}
	}
	chunks := buildResponses(entries, queryFilters{protocol: 68})
	if len(chunks) < 2 {
		t.Fatalf("expected ≥2 chunks for 200 entries, got %d", len(chunks))
	}
	prefix := []byte(OOBHeader + "getserversResponse")
	for i, c := range chunks {
		if !bytes.HasPrefix(c, prefix) {
			t.Fatalf("chunk %d missing prefix", i)
		}
		if len(c) > maxResponseChunk+len("\\EOT\x00\x00\x00") {
			t.Errorf("chunk %d size=%d exceeds %d + EOT", i, len(c), maxResponseChunk)
		}
	}
	last := chunks[len(chunks)-1]
	if !bytes.HasSuffix(last, []byte("\\EOT\x00\x00\x00")) {
		t.Error("final chunk missing EOT")
	}
	for _, c := range chunks[:len(chunks)-1] {
		if bytes.HasSuffix(c, []byte("\\EOT\x00\x00\x00")) {
			t.Error("non-final chunk has EOT")
		}
	}
}

func TestBuildResponsesEmpty(t *testing.T) {
	chunks := buildResponses(nil, queryFilters{protocol: 68})
	if len(chunks) != 1 {
		t.Fatalf("empty result should produce 1 chunk, got %d", len(chunks))
	}
	want := OOBHeader + "getserversResponse" + "\\EOT\x00\x00\x00"
	if string(chunks[0]) != want {
		t.Errorf("got %q, want %q", chunks[0], want)
	}
}

func TestBuildResponsesMixedFamily(t *testing.T) {
	v4 := netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 2, 3, 4}), 27960)
	v6 := netip.AddrPortFrom(netip.MustParseAddr("2001:db8::1"), 27961)
	chunks := buildResponses([]regEntry{
		{addr: v4},
		{addr: v6},
	}, queryFilters{protocol: 68, extended: true})
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	body := chunks[0]
	prefix := []byte(OOBHeader + "getserversExtResponse")
	if !bytes.HasPrefix(body, prefix) {
		t.Fatal("missing extended prefix")
	}
	rest := body[len(prefix):]
	// Expect: \ + 4B v4 + 2B port + / + 16B v6 + 2B port + EOT
	if rest[0] != '\\' {
		t.Errorf("v4 record separator = %q, want '\\'", rest[0])
	}
	v4ip := rest[1:5]
	if !bytes.Equal(v4ip, []byte{1, 2, 3, 4}) {
		t.Errorf("v4 ip = %v", v4ip)
	}
	v4port := binary.BigEndian.Uint16(rest[5:7])
	if v4port != 27960 {
		t.Errorf("v4 port = %d", v4port)
	}
	if rest[7] != '/' {
		t.Errorf("v6 record separator = %q, want '/'", rest[7])
	}
	expected6 := netip.MustParseAddr("2001:db8::1").As16()
	if !bytes.Equal(rest[8:24], expected6[:]) {
		t.Errorf("v6 ip mismatch")
	}
	v6port := binary.BigEndian.Uint16(rest[24:26])
	if v6port != 27961 {
		t.Errorf("v6 port = %d", v6port)
	}
	if !bytes.HasSuffix(body, []byte("\\EOT\x00\x00\x00")) {
		t.Error("missing EOT")
	}
}

// Ensure entries are reasonably randomized for chunking tests so we
// don't accidentally pass with all-zeros padding.
func TestBuildResponsesV4Roundtrip(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	r := newRegistry(15*time.Minute, 10, clock)
	for i := 0; i < 3; i++ {
		ap := netip.AddrPortFrom(netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}), uint16(27960+i))
		r.Upsert(ap, map[string]string{
			"protocol":      "68",
			"gamename":      "baseq3",
			"clients":       "1",
			"sv_maxclients": "16",
		}, int64(i))
	}
	snap := r.Snapshot()
	chunks := buildResponses(snap, queryFilters{protocol: 68})
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	body := chunks[0]
	if !strings.Contains(string(body), "getserversResponse") {
		t.Fatal("missing keyword")
	}
	// Each record is 7 bytes; we have 3 records + EOT (6 bytes).
	want := len(OOBHeader+"getserversResponse") + 3*recordSizeIPv4 + len("\\EOT\x00\x00\x00")
	if len(body) != want {
		t.Errorf("body len=%d, want %d", len(body), want)
	}
	_ = fmt.Sprintf // keep import even if unused on some builds
}
