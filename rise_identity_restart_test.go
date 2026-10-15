package main

import (
	"context"
	"google.golang.org/grpc/metadata"
	"testing"
)

func TestRiseIdentityRestoredFromExistingToken(t *testing.T) {
	const uid = "u-rise-restart-test"
	const pid uint64 = 1800000301
	token := mintNplnAccessToken(pid, nplnTenant+"/users/"+uid, nplnTenant)
	identitesParUid.Lock()
	delete(identitesParUid.m, uid)
	identitesParUid.Unlock()
	t.Cleanup(func() { identitesParUid.Lock(); delete(identitesParUid.m, uid); identitesParUid.Unlock() })
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
	got, err := authenticatedNPLNUID(ctx)
	if err != nil || got != uid || pidPourUid(uid) != pid {
		t.Fatalf("identity not restored: uid=%q pid=%d err=%v", got, pidPourUid(uid), err)
	}
}
