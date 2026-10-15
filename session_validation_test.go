package main

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	commonpb "npln.nintendo.net/npln-practice/proto/common"
)

func TestSessionResourceNameRejectsNestedAndEmptyIDs(t *testing.T) {
	for _, prefix := range []string{nplnTenant, "tenants/current"} {
		if !validSessionName(prefix + "/gameSessions/gs-test") {
			t.Fatal("valid session rejected")
		}
		for _, suffix := range []string{"", ".", "..", "gs-test/userSessions/other"} {
			if validSessionName(prefix + "/gameSessions/" + suffix) {
				t.Fatalf("invalid session accepted: %q", suffix)
			}
		}
	}
}

func TestSignallingConnectionIDRequiresRecoveredIntegerType(t *testing.T) {
	r := testRegistry()
	m := newMatchmaker(r)
	g := newGamesyncServer(r)
	s := testGamesyncJoin(t, g, testMatched(t, m, "u-host", "CourseMatch_20221202", "signal"), "u-host")
	fields := &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"suid":   gamesyncStringValue(s.UID),
		"susid":  gamesyncStringValue(lastResourceSegment(s.UserSession)),
		"sussid": gamesyncIntegerValue(s.Sequence),
		"suscid": gamesyncStringValue("1"),
		"pl":     {ValueType: &commonpb.Value_BytesValue{BytesValue: []byte{1}}},
		"mp":     {ValueType: &commonpb.Value_MapValue{MapValue: cloneFields(nil)}},
	}}
	if err := writableDocument("docs/__stg/All", fields, s, nil, nil, nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("string connection ID accepted: %v", err)
	}
	fields.Fields["suscid"] = gamesyncIntegerValue(s.Connection)
	if err := writableDocument("docs/__stg/All", fields, s, nil, nil, nil); err != nil {
		t.Fatalf("typed signalling envelope rejected: %v", err)
	}
	fields.Fields["suid"] = gamesyncStringValue("u-other")
	if err := writableDocument("docs/__stg/All", fields, s, nil, nil, nil); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("forged sender accepted: %v", err)
	}
}

func TestNatTraversalMonitoringValidation(t *testing.T) {
	local := gamesyncSession{UID: "u-host", GameSession: "gs-friends", UserSession: "userSessions/us-host"}
	peer := gamesyncSession{UID: "u-guest", GameSession: local.GameSession, UserSession: "userSessions/us-guest"}
	sessions := map[string]gamesyncSession{"us-host": local, "us-guest": peer}
	entry := &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"lu":  gamesyncStringValue(local.UID),
		"ru":  gamesyncStringValue(peer.UID),
		"p2p": gamesyncStringValue("nn::pia::npln::NplnPlugin"),
		"rc":  gamesyncIntegerValue(1),
		"re":  {ValueType: &commonpb.Value_BooleanValue{BooleanValue: false}},
	}}
	entries := &commonpb.ArrayValue{Values: []*commonpb.Value{{ValueType: &commonpb.Value_MapValue{MapValue: entry}}}}
	fields := &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"a": {ValueType: &commonpb.Value_ArrayValue{ArrayValue: entries}},
	}}
	if err := writableDocument("docs/__mt/nat_traversal", fields, local, nil, nil, sessions); err != nil {
		t.Fatalf("captured NAT traversal monitoring document rejected: %v", err)
	}
	fields.Fields["a"].GetArrayValue().Values[0].GetMapValue().Fields["ru"] = gamesyncStringValue("u-outsider")
	if err := writableDocument("docs/__mt/nat_traversal", fields, local, nil, nil, sessions); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("inactive remote user accepted: %v", err)
	}
}
