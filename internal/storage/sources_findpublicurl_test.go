package storage

import (
	"context"
	"testing"
)

func TestFindSourcePublicURL(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sources (source, demo_base_url) VALUES (?, ?)`,
		"hub", "https://hub.example.com",
	); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sources (source) VALUES (?)`, "bare",
	); err != nil {
		t.Fatalf("insert bare source: %v", err)
	}

	cases := []struct{ name, source, want string }{
		{"known with url", "hub", "https://hub.example.com"},
		{"known without url", "bare", ""},
		{"unknown source", "ghost", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.FindSourcePublicURL(ctx, tc.source)
			if err != nil {
				t.Fatalf("FindSourcePublicURL: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
