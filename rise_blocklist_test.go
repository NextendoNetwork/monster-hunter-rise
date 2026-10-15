package main

import (
	commonpb "npln.nintendo.net/npln-practice/proto/common"
	"testing"
)

func TestRiseBlocklistPlatformIdentity(t *testing.T) {
	const uid = "u-rise-blocklist-test"
	retenirIdentite(uid, 1800000301)
	t.Cleanup(func() { identitesParUid.Lock(); delete(identitesParUid.m, uid); identitesParUid.Unlock() })
	session := gamesyncSession{UID: uid, UserSession: "userSessions/us-rise-test"}
	for _, tc := range []struct {
		name, id string
		allowed  bool
	}{
		{"docs/__bl/us-rise-test", "1800000301", true},
		{"docs/__bl/us-rise-test", uid, true},
		// Live clients write the game's own 64-bit player id here; only the session path is checked.
		{"docs/__bl/us-rise-test", "13163638032727624288", true},
		{"docs/__bl/us-other", "1800000301", false},
		{"docs/__bl/us-other", "13163638032727624288", false},
	} {
		fields := &commonpb.MapValue{Fields: map[string]*commonpb.Value{"id": gamesyncStringValue(tc.id)}}
		err := writableDocument(tc.name, fields, session, nil, nil, nil)
		if (err == nil) != tc.allowed {
			t.Fatalf("name=%s id=%s allowed=%v err=%v", tc.name, tc.id, tc.allowed, err)
		}
	}
}
