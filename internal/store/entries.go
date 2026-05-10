package store

import "fmt"

func (s *Store) ReplaceEntries(listID int64, domains []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM entries WHERE list_id = ?", listID); err != nil {
		return fmt.Errorf("delete entries: %w", err)
	}

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO entries (list_id, domain) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, d := range domains {
		if _, err := stmt.Exec(listID, d); err != nil {
			return fmt.Errorf("insert entry: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetAllDomains() ([]string, error) {
	rows, err := s.DB.Query("SELECT DISTINCT domain FROM entries ORDER BY domain")
	if err != nil {
		return nil, fmt.Errorf("query domains: %w", err)
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

type DomainSource struct {
	Domain   string `json:"domain"`
	ListName string `json:"list_name"`
	ListURL  string `json:"list_url"`
	Rule     string `json:"rule"`
}

func (s *Store) SearchDomain(query string) ([]DomainSource, error) {
	rows, err := s.DB.Query(`
		SELECT e.domain, l.name, l.url
		FROM entries e
		JOIN lists l ON l.id = e.list_id
		WHERE e.domain LIKE ?
		ORDER BY e.domain, l.name
	`, "%"+query+"%")
	if err != nil {
		return nil, fmt.Errorf("search domain: %w", err)
	}
	defer rows.Close()

	var results []DomainSource
	for rows.Next() {
		var r DomainSource
		if err := rows.Scan(&r.Domain, &r.ListName, &r.ListURL); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) TotalDomains() (int, error) {
	var count int
	err := s.DB.QueryRow("SELECT COUNT(DISTINCT domain) FROM entries").Scan(&count)
	return count, err
}

func (s *Store) TotalLists() (int, error) {
	var count int
	err := s.DB.QueryRow("SELECT COUNT(*) FROM lists").Scan(&count)
	return count, err
}
