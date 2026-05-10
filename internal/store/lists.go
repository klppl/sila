package store

import (
	"database/sql"
	"fmt"
	"time"
)

type List struct {
	ID                  int64     `json:"id"`
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	Enabled             bool      `json:"enabled"`
	LastFetched         time.Time `json:"last_fetched"`
	EntryCount          int       `json:"entry_count"`
	LastError           string    `json:"last_error"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
}

func (s *Store) CreateList(name, url string) error {
	_, err := s.DB.Exec("INSERT INTO lists (name, url) VALUES (?, ?)", name, url)
	if err != nil {
		return fmt.Errorf("create list: %w", err)
	}
	return nil
}

func (s *Store) GetLists() ([]List, error) {
	rows, err := s.DB.Query("SELECT id, name, url, enabled, last_fetched, entry_count, last_error, consecutive_failures FROM lists ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("query lists: %w", err)
	}
	defer rows.Close()
	return scanLists(rows)
}

func (s *Store) GetEnabledLists() ([]List, error) {
	rows, err := s.DB.Query("SELECT id, name, url, enabled, last_fetched, entry_count, last_error, consecutive_failures FROM lists WHERE enabled = 1 ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("query enabled lists: %w", err)
	}
	defer rows.Close()
	return scanLists(rows)
}

func (s *Store) UpdateListEnabled(id int64, enabled bool) error {
	_, err := s.DB.Exec("UPDATE lists SET enabled = ? WHERE id = ?", enabled, id)
	if err != nil {
		return fmt.Errorf("update list enabled: %w", err)
	}
	return nil
}

func (s *Store) UpdateListFetched(id int64, entryCount int) error {
	_, err := s.DB.Exec("UPDATE lists SET last_fetched = datetime('now'), entry_count = ?, last_error = '', consecutive_failures = 0 WHERE id = ?", entryCount, id)
	if err != nil {
		return fmt.Errorf("update list fetched: %w", err)
	}
	return nil
}

func (s *Store) RecordListError(id int64, errMsg string) error {
	_, err := s.DB.Exec("UPDATE lists SET last_error = ?, consecutive_failures = consecutive_failures + 1 WHERE id = ?", errMsg, id)
	if err != nil {
		return fmt.Errorf("record list error: %w", err)
	}
	return nil
}

func (s *Store) DeleteList(id int64) error {
	_, err := s.DB.Exec("DELETE FROM lists WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete list: %w", err)
	}
	return nil
}

func (s *Store) GetList(id int64) (*List, error) {
	row := s.DB.QueryRow("SELECT id, name, url, enabled, last_fetched, entry_count, last_error, consecutive_failures FROM lists WHERE id = ?", id)
	l := &List{}
	var enabled int
	var lastFetched sql.NullString
	err := row.Scan(&l.ID, &l.Name, &l.URL, &enabled, &lastFetched, &l.EntryCount, &l.LastError, &l.ConsecutiveFailures)
	if err != nil {
		return nil, fmt.Errorf("get list: %w", err)
	}
	l.Enabled = enabled == 1
	if lastFetched.Valid {
		l.LastFetched, _ = time.Parse("2006-01-02 15:04:05", lastFetched.String)
	}
	return l, nil
}

func scanLists(rows *sql.Rows) ([]List, error) {
	var lists []List
	for rows.Next() {
		var l List
		var enabled int
		var lastFetched sql.NullString
		if err := rows.Scan(&l.ID, &l.Name, &l.URL, &enabled, &lastFetched, &l.EntryCount, &l.LastError, &l.ConsecutiveFailures); err != nil {
			return nil, fmt.Errorf("scan list: %w", err)
		}
		l.Enabled = enabled == 1
		if lastFetched.Valid {
			l.LastFetched, _ = time.Parse("2006-01-02 15:04:05", lastFetched.String)
		}
		lists = append(lists, l)
	}
	return lists, rows.Err()
}
