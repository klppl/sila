package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.CreateList("Test List", "https://example.com/list.txt"); err != nil {
		t.Fatalf("create list: %v", err)
	}

	lists, err := s.GetLists()
	if err != nil {
		t.Fatalf("get lists: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("expected 1 list, got %d", len(lists))
	}
	if lists[0].Name != "Test List" {
		t.Fatalf("expected name 'Test List', got %q", lists[0].Name)
	}

	if err := s.ReplaceEntries(1, []string{"example.com", "test.com"}); err != nil {
		t.Fatalf("replace entries: %v", err)
	}
	if err := s.UpdateListFetched(1, 2); err != nil {
		t.Fatalf("update list fetched: %v", err)
	}

	c, err := s.TotalDomains()
	if err != nil {
		t.Fatalf("total domains: %v", err)
	}
	if c != 2 {
		t.Fatalf("expected 2 domains, got %d", c)
	}

	if err := s.AddWhitelist("test.com"); err != nil {
		t.Fatalf("add whitelist: %v", err)
	}

	ok, err := s.IsWhitelisted("test.com")
	if err != nil {
		t.Fatalf("is whitelisted: %v", err)
	}
	if !ok {
		t.Fatal("expected test.com to be whitelisted")
	}

	filtered, err := s.GetFilteredDomains()
	if err != nil {
		t.Fatalf("get filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != "example.com" {
		t.Fatalf("expected [example.com], got %v", filtered)
	}

	results, err := s.SearchDomain("example")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Domain != "example.com" {
		t.Fatalf("unexpected search results: %+v", results)
	}

	os.Remove(dbPath)
}

func TestWildcardWhitelist(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.AddWhitelist("*.example.com"); err != nil {
		t.Fatalf("add whitelist: %v", err)
	}

	tests := []struct {
		domain   string
		expected bool
	}{
		{"foo.example.com", true},
		{"bar.example.com", true},
		{"example.com", false},
		{"other.com", false},
	}
	for _, tc := range tests {
		ok, err := s.IsWhitelisted(tc.domain)
		if err != nil {
			t.Fatalf("is whitelisted %q: %v", tc.domain, err)
		}
		if ok != tc.expected {
			t.Fatalf("IsWhitelisted(%q) = %v, want %v", tc.domain, ok, tc.expected)
		}
	}
}
