package main

import (
	"testing"

	commonpb "npln.nintendo.net/npln-practice/proto/common"
)

func TestBlocklistAcceptsGamePlayerIDOnOwnSessionOnly(t *testing.T) {
	session := gamesyncSession{UID: "u-console", UserSession: "userSessions/us-1", Sequence: 1}
	fields := &commonpb.MapValue{Fields: map[string]*commonpb.Value{
		"id": gamesyncStringValue("13163638032727624288"),
		"bi": {ValueType: &commonpb.Value_ArrayValue{ArrayValue: &commonpb.ArrayValue{}}},
	}}
	if err := writableDocument("docs/__bl/us-1", fields, session, map[string]string{}, nil, nil); err != nil {
		t.Fatalf("own blocklist with the game's player id refused: %v", err)
	}
	if err := writableDocument("docs/__bl/us-2", fields, session, map[string]string{}, nil, nil); err == nil {
		t.Fatal("wrote another session's blocklist")
	}
}
