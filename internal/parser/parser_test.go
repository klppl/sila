package parser

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseHosts(t *testing.T) {
	input := `
# This is a comment
0.0.0.0 doubleclick.net
127.0.0.1 bad-ad.com
0.0.0.0 tracker.io # inline comment
`
	r := Parse(input)
	expected := []string{"bad-ad.com", "doubleclick.net", "tracker.io"}
	if !reflect.DeepEqual(r.Domains, expected) {
		t.Fatalf("expected %v, got %v", expected, r.Domains)
	}
	if r.Rules["doubleclick.net"] != "||doubleclick.net^" {
		t.Fatalf("unexpected rule: %s", r.Rules["doubleclick.net"])
	}
}

func TestParseABP(t *testing.T) {
	input := `
! Adblock Plus format
||doubleclick.net^
||ads.example.com^$script
||tracker.io^
`
	r := Parse(input)
	expected := []string{"ads.example.com", "doubleclick.net", "tracker.io"}
	if !reflect.DeepEqual(r.Domains, expected) {
		t.Fatalf("expected %v, got %v", expected, r.Domains)
	}
	if r.Rules["ads.example.com"] != "||ads.example.com^$script" {
		t.Fatalf("expected mod rule, got: %s", r.Rules["ads.example.com"])
	}
}

func TestParseMixed(t *testing.T) {
	input := `
# Mixed format
0.0.0.0 doubleclick.net
||tracker.io^
`
	r := Parse(input)
	expected := []string{"doubleclick.net", "tracker.io"}
	if !reflect.DeepEqual(r.Domains, expected) {
		t.Fatalf("expected %v, got %v", expected, r.Domains)
	}
}

func TestParseDedup(t *testing.T) {
	input := `
0.0.0.0 example.com
||example.com^
0.0.0.0 example.com
`
	r := Parse(input)
	if len(r.Domains) != 1 {
		t.Fatalf("expected 1 domain, got %d: %v", len(r.Domains), r.Domains)
	}
}

func TestParseInvalid(t *testing.T) {
	input := `
0.0.0.0
# just a comment
||invalid domain^
not-a-valid-domain
`
	r := Parse(input)
	if len(r.Domains) != 0 {
		t.Fatalf("expected 0 domains, got %v", r.Domains)
	}
}

func TestParseCosmetic(t *testing.T) {
	input := `
flashback.org##.top-banner
tracker.com#$#scriptlet
other.org#?#.selector
ads.example.com#@#.exception
`
	r := Parse(input)
	expected := []string{"ads.example.com", "flashback.org", "other.org", "tracker.com"}
	if !reflect.DeepEqual(r.Domains, expected) {
		t.Fatalf("expected %v, got %v", expected, r.Domains)
	}
	if r.Rules["flashback.org"] != "flashback.org##.top-banner" {
		t.Fatalf("expected cosmetic rule preserved, got: %s", r.Rules["flashback.org"])
	}
	if r.Rules["tracker.com"] != "tracker.com#$#scriptlet" {
		t.Fatalf("expected scriptlet rule preserved, got: %s", r.Rules["tracker.com"])
	}
}

func TestParseCosmeticGeneric(t *testing.T) {
	input := `
##.generic-banner
#@#.generic-exception
0.0.0.0 real-domain.com
`
	r := Parse(input)
	if len(r.Domains) != 1 || r.Domains[0] != "real-domain.com" {
		t.Fatalf("expected [real-domain.com], got %v", r.Domains)
	}
}

func TestParseExceptionSkips(t *testing.T) {
	input := `
@@||whitelisted.com^
||real-block.com^
`
	r := Parse(input)
	if len(r.Domains) != 1 || r.Domains[0] != "real-block.com" {
		t.Fatalf("expected [real-block.com], got %v", r.Domains)
	}
}

func TestParseURLRegex(t *testing.T) {
	input := `
/monetization/pixel-sdk/*
/^https?:\/\/[a-z]+\.example\.com\//
||tracker.io^
`
	r := Parse(input)
	if len(r.Domains) != 1 || r.Domains[0] != "tracker.io" {
		t.Fatalf("expected [tracker.io], got %v", r.Domains)
	}
	if len(r.Standalone) != 2 {
		t.Fatalf("expected 2 standalone, got %d: %v", len(r.Standalone), r.Standalone)
	}
}

func TestParsePathRules(t *testing.T) {
	input := `
||example.com/analytics/pixel^
||ads.com^
||tracker.net/path^$script,third-party
||cdn.com/lib/v1^
`
	r := Parse(input)
	if len(r.Domains) != 1 || r.Domains[0] != "ads.com" {
		t.Fatalf("expected [ads.com], got %v", r.Domains)
	}
	if len(r.Standalone) != 3 {
		t.Fatalf("expected 3 standalone, got %d: %v", len(r.Standalone), r.Standalone)
	}
	for _, s := range r.Standalone {
		if !strings.HasPrefix(s, "||") {
			t.Fatalf("standalone rule missing || prefix: %s", s)
		}
	}
}
