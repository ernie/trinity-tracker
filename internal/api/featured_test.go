package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func seedFeatureTestMatch(t *testing.T, store *storage.Store, withDemo bool) int64 {
	t.Helper()
	ctx := context.Background()
	srv := &domain.Server{Key: "test-srv-feature", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	uuid := fmt.Sprintf("test-match-%s-%d", t.Name(), time.Now().UnixNano())
	m := &domain.Match{
		UUID:      uuid,
		ServerID:  srv.ID,
		MapName:   "q3dm17",
		GameType:  "ffa",
		StartedAt: time.Now().UTC(),
	}
	if err := store.CreateMatch(ctx, m); err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	if withDemo {
		if err := store.MarkMatchDemoAvailable(ctx, uuid); err != nil {
			t.Fatalf("MarkMatchDemoAvailable: %v", err)
		}
	}
	return m.ID
}

func TestAdminFeatureMatch_RequiresAdmin(t *testing.T) {
	tr := newTestRouter(t)
	matchID := seedFeatureTestMatch(t, tr.store, true)

	// Anonymous (no token) should be rejected.
	w := tr.do(http.MethodPost, "/api/admin/matches/"+strconv.FormatInt(matchID, 10)+"/feature", "", "")
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("expected 401/403 without admin token; got %d", w.Code)
	}

	// Non-admin user should be rejected.
	tok, _ := tr.loginAs(t, "alice", false)
	w = tr.do(http.MethodPost, "/api/admin/matches/"+strconv.FormatInt(matchID, 10)+"/feature", "", tok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin; got %d", w.Code)
	}
}

func TestAdminFeatureMatch_HappyPath(t *testing.T) {
	tr := newTestRouter(t)
	matchID := seedFeatureTestMatch(t, tr.store, true)
	adminTok, _ := tr.loginAs(t, "admin", true)

	// POST sets featured.
	w := tr.do(http.MethodPost, "/api/admin/matches/"+strconv.FormatInt(matchID, 10)+"/feature", "", adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on POST; got %d (%s)", w.Code, w.Body.String())
	}

	ids, err := tr.store.GetFeaturedMatches(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != matchID {
		t.Fatalf("expected match in featured list; got %v", ids)
	}

	// DELETE clears featured.
	w = tr.do(http.MethodDelete, "/api/admin/matches/"+strconv.FormatInt(matchID, 10)+"/feature", "", adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on DELETE; got %d (%s)", w.Code, w.Body.String())
	}

	ids, _ = tr.store.GetFeaturedMatches(context.Background(), 20)
	if len(ids) != 0 {
		t.Fatalf("expected empty after DELETE; got %v", ids)
	}
}

func TestAdminFeatureMatch_BadMatchID(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	w := tr.do(http.MethodPost, "/api/admin/matches/not-a-number/feature", "", adminTok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad id; got %d", w.Code)
	}
}

func TestGetFeaturedMatches_ReturnsIDsOnly(t *testing.T) {
	tr := newTestRouter(t)
	matchID := seedFeatureTestMatch(t, tr.store, true)
	if err := tr.store.SetMatchFeatured(context.Background(), matchID, true); err != nil {
		t.Fatal(err)
	}

	w := tr.do(http.MethodGet, "/api/matches/featured", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d (%s)", w.Code, w.Body.String())
	}

	var resp struct {
		Matches []int64 `json:"matches"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Matches) != 1 || resp.Matches[0] != matchID {
		t.Fatalf("got %v", resp.Matches)
	}
}

func TestGetFeaturedMatches_EmptyByDefault(t *testing.T) {
	tr := newTestRouter(t)

	w := tr.do(http.MethodGet, "/api/matches/featured", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d", w.Code)
	}

	var resp struct {
		Matches []int64 `json:"matches"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Matches == nil {
		t.Fatalf("matches should be empty array, not nil — got null in JSON?")
	}
	if len(resp.Matches) != 0 {
		t.Fatalf("expected empty list; got %v", resp.Matches)
	}
}
