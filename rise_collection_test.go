package main

import "testing"

func TestRiseCollectionCleanupOwnership(t *testing.T) {
	session := gamesyncSession{UID: "u-rise", UserSession: "userSessions/us-rise", Sequence: 1}
	if err := writableDocument("docs/col/1", nil, session, nil, nil, nil); err != nil {
		t.Fatalf("own cleanup rejected: %v", err)
	}
	if err := writableDocument("docs/col/2", nil, session, nil, nil, nil); err == nil {
		t.Fatal("another participant's cleanup accepted")
	}
}
