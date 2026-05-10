package store

import (
	"fmt"
	"strings"
	"time"
)

type WhitelistEntry struct {
	ID        int64     `json:"id"`
	Pattern   string    `json:"pattern"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) AddWhitelist(pattern string) error {
	_, err := s.DB.Exec("INSERT INTO whitelist (pattern) VALUES (?)", pattern)
	if err != nil {
		return fmt.Errorf("add whitelist: %w", err)
	}
	return nil
}

func (s *Store) RemoveWhitelist(id int64) error {
	_, err := s.DB.Exec("DELETE FROM whitelist WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("remove whitelist: %w", err)
	}
	return nil
}

func (s *Store) GetWhitelist() ([]WhitelistEntry, error) {
	rows, err := s.DB.Query("SELECT id, pattern, created_at FROM whitelist ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("query whitelist: %w", err)
	}
	defer rows.Close()

	var entries []WhitelistEntry
	for rows.Next() {
		var e WhitelistEntry
		var ts string
		if err := rows.Scan(&e.ID, &e.Pattern, &ts); err != nil {
			return nil, fmt.Errorf("scan whitelist: %w", err)
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ts)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func isWildcardMatch(pattern, domain string) bool {
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	suffix := pattern[1:] // ".example.com"
	if !strings.HasSuffix(domain, suffix) {
		return false
	}
	prefix := domain[:len(domain)-len(suffix)]
	return prefix != ""
}

func (s *Store) IsWhitelisted(domain string) (bool, error) {
	entries, err := s.GetWhitelist()
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Pattern == domain {
			return true, nil
		}
		if isWildcardMatch(e.Pattern, domain) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) GetFilteredDomains() ([]string, error) {
	domains, err := s.GetAllDomains()
	if err != nil {
		return nil, err
	}

	whitelist, err := s.GetWhitelist()
	if err != nil {
		return nil, err
	}

	exact := make(map[string]bool)
	var wildcards []string
	for _, w := range whitelist {
		if strings.HasPrefix(w.Pattern, "*.") {
			wildcards = append(wildcards, w.Pattern)
		} else {
			exact[w.Pattern] = true
		}
	}

	filtered := make([]string, 0, len(domains))
	for _, d := range domains {
		if exact[d] {
			continue
		}
		skip := false
		for _, w := range wildcards {
			if isWildcardMatch(w, d) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, d)
		}
	}
	return filtered, nil
}
