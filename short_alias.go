package main

// Game session short aliases: Rise names a quest session by a short alias the host creates and a
// joiner resolves (GetGameSessionShortAlias) before JoinGameSession.

import (
	"context"
	"crypto/rand"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

const shortAliasTTL = 24 * time.Hour

// Guarded by the shared registry mutex (gameSessionServer.mu).
var (
	shortAliases        = map[string]*mmpb.GameSessionShortAlias{} // code -> alias
	shortAliasBySession = map[string]string{}                      // session id -> code
)

const shortAliasAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newShortAliasCodeLocked() string {
	for {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		for i := range b {
			b[i] = shortAliasAlphabet[int(b[i])%len(shortAliasAlphabet)]
		}
		if shortAliases[string(b)] == nil {
			return string(b)
		}
	}
}

// The SDK parses this name and compares the collection segment to "GameSessionShortAliases" exactly
// (Rise 16.0.2 main+0x2409310); the lowercase form left the joiner stuck on "Joining quest session".
func shortAliasName(code string) string { return nplnTenant + "/GameSessionShortAliases/" + code }

func (g *gameSessionServer) CreateGameSessionShortAlias(ctx context.Context, req *mmpb.CreateGameSessionShortAliasRequest) (*mmpb.GameSessionShortAlias, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("[NPLN GameSession] CreateGameSessionShortAlias uid=%s request={%s}", uid, prototext.MarshalOptions{}.Format(req))
	sessionID := lastResourceSegment(req.GetGameSessionShortAlias().GetGameSession())
	g.mu.Lock()
	defer g.mu.Unlock()
	gs := g.sessions[sessionID]
	if gs == nil || gs.State != mmpb.GameSession_ACTIVE {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	if !sessionHasUID(gs, uid) {
		return nil, status.Error(codes.PermissionDenied, "only a member can alias its session")
	}
	if code, ok := shortAliasBySession[sessionID]; ok && shortAliases[code] != nil {
		log.Printf("[NPLN GameSession] CreateGameSessionShortAlias reuse %s -> %s", code, sessionID)
		return shortAliases[code], nil
	}
	code := lastResourceSegment(req.GetGameSessionShortAlias().GetName())
	if code == "" || shortAliases[code] != nil {
		code = newShortAliasCodeLocked()
	}
	alias := &mmpb.GameSessionShortAlias{
		Name:        shortAliasName(code),
		GameSession: gs.Name,
		ExpireTime:  timestamppb.New(time.Now().Add(shortAliasTTL)),
	}
	shortAliases[code] = alias
	shortAliasBySession[sessionID] = code
	log.Printf("[NPLN GameSession] CreateGameSessionShortAlias %s -> %s", code, sessionID)
	return alias, nil
}

func (g *gameSessionServer) GetGameSessionShortAlias(ctx context.Context, req *mmpb.GetGameSessionShortAliasRequest) (*mmpb.GameSessionShortAlias, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("[NPLN GameSession] GetGameSessionShortAlias uid=%s request={%s}", uid, prototext.MarshalOptions{}.Format(req))
	code := lastResourceSegment(req.GetName())
	g.mu.Lock()
	defer g.mu.Unlock()
	alias := shortAliases[code]
	// Rise asks for an alias named after the quest session's own id, which the host never created.
	if gs := g.sessions[code]; alias == nil && gs != nil && gs.State == mmpb.GameSession_ACTIVE {
		alias = &mmpb.GameSessionShortAlias{
			Name:        shortAliasName(code),
			GameSession: gs.Name,
			ExpireTime:  timestamppb.New(time.Now().Add(shortAliasTTL)),
		}
	}
	if alias == nil || time.Now().After(alias.GetExpireTime().AsTime()) {
		log.Printf("[NPLN GameSession] GetGameSessionShortAlias %q not found", code)
		return nil, status.Error(codes.NotFound, "short alias not found")
	}
	if gs := g.sessions[lastResourceSegment(alias.GameSession)]; gs == nil || gs.State != mmpb.GameSession_ACTIVE {
		log.Printf("[NPLN GameSession] GetGameSessionShortAlias %q -> session gone", code)
		return nil, status.Error(codes.NotFound, "session not found")
	}
	log.Printf("[NPLN GameSession] GetGameSessionShortAlias %s -> %s", code, alias.GameSession)
	return alias, nil
}
