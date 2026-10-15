package main

import "testing"

func TestDocumentPathAcceptsPageGroupForm(t *testing.T) {
	cases := []struct {
		name, coll, id string
		ok             bool
	}{
		{"docs/__pus/us-1", "__pus", "us-1", true},
		{"docs/__pgn/All/__pus/us-1", "__pus", "us-1", true},
		{"docs/__pgn/All/__stu/us-2", "__stu", "us-2", true},
		{"docs/__pgn//__pus/us-1", "", "", false},
		{"docs/__pgn/All/__pus", "", "", false},
		{"docs/other/All/__pus/us-1", "", "", false},
	}
	for _, c := range cases {
		coll, id, ok := documentPath(c.name)
		if coll != c.coll || id != c.id || ok != c.ok {
			t.Errorf("%s: got (%q,%q,%v), want (%q,%q,%v)", c.name, coll, id, ok, c.coll, c.id, c.ok)
		}
	}
}
