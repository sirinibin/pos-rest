package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseRepairLedgersArgs(t *testing.T) {
	sid := "6acaf31abcd7c1a47874acd4"
	for _, c := range []struct {
		args           []string
		apply, restore bool
		store          string
		bad            bool
	}{
		{args: nil},
		{args: []string{"--apply"}, apply: true},
		{args: []string{"--store=" + sid}, store: sid},
		{args: []string{"--apply", "--store", sid}, apply: true, store: sid},
		{args: []string{"--restore", "--store=" + sid}, restore: true, store: sid},
		{args: []string{"--apply", "--restore"}, bad: true},
		{args: []string{"--store=nope"}, bad: true},
		{args: []string{"--force"}, bad: true},
		{args: []string{"extra"}, bad: true},
	} {
		st, apply, restore, err := parseRepairLedgersArgs(c.args)
		if c.bad {
			if err == nil {
				t.Errorf("%v: want an error", c.args)
			}
			continue
		}
		got := ""
		if st != nil {
			got = st.Hex()
		}
		if err != nil || apply != c.apply || restore != c.restore || got != c.store {
			t.Errorf("%v: store=%q apply=%v restore=%v err=%v", c.args, got, apply, restore, err)
		}
	}
}

// Bad arguments exit with 2 before touching any database.
func TestRunRepairLedgers_BadArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runRepairLedgers([]string{"--store=nope"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "usage") || out.Len() != 0 {
		t.Fatalf("code %d, out %q, err %q", code, out.String(), errOut.String())
	}
}
