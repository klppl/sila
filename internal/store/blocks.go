package store

import (
	"fmt"
	"regexp"
	"time"
)

var blockDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)+$`)

type BlockEntry struct {
	ID        int64     `json:"id"`
	Pattern   string    `json:"pattern"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) AddBlock(pattern string) error {
	_, err := s.DB.Exec("INSERT INTO blocks (pattern) VALUES (?)", pattern)
	if err != nil {
		return fmt.Errorf("add block: %w", err)
	}
	return nil
}

func (s *Store) RemoveBlock(id int64) error {
	_, err := s.DB.Exec("DELETE FROM blocks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("remove block: %w", err)
	}
	return nil
}

func (s *Store) GetBlocks() ([]BlockEntry, error) {
	rows, err := s.DB.Query("SELECT id, pattern, created_at FROM blocks ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	defer rows.Close()

	var entries []BlockEntry
	for rows.Next() {
		var e BlockEntry
		var ts string
		if err := rows.Scan(&e.ID, &e.Pattern, &ts); err != nil {
			return nil, fmt.Errorf("scan block: %w", err)
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ts)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) GetBlockRules() []string {
	entries, err := s.GetBlocks()
	if err != nil {
		return nil
	}
	var rules []string
	for _, e := range entries {
		if e.Pattern == "" {
			continue
		}
		if blockDomainRe.MatchString(e.Pattern) {
			rules = append(rules, "||"+e.Pattern+"^")
		} else {
			rules = append(rules, e.Pattern)
		}
	}
	return rules
}
