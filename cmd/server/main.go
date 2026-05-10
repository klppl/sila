package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/klppl/sila/internal/fetcher"
	"github.com/klppl/sila/internal/scheduler"
	"github.com/klppl/sila/internal/server"
	"github.com/klppl/sila/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("SILA_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir(), "sila.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	seedLists(s)

	f := fetcher.New()

	interval := 6 * time.Hour
	if v := os.Getenv("SILA_REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	var aggrThreshold int
	if v := os.Getenv("SILA_AGGRESSIVE_THRESHOLD"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			aggrThreshold = t
		}
	}

	sc := scheduler.New(s, f, interval, aggrThreshold)
	sc.Start()

	adminPassword := os.Getenv("SILA_ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "hunter2"
	}

	srv, err := server.New(s, sc, adminPassword)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: srv.Router(),
	}

	go func() {
		log.Printf("Sila listening on :%s", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")

	sc.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}

	if err := s.Close(); err != nil {
		log.Printf("store close: %v", err)
	}

	log.Println("stopped")
}

func seedLists(s *store.Store) {
	lists, _ := s.GetLists()
	if len(lists) > 0 {
		return
	}
	seeds := []struct{ name, url string }{
		{"OISD Small", "https://small.oisd.nl"},
		{"Frellwit Swedish", "https://raw.githubusercontent.com/lassekongo83/Frellwits-filter-lists/refs/heads/master/Frellwits-Swedish-Filter.txt"},
	}
	for _, seed := range seeds {
		if err := s.CreateList(seed.name, seed.url); err != nil {
			log.Printf("seed list %s: %v", seed.name, err)
		} else {
			log.Printf("seeded list: %s", seed.name)
		}
	}
}

func dataDir() string {
	if d := os.Getenv("SILA_DATA_DIR"); d != "" {
		return d
	}
	return "data"
}
