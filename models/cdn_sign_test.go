package models

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignCdnURL(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	link := "/cdn/zatca/5f0000000000000000000001/sales/xml/S-1.xml"
	signed := SignCdnURL(link, now)
	u, err := url.Parse(signed)
	if err != nil || u.Path != link || u.Query().Get("sig") == "" || u.Query().Get("exp") == "" {
		t.Fatalf("signed link %q", signed)
	}
	q := u.Query()
	if !ValidCdnSignature(link, q, now.Add(time.Hour)) {
		t.Fatal("a fresh link must verify")
	}
	if ValidCdnSignature(link, q, now.Add(CdnLinkTTL+time.Minute)) {
		t.Fatal("an expired link must not verify")
	}
	if ValidCdnSignature(strings.Replace(link, "S-1", "S-2", 1), q, now) {
		t.Fatal("the signature is for one path only")
	}
	q2 := url.Values{"exp": {q.Get("exp") + "0"}, "sig": {q.Get("sig")}}
	if ValidCdnSignature(link, q2, now) {
		t.Fatal("a changed expiry must not verify")
	}
	if ValidCdnSignature(link, url.Values{}, now) {
		t.Fatal("no signature must not verify")
	}
	for _, in := range []string{"", "https://x/y", "/cdn/a?b=1"} {
		if got := SignCdnURL(in, now); got != in {
			t.Errorf("SignCdnURL(%q) = %q, want unchanged", in, got)
		}
	}
}
