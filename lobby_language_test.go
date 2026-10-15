package main

import (
	"testing"

	commonpb "npln.nintendo.net/npln-practice/proto/common"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

func riseLobbyProps(config string, lang int64) *commonpb.MapValue {
	return &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"6":  {ValueType: &commonpb.Value_IntegerValue{IntegerValue: lang}},
		"14": {ValueType: &commonpb.Value_IntegerValue{IntegerValue: 361872192}},
		"_BaseConfigName": {ValueType: &commonpb.Value_StringValue{StringValue: config}},
	}}
}

func TestLobbiesArePublishedForAllLanguages(t *testing.T) {
	for _, lang := range []int64{361872193, 361872202, 361872255} {
		p := mergeMatchmakingSessionProperties("tenants/current/matchmakingConfigs/Lobby4Max_MR", riseLobbyProps("x", lang))
		if got := p.GetFields()["6"].GetIntegerValue(); got != 361872255 {
			t.Fatalf("lobby language %d published as %d, want any (361872255)", lang, got)
		}
	}
	quest := mergeMatchmakingSessionProperties("tenants/current/matchmakingConfigs/NormalQuest14_MR_V2", riseLobbyProps("x", 361872193))
	if got := quest.GetFields()["6"].GetIntegerValue(); got != 361872193 {
		t.Fatalf("quest session slot 6 changed to %d", got)
	}
}

func TestHostRepublishKeepsLobbyOpenToAllLanguages(t *testing.T) {
	r := testRegistry()
	r.sessions["gs-lobby"] = &mmpb.GameSession{Name: "tenants/t/gameSessions/gs-lobby", Properties: riseLobbyProps("Lobby4Max_MR", 361872255)}
	g := &gamesyncServer{registry: r}
	g.publishRoomProperties("tenants/t/gameSessions/gs-lobby", riseLobbyProps("ignored", 361872193))
	if got := r.sessions["gs-lobby"].GetProperties().GetFields()["6"].GetIntegerValue(); got != 361872255 {
		t.Fatalf("host republish set language %d", got)
	}
}
