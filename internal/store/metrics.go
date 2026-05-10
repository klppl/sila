package store

import (
	"database/sql"
	"fmt"
)

func (s *Store) RecordDownload(ip string) error {
	_, err := s.DB.Exec(`
		INSERT INTO clients (ip, last_downloaded, download_count)
		VALUES (?, datetime('now'), 1)
		ON CONFLICT(ip) DO UPDATE SET
			last_downloaded = datetime('now'),
			download_count = download_count + 1
	`, ip)
	if err != nil {
		return fmt.Errorf("record download: %w", err)
	}
	return nil
}

func (s *Store) GetDownloadStats() (int, int, error) {
	var unique, total sql.NullInt64
	err := s.DB.QueryRow("SELECT COUNT(*), SUM(download_count) FROM clients").Scan(&unique, &total)
	if err != nil {
		return 0, 0, fmt.Errorf("get download stats: %w", err)
	}
	return int(unique.Int64), int(total.Int64), nil
}
