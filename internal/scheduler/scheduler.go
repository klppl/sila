package scheduler

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/klppl/sila/internal/fetcher"
	"github.com/klppl/sila/internal/parser"
	"github.com/klppl/sila/internal/store"
	"golang.org/x/net/publicsuffix"
)

type Scheduler struct {
	store   *store.Store
	fetcher *fetcher.Client

	interval            time.Duration
	aggressiveThreshold int
	mu                  sync.RWMutex
	last     time.Time
	running  bool

	rules   map[string]string
	extra   []string
	rulesMu sync.RWMutex

	stop chan struct{}
}

func New(s *store.Store, f *fetcher.Client, interval time.Duration, aggrThreshold int) *Scheduler {
	return &Scheduler{
		store:               s,
		fetcher:             f,
		interval:            interval,
		aggressiveThreshold: aggrThreshold,
		stop:                make(chan struct{}),
	}
}

func (sc *Scheduler) LastRefresh() time.Time {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.last
}

func (sc *Scheduler) IsRunning() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.running
}

func (sc *Scheduler) GetRule(domain string) (string, bool) {
	sc.rulesMu.RLock()
	defer sc.rulesMu.RUnlock()
	rule, ok := sc.rules[domain]
	return rule, ok
}

func (sc *Scheduler) WalkRules(cb func(domain, rule string)) {
	sc.rulesMu.RLock()
	defer sc.rulesMu.RUnlock()
	for k, v := range sc.rules {
		cb(k, v)
	}
}

func (sc *Scheduler) GetDomainCount() int {
	sc.rulesMu.RLock()
	defer sc.rulesMu.RUnlock()
	return len(sc.rules)
}

func (sc *Scheduler) GetExtra() []string {
	sc.rulesMu.RLock()
	defer sc.rulesMu.RUnlock()
	e := make([]string, len(sc.extra))
	copy(e, sc.extra)
	return e
}

func (sc *Scheduler) Start() {
	go func() {
		sc.refresh()
		ticker := time.NewTicker(sc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sc.refresh()
			case <-sc.stop:
				return
			}
		}
	}()
}

func (sc *Scheduler) Stop() {
	close(sc.stop)
}

func (sc *Scheduler) Refresh() {
	sc.mu.Lock()
	if sc.running {
		sc.mu.Unlock()
		return
	}
	sc.mu.Unlock()
	go sc.refresh()
}

func (sc *Scheduler) refresh() {
	sc.mu.Lock()
	sc.running = true
	sc.mu.Unlock()

	defer func() {
		sc.mu.Lock()
		sc.running = false
		sc.mu.Unlock()
	}()

	lists, err := sc.store.GetEnabledLists()
	if err != nil {
		log.Printf("scheduler: get enabled lists: %v", err)
		return
	}

	sc.rulesMu.RLock()
	allRules := make(map[string]string, len(sc.rules))
	for k, v := range sc.rules {
		allRules[k] = v
	}
	allExtra := make([]string, len(sc.extra))
	copy(allExtra, sc.extra)
	sc.rulesMu.RUnlock()

	updated := 0
	for _, l := range lists {
		body, err := sc.fetcher.Fetch(l.URL)
		if err != nil {
			if err == fetcher.ErrNotModified {
				log.Printf("scheduler: %s not modified, skipping", l.Name)
				continue
			}
			log.Printf("scheduler: fetch %s: %v", l.Name, err)
			if recErr := sc.store.RecordListError(l.ID, err.Error()); recErr != nil {
				log.Printf("scheduler: record error %s: %v", l.Name, recErr)
			}
			continue
		}

		result := parser.Parse(body)
		if err := sc.store.ReplaceEntries(l.ID, result.Domains); err != nil {
			log.Printf("scheduler: replace entries %s: %v", l.Name, err)
			if recErr := sc.store.RecordListError(l.ID, err.Error()); recErr != nil {
				log.Printf("scheduler: record error %s: %v", l.Name, recErr)
			}
			continue
		}
		if err := sc.store.UpdateListFetched(l.ID, len(result.Domains)); err != nil {
			log.Printf("scheduler: update list %s: %v", l.Name, err)
			continue
		}
		for d, r := range result.Rules {
			allRules[d] = r
		}
		allExtra = append(allExtra, result.Standalone...)
		log.Printf("scheduler: refreshed %s (%d domains)", l.Name, len(result.Domains))
		updated++
	}

	if sc.aggressiveThreshold > 0 {
		baseCounts := make(map[string]int)
		for domain := range allRules {
			base, err := publicsuffix.EffectiveTLDPlusOne(domain)
			if err == nil && base != domain {
				baseCounts[base]++
			}
		}

		aggrAdded := 0
		for base, count := range baseCounts {
			if count >= sc.aggressiveThreshold {
				if _, exists := allRules[base]; !exists {
					allRules[base] = "||" + base + "^"
					aggrAdded++
				}
			}
		}
		if aggrAdded > 0 {
			log.Printf("scheduler: aggressive compression added %d base domains", aggrAdded)
		}
	}

	// Compress allRules by removing redundant subdomains
	var removed int
	for domain := range allRules {
		parts := strings.Split(domain, ".")
		if len(parts) <= 2 {
			continue
		}
		for i := 1; i < len(parts)-1; i++ {
			parent := strings.Join(parts[i:], ".")
			if pRule, ok := allRules[parent]; ok && pRule == "||"+parent+"^" {
				delete(allRules, domain)
				removed++
				break
			}
		}
	}
	if removed > 0 {
		log.Printf("scheduler: compressed out %d redundant subdomains across lists", removed)
	}

	sc.rulesMu.Lock()
	sc.rules = allRules
	sc.extra = allExtra
	sc.rulesMu.Unlock()

	sc.mu.Lock()
	sc.last = time.Now()
	sc.mu.Unlock()

	log.Printf("scheduler: refresh complete (%d/%d lists updated)", updated, len(lists))
}
