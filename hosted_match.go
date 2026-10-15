package main

// Rise's quest Join Request is a matchmaking ticket (team "Guest") for the quest config, carrying the
// quest's hopper_name. It must land in the host's quest session, which the host made with a creation
// ticket and so is not in the matchmaker's own pool. A guest is never given a fresh session to host.

import (
	"fmt"
	"log"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	commonpb "npln.nintendo.net/npln-practice/proto/common"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

func ticketIsGuest(t *mmpb.MatchmakingTicket) bool {
	for _, d := range t.GetUserDefinitions() {
		if d.GetTeam() == "Guest" {
			return true
		}
	}
	return false
}

// Rise's quest sessions store the ticket's named attributes in numbered slots (seen in captures):
// "0" hopper_name, "2" AssetVersion, "3" ReAndLibVersion, "5" the quest id (ticket: QuestId_0..QuestId_1).
var riseEqualSlots = map[string]string{"hopper_name": "0", "AssetVersion": "2", "ReAndLibVersion": "3"}

// hostedSessionMatches: every slot the ticket names must agree; slots a ticket doesn't send are ignored.
func hostedSessionMatches(gs *mmpb.GameSession, attrs map[string]int64) bool {
	props := gs.GetProperties().GetFields()
	for attr, slot := range riseEqualSlots {
		if want, ok := attrs[attr]; ok && props[slot].GetIntegerValue() != want {
			return false
		}
	}
	lo, okLo := attrs["QuestId_0"]
	hi, okHi := attrs["QuestId_1"]
	if okLo && okHi {
		q := props["5"].GetIntegerValue()
		if q < lo || q > hi {
			return false
		}
	}
	return true
}

// findHostedSessionLocked: an open host-created session of the ticket's config that matches its
// attributes. Caller holds registry.mu.
func (m *matchmakerServer) findHostedSessionLocked(t *mmpb.MatchmakingTicket, uid string) *mmpb.GameSession {
	config := lastResourceSegment(t.GetMatchmakingConfig())
	attrs := map[string]int64{}
	for _, d := range t.GetUserDefinitions() {
		for k, v := range d.GetAttributes().GetFields() {
			if iv, ok := v.GetValueType().(*commonpb.Value_IntegerValue); ok {
				attrs[k] = iv.IntegerValue
			}
		}
	}
	for id, gs := range m.registry.sessions {
		if m.registry.pooled[id] != nil || gs.GetState() != mmpb.GameSession_ACTIVE || !gs.GetCanParticipate() {
			continue
		}
		if gs.GetProperties().GetFields()["_BaseConfigName"].GetStringValue() != config {
			continue
		}
		if !hostedSessionMatches(gs, attrs) {
			continue
		}
		if sessionHasUID(gs, uid) || gs.GetCurrentParticipantCount() < gs.GetMaxParticipantCount() {
			return gs
		}
	}
	return nil
}

// joinHostedSessionLocked adds the ticket's user to gs and wraps it for the ticket's later Track calls.
func joinHostedSessionLocked(gs *mmpb.GameSession, t *mmpb.MatchmakingTicket, uid string) *publicMatchSession {
	if !sessionHasUID(gs, uid) {
		d := proto.Clone(t.GetUserDefinitions()[0]).(*mmpb.UserDefinition)
		d.User = nplnTenant + "/users/" + uid
		gs.UserSessions = append(gs.UserSessions, &mmpb.UserSession{
			Name: gs.Name + fmt.Sprintf("/userSessions/us-%d", time.Now().UnixNano()), User: d.User, Team: d.Team,
			Attributes: d.Attributes, LatencyData: d.LatencyData, State: mmpb.UserSession_ACTIVE, CreateTime: timestamppb.Now(),
		})
		gs.CurrentParticipantCount = int32(len(gs.UserSessions))
	}
	wrap := &publicMatchSession{gameSession: gs}
	for _, u := range gs.GetUserSessions() {
		if u.GetState() != mmpb.UserSession_ACTIVE {
			continue
		}
		member := &publicMatchMember{
			definition:  &mmpb.UserDefinition{User: u.User, Team: u.Team, Attributes: u.Attributes, LatencyData: u.LatencyData},
			userSession: u.Name,
		}
		if userIDFromPath(u.User) == uid {
			member.matchToken = mintGssMatchToken(uid, nplnTenant, gs.Name, u.Name, u.Team, gamesyncAttrJSON(u.Attributes), gamesyncLatencyJSON(u.LatencyData))
		}
		wrap.members = append(wrap.members, member)
	}
	log.Printf("[NPLN MM] guest %s placed in hosted session %s (%d member(s))", uid, lastResourceSegment(gs.Name), len(wrap.members))
	return wrap
}
