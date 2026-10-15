package main

import (
	"testing"

	commonpb "npln.nintendo.net/npln-practice/proto/common"
	gspb "npln.nintendo.net/npln-practice/proto/gamesync/v1"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

func TestJoinerOfferWrittenIntoHostMailbox(t *testing.T) {
	g := newGamesyncServer()
	const gs = "tenants/t-4f53195c-lp1/gameSessions/gs-7"
	host := gamesyncSession{UID: "u-host", GameSession: gs, UserSession: "userSessions/us-host", Sequence: 1, Connection: 1}
	guest := gamesyncSession{UID: "u-guest", GameSession: gs, UserSession: "userSessions/us-guest", Sequence: 2, Connection: 2}
	g.rememberSession(host)
	g.rememberSession(guest)
	offer := &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"v":   gamesyncIntegerValue(1),
		"i":   gamesyncIntegerValue(1),
		"r":   {ValueType: &commonpb.Value_BooleanValue{BooleanValue: false}},
		"sd":  gamesyncStringValue("v=1\r\na=candidate:1 1 udp 2113929727 10.0.0.123 60526 typ host\r\n"),
		"ssc": gamesyncStringValue("-----BEGIN CERTIFICATE-----\n"),
	}}
	req := &gspb.WriteDocumentsRequest{WriteOperations: []*gspb.WriteOperation{{OperationType: &gspb.WriteOperation_UpdateDocument{
		UpdateDocument: &gspb.UpdateDocumentRequest{Document: &gspb.Document{Name: "docs/__pgn/All/__stu/us-host", Fields: &commonpb.MapValue{Fields: map[string]*commonpb.Value{
			"mp":     gamesyncMapValue(offer),
			"suid":   gamesyncStringValue("u-guest"),
			"susid":  gamesyncStringValue("us-guest"),
			"sussid": gamesyncIntegerValue(2),
			"suscid": gamesyncIntegerValue(2),
		}}}},
	}}}}
	if _, err := g.WriteDocuments(gamesyncContextForSession(guest), req); err != nil {
		t.Fatalf("joiner offer rejected: %v", err)
	}
	got := g.documents[gs]["docs/__pgn/All/__stu/us-host"]
	if got.GetFields().GetFields()["mp"].GetMapValue().GetFields()["sd"].GetStringValue() == "" {
		t.Fatalf("host mailbox lost the offer: %v", got)
	}
}

func TestHostSettingsMirroredToStableCopy(t *testing.T) {
	g := newGamesyncServer()
	const gs = "tenants/t-4f53195c-lp1/gameSessions/gs-5"
	host := gamesyncSession{UID: "u-host", GameSession: gs, UserSession: "userSessions/us-host", Sequence: 1}
	g.rememberSession(host)
	req := &gspb.WriteDocumentsRequest{WriteOperations: []*gspb.WriteOperation{{OperationType: &gspb.WriteOperation_UpdateDocument{
		UpdateDocument: &gspb.UpdateDocumentRequest{Document: &gspb.Document{Name: "docs/__gs/m", Fields: &commonpb.MapValue{Fields: map[string]*commonpb.Value{
			"prp": gamesyncMapValue(&commonpb.MapValue{Fields: map[string]*commonpb.Value{
				"HelloOpt": {ValueType: &commonpb.Value_BytesValue{BytesValue: []byte("COLLECTING")}},
			}}),
		}}}},
	}}}}
	if _, err := g.WriteDocuments(gamesyncContextForSession(host), req); err != nil {
		t.Fatalf("host settings write: %v", err)
	}
	s := g.documents[gs]["docs/__gs/s"]
	if s == nil || string(s.GetFields().GetFields()["prp"].GetMapValue().GetFields()["HelloOpt"].GetBytesValue()) != "COLLECTING" {
		t.Fatalf("__gs/s did not receive the host's HelloOpt: %v", s)
	}
}

func TestRoomHiddenWhileHostDisconnected(t *testing.T) {
	r := newSessionRegistry()
	const gs = "tenants/t-4f53195c-lp1/gameSessions/gs-6"
	r.sessions["gs-6"] = &mmpb.GameSession{Name: gs, State: mmpb.GameSession_ACTIVE, CanParticipate: true,
		UserSessions: []*mmpb.UserSession{{Name: gs + "/userSessions/us-host"}}}
	r.setHostPresent(gs, false)
	if r.sessions["gs-6"].CanParticipate {
		t.Fatal("room still joinable after its host disconnected")
	}
	r.setHostPresent(gs, true)
	if !r.sessions["gs-6"].CanParticipate {
		t.Fatal("room not restored when the host reconnected")
	}
}
