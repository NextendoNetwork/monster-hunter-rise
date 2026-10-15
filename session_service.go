package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	commonpb "npln.nintendo.net/npln-practice/proto/common"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

// propertyMatches applies the search screen's semantics rather than plain equality: a numeric 0 is
// the "ALL" option, and StoryProgress asks for missions the searcher has already reached, so a host
// at chapter 10 stays visible to a player at 12.
func propertyMatches(key string, want, have *commonpb.Value) bool {
	if iv, ok := want.GetValueType().(*commonpb.Value_IntegerValue); ok && iv.IntegerValue == 0 {
		return true
	}
	if key == "StoryProgress" {
		return have.GetIntegerValue() <= want.GetIntegerValue()
	}
	return proto.Equal(want, have)
}

func validateCallerDefinitions(defs []*mmpb.UserDefinition, uid string) error {
	if len(defs) != 1 {
		return status.Error(codes.InvalidArgument, "exactly one local user definition required; delegation is not supported")
	}
	u := lastResourceSegment(defs[0].GetUser())
	if u != "" && u != "current" && u != uid {
		return status.Error(codes.PermissionDenied, "cannot submit another user's definition")
	}
	return nil
}
func ticketOwnedBy(defs []*mmpb.UserDefinition, uid string) bool {
	return len(defs) == 1 && lastResourceSegment(defs[0].GetUser()) == uid
}
func ticketFriendRoom(t *mmpb.MatchmakingTicket) string {
	for _, d := range t.GetUserDefinitions() {
		if v := matchmakingStringAttribute(d, "FriendGameSessionId"); v != "" {
			return lastResourceSegment(v)
		}
	}
	return ""
}
func (m *matchmakerServer) authorizeFriendPool(ctx context.Context, t *mmpb.MatchmakingTicket, uid string) error {
	room := ticketFriendRoom(t)
	if !strings.HasPrefix(lastResourceSegment(t.MatchmakingConfig), "Friend") {
		if room != "" {
			return status.Error(codes.InvalidArgument, "room attribute requires friend config")
		}
		return nil
	}
	if room == "" {
		return status.Error(codes.InvalidArgument, "FriendGameSessionId required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	gs := m.registry.sessions[room]
	if gs == nil || gs.State != mmpb.GameSession_ACTIVE {
		return status.Error(codes.NotFound, "friend room is no longer active")
	}
	// Possession of a room ID is not membership; JoinGameSession checks friends
	// and password before a player can enter its associated course/map pool.
	if !sessionHasUID(gs, uid) {
		return status.Error(codes.PermissionDenied, "join the friend room before its matchmaking pool")
	}
	return nil
}

func matchedForCaller(gs *mmpb.GameSession, uid string, include []string) []*mmpb.MatchedUserSession {
	out := make([]*mmpb.MatchedUserSession, 0, len(gs.GetUserSessions()))
	for _, u := range gs.GetUserSessions() {
		d := &mmpb.UserDefinition{User: u.User, Team: u.Team, Attributes: u.Attributes, LatencyData: u.LatencyData}
		token := ""
		if userIDFromPath(u.User) == uid && includeMatchmakingIDToken(include, u.User, uid) {
			token = mintGssMatchToken(uid, nplnTenant, gs.Name, u.Name, u.Team, gamesyncAttrJSON(u.Attributes), gamesyncLatencyJSON(u.LatencyData))
		}
		out = append(out, &mmpb.MatchedUserSession{UserDefinition: proto.Clone(d).(*mmpb.UserDefinition), UserSession: u.Name, MatchmakingIdToken: token})
	}
	return out
}
func safeSessionView(gs *mmpb.GameSession, view mmpb.GameSessionView) *mmpb.GameSession {
	out := proto.Clone(gs).(*mmpb.GameSession)
	out.Password = ""
	if view == mmpb.GameSessionView_BASIC {
		out.UserSessions = nil
	}
	return out
}
func roomVisible(gs *mmpb.GameSession, uid string, friends map[string]bool) bool {
	if gs == nil || gs.State != mmpb.GameSession_ACTIVE {
		return false
	}
	if sessionHasUID(gs, uid) {
		return true
	}
	// Internal friend-course pools are visible only to members, never advertised
	// as independently joinable friend rooms.
	if gs.Properties.GetFields()["FriendGameSessionId"].GetStringValue() != "" {
		return false
	}
	if gs.IsPublic {
		return true
	}
	for _, u := range gs.UserSessions {
		if friends[userIDFromPath(u.User)] {
			return true
		}
	}
	return false
}
func validSessionName(name string) bool {
	for _, prefix := range []string{nplnTenant + "/gameSessions/", "tenants/current/gameSessions/"} {
		if strings.HasPrefix(name, prefix) {
			id := strings.TrimPrefix(name, prefix)
			return id != "" && id != "." && id != ".." && !strings.Contains(id, "/")
		}
	}
	return false
}

func validateGameSessionSearchConfig(req *mmpb.QueryGameSessionsRequest, uid string, friends map[string]bool) error {
	config := lastResourceSegment(req.GetGameSessionSearchConfig())
	if config == "" {
		return nil
	}
	if config != "FriendSearch" {
		// Other titles' search configs run as a plain visibility-filtered search.
		return nil
	}
	if len(req.GetUsers()) == 0 {
		return status.Error(codes.InvalidArgument, "FriendSearch requires at least one user")
	}
	for _, user := range req.GetUsers() {
		target := lastResourceSegment(user)
		if target == "current" {
			target = uid
		}
		if target == "" || !friends[target] {
			return status.Error(codes.PermissionDenied, "FriendSearch target is not a friend")
		}
	}
	return nil
}

func (g *gameSessionServer) GetGameSession(ctx context.Context, req *mmpb.GetGameSessionRequest) (*mmpb.GameSession, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	if !validSessionName(req.GetName()) {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant session name")
	}
	g.mu.Lock()
	gs := g.sessions[lastResourceSegment(req.Name)]
	if gs != nil {
		gs = proto.Clone(gs).(*mmpb.GameSession)
	}
	g.mu.Unlock()
	if gs == nil {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	friends := map[string]bool{uid: true}
	if !gs.IsPublic && !sessionHasUID(gs, uid) {
		friends, err = g.registry.friendUIDs(ctx, uid)
		if err != nil {
			return nil, err
		}
	}
	if !roomVisible(gs, uid, friends) {
		return nil, status.Error(codes.NotFound, "session not visible")
	}
	return safeSessionView(gs, req.View), nil
}
func (g *gameSessionServer) BatchGetGameSessions(ctx context.Context, req *mmpb.BatchGetGameSessionsRequest) (*mmpb.BatchGetGameSessionsResponse, error) {
	if len(req.GetNames()) > 100 {
		return nil, status.Error(codes.InvalidArgument, "too many sessions")
	}
	if _, err := authenticatedNPLNUID(ctx); err != nil {
		return nil, err
	}
	out := &mmpb.BatchGetGameSessionsResponse{}
	for _, name := range req.Names {
		gs, err := g.GetGameSession(ctx, &mmpb.GetGameSessionRequest{Name: name, View: req.View})
		if err != nil {
			return nil, err
		}
		out.GameSessions = append(out.GameSessions, gs)
	}
	return out, nil
}

func (g *gameSessionServer) QueryGameSessions(ctx context.Context, req *mmpb.QueryGameSessionsRequest) (*mmpb.QueryGameSessionsResponse, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("[NPLN MM] QueryGameSessions uid=%s request={%s}", uid, prototext.MarshalOptions{}.Format(req))
	if req.Tenant != "" && req.Tenant != "tenants/current" && req.Tenant != nplnTenant {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant")
	}
	if req.PageSize < 0 || req.MinVacancyCount < 0 || req.MinParticipantCount < 0 {
		return nil, status.Error(codes.InvalidArgument, "negative query limit")
	}
	friends, err := g.registry.friendUIDs(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := validateGameSessionSearchConfig(req, uid, friends); err != nil {
		return nil, err
	}
	limit := int(req.PageSize)
	if limit == 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	lobbySearch := strings.HasPrefix(lastResourceSegment(req.GetGameSessionSearchConfig()), "Lobby")
	g.mu.Lock()
	defer g.mu.Unlock()
	names := make([]string, 0, len(g.sessions))
	for name := range g.sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	out := &mmpb.QueryGameSessionsResponse{}
	for _, name := range names {
		if name <= req.PageToken {
			continue
		}
		gs := g.sessions[name]
		if !roomVisible(gs, uid, friends) || !gs.CanParticipate || gs.MaxParticipantCount-gs.CurrentParticipantCount < req.MinVacancyCount || gs.CurrentParticipantCount < req.MinParticipantCount {
			continue
		}
		// Rise stops reading a lobby list at the first quest session, hiding every lobby after it.
		if lobbySearch && !strings.HasPrefix(gs.GetProperties().GetFields()["_BaseConfigName"].GetStringValue(), "Lobby") {
			continue
		}
		matched := true
		for k, v := range req.Properties.GetFields() {
			// An unset value is the search screen's "ALL".
			if v.GetValueType() == nil {
				continue
			}
			// Rise searches by name (LanguageType, ranks) but its lobbies store numbered slots.
			have, ok := gs.Properties.GetFields()[k]
			if !ok {
				continue
			}
			if !propertyMatches(k, v, have) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if len(req.Users) > 0 {
			matched = false
			for _, user := range req.Users {
				want := lastResourceSegment(user)
				if want == "current" {
					want = uid
				}
				if sessionHasUID(gs, want) {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		if len(out.GameSessions) == limit {
			out.NextPageToken = lastResourceSegment(out.GameSessions[len(out.GameSessions)-1].Name)
			break
		}
		out.GameSessions = append(out.GameSessions, safeSessionView(gs, req.View))
	}
	log.Printf("[NPLN MM] QueryGameSessions uid=%s -> %d session(s) of %d", uid, len(out.GameSessions), len(names))
	for _, gs := range out.GameSessions {
		log.Printf("[NPLN MM]   returned %s", prototext.MarshalOptions{}.Format(gs))
	}
	return out, nil
}

func (g *gameSessionServer) JoinGameSession(ctx context.Context, req *mmpb.JoinGameSessionRequest) (*mmpb.JoinGameSessionResponse, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	if !validSessionName(req.Name) {
		return nil, status.Error(codes.InvalidArgument, "invalid session name")
	}
	if err := validateCallerDefinitions(req.UserDefinitions, uid); err != nil {
		return nil, err
	}
	friends, err := g.registry.friendUIDs(ctx, uid)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	gs := g.sessions[lastResourceSegment(req.Name)]
	if !roomVisible(gs, uid, friends) {
		return nil, status.Error(codes.NotFound, "session not visible")
	}
	if !sessionHasUID(gs, uid) {
		if gs.Password != "" && subtle.ConstantTimeCompare([]byte(gs.Password), []byte(req.Password)) != 1 {
			return nil, status.Error(codes.PermissionDenied, "incorrect room password")
		}
		if !gs.CanParticipate || gs.CurrentParticipantCount >= gs.MaxParticipantCount {
			return nil, status.Error(codes.ResourceExhausted, "room is full or closed")
		}
		d := proto.Clone(req.UserDefinitions[0]).(*mmpb.UserDefinition)
		d.User = nplnTenant + "/users/" + uid
		if pooled := g.registry.pooled[lastResourceSegment(gs.Name)]; pooled != nil {
			addPublicMatchMembers(pooled, &mmpb.MatchmakingTicket{UserDefinitions: []*mmpb.UserDefinition{d}}, uid, time.Now())
		} else {
			gs.UserSessions = append(gs.UserSessions, &mmpb.UserSession{Name: gs.Name + fmt.Sprintf("/userSessions/us-%d", time.Now().UnixNano()), User: d.User, Team: d.Team, Attributes: d.Attributes, LatencyData: d.LatencyData, State: mmpb.UserSession_ACTIVE, CreateTime: timestamppb.Now()})
			gs.CurrentParticipantCount = int32(len(gs.UserSessions))
		}
	}
	return &mmpb.JoinGameSessionResponse{GameSession: safeSessionView(gs, mmpb.GameSessionView_FULL), MatchedUserSessions: joinerMatchedSession(gs, uid)}, nil
}

// joinerMatchedSession returns only the caller's own matched user session, always with its gamesync
// token, as in a captured Nintendo JoinGameSession response. Returning every member left Peace Walker
// using the host's entry, which carries no token, and Gamesync IssueToken rejected it.
func joinerMatchedSession(gs *mmpb.GameSession, uid string) []*mmpb.MatchedUserSession {
	var mine *mmpb.UserSession
	for _, u := range gs.GetUserSessions() {
		if userIDFromPath(u.User) == uid {
			mine = u
		}
	}
	if mine == nil {
		return nil
	}
	d := &mmpb.UserDefinition{User: mine.User, Team: mine.Team, Attributes: mine.Attributes, LatencyData: mine.LatencyData}
	token := mintGssMatchToken(uid, nplnTenant, gs.Name, mine.Name, mine.Team, gamesyncAttrJSON(mine.Attributes), gamesyncLatencyJSON(mine.LatencyData))
	return []*mmpb.MatchedUserSession{{UserDefinition: proto.Clone(d).(*mmpb.UserDefinition), UserSession: mine.Name, MatchmakingIdToken: token}}
}

func (g *gameSessionServer) IssueMatchmakingIdToken(ctx context.Context, req *mmpb.IssueMatchmakingIdTokenRequest) (*mmpb.IssueMatchmakingIdTokenResponse, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	if !validSessionName(req.GameSession) {
		return nil, status.Error(codes.InvalidArgument, "invalid session name")
	}
	for _, u := range req.Users {
		if lastResourceSegment(u) != "current" && lastResourceSegment(u) != uid {
			return nil, status.Error(codes.PermissionDenied, "delegation not supported")
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	gs := g.sessions[lastResourceSegment(req.GameSession)]
	if !sessionHasUID(gs, uid) {
		return nil, status.Error(codes.PermissionDenied, "active membership required")
	}
	return &mmpb.IssueMatchmakingIdTokenResponse{GameSession: safeSessionView(gs, mmpb.GameSessionView_FULL), MatchedUserSessions: matchedForCaller(gs, uid, req.IncludeIdTokenUsers)}, nil
}

func (g *gameSessionServer) ListUserSessions(ctx context.Context, req *mmpb.ListUserSessionsRequest) (*mmpb.ListUserSessionsResponse, error) {
	uid, err := authenticatedNPLNUID(ctx)
	if err != nil {
		return nil, err
	}
	if !validSessionName(req.Parent) {
		return nil, status.Error(codes.InvalidArgument, "invalid parent")
	}
	if req.PageSize < 0 {
		return nil, status.Error(codes.InvalidArgument, "negative page size")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	gs := g.sessions[lastResourceSegment(req.Parent)]
	if !sessionHasUID(gs, uid) {
		return nil, status.Error(codes.PermissionDenied, "membership required")
	}
	limit := int(req.PageSize)
	if limit == 0 || limit > 100 {
		limit = 100
	}
	users := append([]*mmpb.UserSession(nil), gs.UserSessions...)
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	out := &mmpb.ListUserSessionsResponse{}
	for _, u := range users {
		if u.Name <= req.PageToken {
			continue
		}
		if len(out.UserSessions) == limit {
			out.NextPageToken = out.UserSessions[limit-1].Name
			break
		}
		out.UserSessions = append(out.UserSessions, proto.Clone(u).(*mmpb.UserSession))
	}
	return out, nil
}
func (g *gameSessionServer) GetUserSession(ctx context.Context, req *mmpb.GetUserSessionRequest) (*mmpb.UserSession, error) {
	parent, _, ok := strings.Cut(req.Name, "/userSessions/")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid user session")
	}
	list, err := g.ListUserSessions(ctx, &mmpb.ListUserSessionsRequest{Parent: parent})
	if err != nil {
		return nil, err
	}
	for _, u := range list.UserSessions {
		if lastResourceSegment(u.Name) == lastResourceSegment(req.Name) {
			return u, nil
		}
	}
	return nil, status.Error(codes.NotFound, "user session not found")
}
