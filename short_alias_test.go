package main

import (
	"testing"

	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

func TestShortAliasCreateAndResolve(t *testing.T) {
	g := newGameSessionServer()
	gsName := nplnTenant + "/gameSessions/gs-quest-1"
	g.sessions["gs-quest-1"] = &mmpb.GameSession{
		Name:  gsName,
		State: mmpb.GameSession_ACTIVE,
		UserSessions: []*mmpb.UserSession{{
			Name: gsName + "/userSessions/us-1", User: nplnTenant + "/users/u-host", State: mmpb.UserSession_ACTIVE,
		}},
	}
	req := &mmpb.CreateGameSessionShortAliasRequest{
		Parent:                "tenants/current",
		GameSessionShortAlias: &mmpb.GameSessionShortAlias{GameSession: "tenants/current/gameSessions/gs-quest-1"},
	}
	if _, err := g.CreateGameSessionShortAlias(callerContext("u-other"), req); err == nil {
		t.Fatal("a non-member aliased someone else's session")
	}
	alias, err := g.CreateGameSessionShortAlias(callerContext("u-host"), req)
	if err != nil {
		t.Fatal(err)
	}
	if alias.GetGameSession() != gsName || alias.GetName() == "" {
		t.Fatalf("bad alias %+v", alias)
	}
	again, _ := g.CreateGameSessionShortAlias(callerContext("u-host"), req)
	if again.GetName() != alias.GetName() {
		t.Fatal("second create for the same session minted a new alias")
	}
	got, err := g.GetGameSessionShortAlias(callerContext("u-joiner"), &mmpb.GetGameSessionShortAliasRequest{Name: "tenants/current/gameSessionShortAliases/" + lastResourceSegment(alias.GetName())})
	if err != nil || got.GetGameSession() != gsName {
		t.Fatalf("resolve failed: %v %+v", err, got)
	}
	// Rise's joiner asks for the session id itself (capital G, as captured).
	byID, err := g.GetGameSessionShortAlias(callerContext("u-joiner"), &mmpb.GetGameSessionShortAliasRequest{Name: "tenants/current/GameSessionShortAliases/gs-quest-1"})
	if err != nil || byID.GetGameSession() != gsName {
		t.Fatalf("session-id alias failed: %v %+v", err, byID)
	}
	if want := nplnTenant + "/GameSessionShortAliases/gs-quest-1"; byID.GetName() != want {
		t.Fatalf("alias name %q, want %q (the SDK matches the collection case-sensitively)", byID.GetName(), want)
	}
	if _, err := g.GetGameSessionShortAlias(callerContext("u-joiner"), &mmpb.GetGameSessionShortAliasRequest{Name: "tenants/current/gameSessionShortAliases/NOPE"}); err == nil {
		t.Fatal("unknown alias resolved")
	}
}
