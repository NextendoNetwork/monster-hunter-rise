package main

import (
	"testing"

	commonpb "npln.nintendo.net/npln-practice/proto/common"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

func riseQuestTicket(team string, hopper, quest int64) *mmpb.CreateMatchmakingTicketRequest {
	iv := func(n int64) *commonpb.Value { return &commonpb.Value{ValueType: &commonpb.Value_IntegerValue{IntegerValue: n}} }
	return &mmpb.CreateMatchmakingTicketRequest{
		Parent: "tenants/current",
		MatchmakingTicket: &mmpb.MatchmakingTicket{
			MatchmakingConfig: "tenants/current/matchmakingConfigs/NormalQuest14_MR_V2",
			UserDefinitions: []*mmpb.UserDefinition{{
				User: "tenants/current/users/current",
				Team: team,
				Attributes: &commonpb.MapValue{Fields: map[string]*commonpb.Value{
					"hopper_name": iv(hopper), "QuestId_0": iv(quest), "QuestId_1": iv(quest), "AssetVersion": iv(411373),
				}},
			}},
		},
	}
}

func TestRiseJoinRequestLandsInHostQuest(t *testing.T) {
	reg := newSessionRegistry()
	mm := newMatchmaker(reg)
	iv := func(n int64) *commonpb.Value { return &commonpb.Value{ValueType: &commonpb.Value_IntegerValue{IntegerValue: n}} }
	hostQuest := nplnTenant + "/gameSessions/gs-hostquest"
	reg.sessions["gs-hostquest"] = &mmpb.GameSession{
		Name: hostQuest, State: mmpb.GameSession_ACTIVE, CanParticipate: true, IsPublic: true,
		MaxParticipantCount: 4, CurrentParticipantCount: 1,
		Properties: &commonpb.MapValue{Fields: map[string]*commonpb.Value{
			"_BaseConfigName": {ValueType: &commonpb.Value_StringValue{StringValue: "NormalQuest14_MR_V2"}},
			"0": iv(3002576809), "2": iv(411373), "5": iv(10105),
		}},
		UserSessions: []*mmpb.UserSession{{Name: hostQuest + "/userSessions/us-1", User: nplnTenant + "/users/u-host", Team: "Host", State: mmpb.UserSession_ACTIVE}},
	}

	ctx := callerContext("u-guest")
	ticket, err := mm.CreateMatchmakingTicket(ctx, riseQuestTicket("Guest", 3002576809, 10105))
	if err != nil {
		t.Fatal(err)
	}
	res, ok := mm.completeMatchmakingTicket(ctx, ticket.GetName(), []string{"tenants/current/users/current"})
	if !ok || res.GetGameSession().GetName() != hostQuest {
		t.Fatalf("guest not placed in host quest: ok=%v session=%q", ok, res.GetGameSession().GetName())
	}
	if !sessionHasUID(reg.sessions["gs-hostquest"], "u-guest") || reg.sessions["gs-hostquest"].CurrentParticipantCount != 2 {
		t.Fatal("guest not added to the host's quest session")
	}
	var token bool
	for _, m := range res.GetMatchedUserSessions() {
		if userIDFromPath(m.GetUserDefinition().GetUser()) == "u-guest" && m.GetMatchmakingIdToken() != "" {
			token = true
		}
	}
	if !token {
		t.Fatal("guest got no matchmaking token for its own user session")
	}
	if again, ok := mm.completeMatchmakingTicket(ctx, ticket.GetName(), nil); !ok || again.GetGameSession().GetName() != hostQuest {
		t.Fatal("re-tracking the guest ticket changed its session")
	}

	other := callerContext("u-other")
	wrong, _ := mm.CreateMatchmakingTicket(other, riseQuestTicket("Guest", 3002576809, 99999))
	if _, ok := mm.completeMatchmakingTicket(other, wrong.GetName(), nil); ok {
		t.Fatal("guest for another quest was placed (or given a fresh session)")
	}
	if len(reg.sessions) != 1 {
		t.Fatalf("a guest ticket created a session: %d sessions", len(reg.sessions))
	}

	host := callerContext("u-solo")
	hostTicket, _ := mm.CreateMatchmakingTicket(host, riseQuestTicket("Host", 3002576809, 10105))
	if res, ok := mm.completeMatchmakingTicket(host, hostTicket.GetName(), nil); !ok || res.GetGameSession().GetName() == hostQuest {
		t.Fatal("a non-guest ticket should keep using the public pool")
	}
}
