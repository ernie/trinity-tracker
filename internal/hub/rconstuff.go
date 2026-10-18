package hub

import (
	"context"
	"crypto/rand"
	"log"

	"github.com/ernie/trinity-tracker/internal/crypto"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// evaluateRconStuff returns RconStuff when the authenticated user has
// rcon access to the server they just joined, and nil otherwise; the
// collector treats nil as the feature being off. Each success writes a
// source_audit row, so the autoset shows in the same audit log as
// manual rcon commands.
func (w *Writer) evaluateRconStuff(ctx context.Context, user *storage.User, serverID int64) *RconStuff {
	server, err := w.store.GetServerByID(ctx, serverID)
	if err != nil || server == nil {
		return nil
	}
	owners, err := w.store.SourceOwners(ctx)
	if err != nil {
		log.Printf("hub: rcon-autoset SourceOwners failed: %v", err)
		return nil
	}
	role, ok := AuthorizeUserForRcon(user.ID, user.IsAdmin, server, w.localSource, owners)
	if !ok {
		return nil
	}

	var nonce [crypto.RconsetNonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		log.Printf("hub: rcon-autoset rand.Read failed: %v", err)
		return nil
	}
	key := crypto.DeriveRconsetKey(user.GameToken, nonce)

	// Best effort: an audit failure doesn't deny rcon access.
	if w.store != nil {
		uid := user.ID
		if err := w.store.WriteSourceAudit(ctx, server.Source, &uid, "rcon.autoset", server.Key+": "+string(role)); err != nil {
			log.Printf("hub: rcon-autoset audit insert failed: %v", err)
		}
	}

	return &RconStuff{
		EpochNonce: nonce,
		Key:        key,
		Role:       string(role),
	}
}
