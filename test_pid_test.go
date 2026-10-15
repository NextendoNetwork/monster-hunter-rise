package main

import (
	"encoding/base64"
	"testing"

	authpb "npln.nintendo.net/npln-practice/proto/auth/v1"
)

func idTokenWithClaims(payload string) *authpb.ExternalIdToken {
	enc := base64.RawURLEncoding
	jwt := enc.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + enc.EncodeToString([]byte(payload)) + ".sig"
	return &authpb.ExternalIdToken{Token: &authpb.ExternalIdToken_NsaIdToken{NsaIdToken: jwt}}
}

func TestTestPIDAcceptedOnlyInBarePIDRange(t *testing.T) {
	cases := []struct {
		payload string
		want    uint64
		ok      bool
	}{
		{`{"sub":"ab","npid":"1800009999"}`, 1800009999, true},
		{`{"sub":"ab","npid":"1809999999"}`, 1809999999, true},
		{`{"sub":"ab","npid":"1810000000"}`, 0, false},
		{`{"sub":"ab","npid":"1799999999"}`, 0, false},
		{`{"sub":"ab","npid":"12"}`, 0, false},
		{`{"sub":"ab"}`, 0, false},
		{`{"sub":"ab","nnex":"nx2.x.y","npid":"1800009999"}`, 0, false},
	}
	for _, c := range cases {
		got, ok := testPIDFromIDToken(idTokenWithClaims(c.payload))
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", c.payload, got, ok, c.want, c.ok)
		}
	}
}

func TestTestPIDCanBeDisabled(t *testing.T) {
	t.Setenv("NPLN_ALLOW_TEST_PID", "0")
	if _, ok := testPIDFromIDToken(idTokenWithClaims(`{"npid":"1800009999"}`)); ok {
		t.Fatal("test pid accepted with NPLN_ALLOW_TEST_PID=0")
	}
}

func TestTwoTestPIDsGetDistinctIdentities(t *testing.T) {
	pidA, userA, errA := gatedIdentity(idTokenWithClaims(`{"npid":"1800000501"}`), "tenants/t-4f53195c-lp1")
	pidB, userB, errB := gatedIdentity(idTokenWithClaims(`{"npid":"1800000502"}`), "tenants/t-4f53195c-lp1")
	if errA != nil || errB != nil {
		t.Fatalf("gatedIdentity errors: %v %v", errA, errB)
	}
	if pidA != 1800000501 || pidB != 1800000502 || userA == userB {
		t.Fatalf("test pids collapsed: %d %q / %d %q", pidA, userA, pidB, userB)
	}
}
