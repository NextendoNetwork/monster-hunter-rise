package main

import (
	"testing"

	commonpb "npln.nintendo.net/npln-practice/proto/common"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

func TestOwnSignallingCleanupAllowedBeforeEnvelopeExists(t *testing.T) {
	const gs = "tenants/t-4f53195c-lp1/gameSessions/gs-1"
	host := gamesyncSession{UID: "u-host", GameSession: gs, UserSession: "userSessions/us-host"}
	guest := gamesyncSession{UID: "u-guest", GameSession: gs, UserSession: "userSessions/us-guest"}
	sessions := map[string]gamesyncSession{"us-host": host, "us-guest": guest}

	own := "docs/__pgn/All/__stu/us-host"
	if err := writableDocument(own, nil, host, map[string]string{}, nil, sessions); err != nil {
		t.Fatalf("cleanup of the caller's own envelope was refused: %v", err)
	}

	other := "docs/__pgn/All/__stu/us-guest"
	if err := writableDocument(other, nil, host, map[string]string{}, nil, sessions); err == nil {
		t.Fatal("cleanup of another session's unowned envelope was allowed")
	}

	if err := writableDocument(own, nil, host, map[string]string{own: "us-guest"}, nil, sessions); err == nil {
		t.Fatal("cleanup of an envelope owned by someone else was allowed")
	}
}

func TestOnlyHostWritesRoomSettings(t *testing.T) {
	const gs = "tenants/t-4f53195c-lp1/gameSessions/gs-1"
	host := gamesyncSession{UID: "u-host", GameSession: gs, UserSession: "userSessions/us-host", Sequence: 1}
	guest := gamesyncSession{UID: "u-guest", GameSession: gs, UserSession: "userSessions/us-guest", Sequence: 2}
	settings := cloneFields(nil)
	settings.Fields["cp"] = gamesyncIntegerValue(1)

	if err := writableDocument("docs/__gs/m", settings, host, nil, nil, nil); err != nil {
		t.Fatalf("host settings write refused: %v", err)
	}
	if err := writableDocument("docs/__gs/m", settings, guest, nil, nil, nil); err == nil {
		t.Fatal("guest was allowed to rewrite room settings")
	}
	if err := writableDocument("docs/__gs/r", settings, host, nil, nil, nil); err == nil {
		t.Fatal("host was allowed to rewrite the server-owned player count")
	}
	if err := writableDocument("docs/__gs/m", nil, host, nil, nil, nil); err == nil {
		t.Fatal("room settings could be deleted")
	}
}

func TestCapacityFromPeaceWalkerConfigName(t *testing.T) {
	for config, want := range map[string]int32{
		"tenants/t-4f53195c-lp1/matchmakingConfigs/Mmc_CoOps2": 2,
		"tenants/t-4f53195c-lp1/matchmakingConfigs/Mmc_CoOps4": 4,
		"tenants/current/matchmakingConfigs/CourseMatch_20221202": 4,
		"tenants/current/matchmakingConfigs/WorldMapMatch_1":      12,
	} {
		if got := publicMatchCapacity(config); got != want {
			t.Errorf("%s: capacity %d, want %d", config, got, want)
		}
	}
}

func TestHostRoomPropertiesReachMatchmaking(t *testing.T) {
	r := newSessionRegistry()
	r.sessions["gs-1"] = &mmpb.GameSession{
		Name: "tenants/t-4f53195c-lp1/gameSessions/gs-1",
		Properties: &commonpb.MapValue{Fields: map[string]*commonpb.Value{
			"MatchMode":       gamesyncStringValue("COOPS"),
			"_BaseConfigName": gamesyncStringValue("Mmc_CoOps2"),
		}},
	}
	g := newGamesyncServer(r)
	prp := &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"HelloOpt":        {ValueType: &commonpb.Value_BytesValue{BytesValue: []byte("COLLECTING")}},
		"UpdateInfo":      gamesyncIntegerValue(3),
		"_BaseConfigName": gamesyncStringValue("tampered"),
	}}
	g.publishRoomProperties("tenants/t-4f53195c-lp1/gameSessions/gs-1", prp)

	f := r.sessions["gs-1"].GetProperties().GetFields()
	if string(f["HelloOpt"].GetBytesValue()) != "COLLECTING" || f["UpdateInfo"].GetIntegerValue() != 3 {
		t.Fatalf("host properties not published: %v", f)
	}
	if f["MatchMode"].GetStringValue() != "COOPS" || f["_BaseConfigName"].GetStringValue() != "Mmc_CoOps2" {
		t.Fatalf("existing or server-owned properties were changed: %v", f)
	}
}

func TestJoinReturnsOnlyJoinersVerifiableSession(t *testing.T) {
	gs := &mmpb.GameSession{
		Name: nplnTenant + "/gameSessions/gs-7",
		UserSessions: []*mmpb.UserSession{
			{Name: nplnTenant + "/gameSessions/gs-7/userSessions/us-host", User: nplnTenant + "/users/u-host", Team: "HOST"},
			{Name: nplnTenant + "/gameSessions/gs-7/userSessions/us-guest", User: nplnTenant + "/users/u-guest", Team: "GUEST"},
		},
	}
	got := joinerMatchedSession(gs, "u-guest")
	if len(got) != 1 || got[0].GetUserSession() != nplnTenant+"/gameSessions/gs-7/userSessions/us-guest" {
		t.Fatalf("join response = %v, want only the guest's session", got)
	}
	claims, err := verifyGssMatchToken(got[0].GetMatchmakingIdToken())
	if err != nil {
		t.Fatalf("joiner token rejected by gamesync: %v", err)
	}
	if claims.Subject != "u-guest" || claims.Game.UserSessionID != "us-guest" || claims.Game.GameSessionID != "gs-7" {
		t.Fatalf("token bound to the wrong session: %+v", claims)
	}
	if joinerMatchedSession(gs, "u-stranger") != nil {
		t.Fatal("a non-member received a matched session")
	}
}
