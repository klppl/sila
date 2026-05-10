package server

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/alex/sila/internal/fetcher"
	"github.com/alex/sila/internal/scheduler"
	"github.com/alex/sila/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	store         *store.Store
	scheduler     *scheduler.Scheduler
	tmpl          *template.Template
	adminPassword string
	sessionToken  string
}

func New(s *store.Store, sc *scheduler.Scheduler, adminPassword string) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"relativeTime": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				m := int(d.Minutes())
				return fmt.Sprintf("%dm ago", m)
			case d < 24*time.Hour:
				h := int(d.Hours())
				return fmt.Sprintf("%dh ago", h)
			default:
				days := int(d.Hours() / 24)
				if days == 1 {
					return "1d ago"
				}
				return fmt.Sprintf("%dd ago", days)
			}
		},
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	h := sha256.Sum256([]byte("sila:" + adminPassword))
	sessionToken := base64.RawURLEncoding.EncodeToString(h[:])
	return &Server{store: s, scheduler: sc, tmpl: tmpl, adminPassword: adminPassword, sessionToken: sessionToken}, nil
}

const sessionCookie = "sila_session"

func (srv *Server) sessionAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value != srv.sessionToken {
			http.Redirect(w, r, "/web/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (srv *Server) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "text/plain", "text/html"))

	r.Get("/health", srv.health)
	r.Get("/", srv.publicPage)
	r.Get("/blocklist.txt", srv.blocklist)

	r.Route("/web", func(r chi.Router) {
		r.Get("/login", srv.loginPage)
		r.Post("/login", srv.loginSubmit)
		r.Post("/logout", srv.logout)

		r.Group(func(r chi.Router) {
			r.Use(srv.sessionAuth)

			r.Get("/", srv.dashboard)

			r.Route("/lists", func(r chi.Router) {
				r.Get("/", srv.listsPage)
				r.Get("/table", srv.listsTable)
				r.Post("/", srv.listsAdd)
				r.Put("/{id}/toggle", srv.listsToggle)
				r.Delete("/{id}", srv.listsDelete)
			})

		r.Route("/whitelist", func(r chi.Router) {
			r.Get("/", srv.whitelistPage)
			r.Get("/table", srv.whitelistTable)
			r.Post("/", srv.whitelistAdd)
			r.Post("/bulk", srv.whitelistBulk)
			r.Delete("/{id}", srv.whitelistDelete)
		})

		r.Route("/blocks", func(r chi.Router) {
			r.Get("/", srv.blocksPage)
			r.Get("/table", srv.blocksTable)
			r.Post("/", srv.blocksAdd)
			r.Post("/bulk", srv.blocksBulk)
			r.Delete("/{id}", srv.blocksDelete)
		})

		r.Route("/search", func(r chi.Router) {
			r.Get("/", srv.searchPage)
			r.Get("/results", srv.searchResults)
			r.Post("/whitelist", srv.searchWhitelist)
		})

			r.Post("/refresh", srv.refresh)
		})
	})

	return r
}

func (srv *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(sessionCookie); err == nil {
		http.Redirect(w, r, "/web", http.StatusSeeOther)
		return
	}
	srv.render(w, "login", nil)
}

func (srv *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	if password != srv.adminPassword {
		srv.render(w, "login", map[string]any{"Error": "Wrong password"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    srv.sessionToken,
		Path:     "/web",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30,
	})
	http.Redirect(w, r, "/web", http.StatusSeeOther)
}

func (srv *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/web",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/web/login", http.StatusSeeOther)
}

func (srv *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := srv.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func logError(r *http.Request, format string, args ...any) {
	log.Printf("%s %s: %s", r.Method, r.URL.Path, fmt.Sprintf(format, args...))
}

type kv struct {
	Key   string
	Value int
}

func (srv *Server) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type statsData struct {
	TotalDomains int
	TotalLists   int
	LastRefresh  time.Time
	IsRunning    bool
}

type barItem struct {
	Label string
	Count int
	Width float64
}

type computedStats struct {
	Network  int
	Cosmetic int
	Script   int
	ExtCSS   int
	Except   int
	TopRoots []barItem
}

func computeRuleStats(rules map[string]string) computedStats {
	cs := computedStats{}
	roots := make(map[string]int)

	for domain, rule := range rules {
		switch {
		case strings.Contains(rule, "#$#"):
			cs.Script++
		case strings.Contains(rule, "#?#"):
			cs.ExtCSS++
		case strings.Contains(rule, "#@#"):
			cs.Except++
		case strings.Contains(rule, "##"):
			cs.Cosmetic++
		default:
			cs.Network++
		}

		parts := strings.Split(domain, ".")
		if len(parts) >= 2 {
			roots[parts[len(parts)-2]+"."+parts[len(parts)-1]]++
		} else if len(parts) == 1 {
			roots[domain]++
		}
	}

	cs.TopRoots = topBars(roots, 10)
	return cs
}

func topBars(m map[string]int, n int) []barItem {
	entries := make([]kv, 0, len(m))
	for k, v := range m {
		entries = append(entries, kv{k, v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Value > entries[j].Value })
	if len(entries) > n {
		entries = entries[:n]
	}

	var max int
	for _, e := range entries {
		if e.Value > max {
			max = e.Value
		}
	}

	items := make([]barItem, len(entries))
	for i, e := range entries {
		items[i] = barItem{Label: e.Key, Count: e.Value, Width: 100}
		if max > 0 {
			items[i].Width = float64(e.Value) / float64(max) * 100
		}
	}
	return items
}

func (srv *Server) publicPage(w http.ResponseWriter, r *http.Request) {
	domains, err := srv.store.TotalDomains()
	if err != nil {
		logError(r, "total domains: %v", err)
	}
	listCount, err := srv.store.TotalLists()
	if err != nil {
		logError(r, "total lists: %v", err)
	}
	lists, err := srv.store.GetLists()
	if err != nil {
		logError(r, "get lists: %v", err)
		lists = nil
	}
	whitelistEntries, err := srv.store.GetWhitelist()
	if err != nil {
		logError(r, "get whitelist: %v", err)
		whitelistEntries = nil
	}
	blocks, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
		blocks = nil
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	blocklistURL := scheme + "://" + r.Host

	rules := srv.scheduler.GetRules()
	stats := computeRuleStats(rules)

	sort.Slice(lists, func(i, j int) bool { return lists[i].EntryCount > lists[j].EntryCount })
	topLists := lists
	if len(topLists) > 5 {
		topLists = topLists[:5]
	}
	var maxEntries int
	for _, l := range topLists {
		if l.EntryCount > maxEntries {
			maxEntries = l.EntryCount
		}
	}
	topListBars := make([]barItem, len(topLists))
	for i, l := range topLists {
		topListBars[i] = barItem{Label: l.Name, Count: l.EntryCount, Width: 100}
		if maxEntries > 0 {
			topListBars[i].Width = float64(l.EntryCount) / float64(maxEntries) * 100
		}
	}

	srv.render(w, "public", map[string]any{
		"Title":          "Sila",
		"TotalDomains":   domains,
		"TotalLists":     listCount,
		"WhitelistCount": len(whitelistEntries),
		"BlockCount":     len(blocks),
		"LastRefresh":    srv.scheduler.LastRefresh(),
		"Lists":          lists,
		"TopLists":       topListBars,
		"BlocklistURL":   blocklistURL,
		"Stats":          stats,
	})
}

func (srv *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	domains, err := srv.store.TotalDomains()
	if err != nil {
		logError(r, "total domains: %v", err)
	}
	listCount, err := srv.store.TotalLists()
	if err != nil {
		logError(r, "total lists: %v", err)
	}
	blocks, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
	}
	lists, _ := srv.store.GetLists()

	rules := srv.scheduler.GetRules()
	stats := computeRuleStats(rules)

	sort.Slice(lists, func(i, j int) bool { return lists[i].EntryCount > lists[j].EntryCount })
	topLists := lists
	if len(topLists) > 5 {
		topLists = topLists[:5]
	}
	var maxEntries int
	for _, l := range topLists {
		if l.EntryCount > maxEntries {
			maxEntries = l.EntryCount
		}
	}
	topListBars := make([]barItem, len(topLists))
	for i, l := range topLists {
		topListBars[i] = barItem{Label: l.Name, Count: l.EntryCount, Width: 100}
		if maxEntries > 0 {
			topListBars[i].Width = float64(l.EntryCount) / float64(maxEntries) * 100
		}
	}

	data := statsData{
		TotalDomains: domains,
		TotalLists:   listCount,
		LastRefresh:  srv.scheduler.LastRefresh(),
		IsRunning:    srv.scheduler.IsRunning(),
	}
	srv.render(w, "dashboard", map[string]any{
		"Title":      "Dashboard",
		"Data":       data,
		"BlockCount": len(blocks),
		"Stats":      stats,
		"TopLists":   topListBars,
	})
}

func (srv *Server) blocklist(w http.ResponseWriter, r *http.Request) {
	domains, err := srv.store.GetFilteredDomains()
	if err != nil {
		logError(r, "filter domains: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	listCount, err := srv.store.TotalLists()
	if err != nil {
		listCount = 0
	}

	lastRefresh := srv.scheduler.LastRefresh()
	etag := fmt.Sprintf(`"%d-%d"`, len(domains), lastRefresh.Unix())
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	rules := srv.scheduler.GetRules()
	extra := srv.scheduler.GetExtra()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("ETag", etag)
	fmt.Fprintf(w, "# Sila blocklist — generated %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(w, "# %d domain rules + %d pattern rules from %d source lists\n#\n", len(domains), len(extra), listCount)
	for _, d := range domains {
		if rule, ok := rules[d]; ok {
			fmt.Fprintln(w, rule)
		} else {
			fmt.Fprintf(w, "||%s^\n", d)
		}
	}
	for _, r := range extra {
		fmt.Fprintln(w, r)
	}
	for _, r := range srv.store.GetBlockRules() {
		fmt.Fprintln(w, r)
	}
}

func (srv *Server) listsPage(w http.ResponseWriter, r *http.Request) {
	lists, err := srv.store.GetLists()
	if err != nil {
		logError(r, "get lists: %v", err)
	}
	srv.render(w, "lists", map[string]any{
		"Title": "Lists",
		"Lists": lists,
	})
}

func (srv *Server) listsTable(w http.ResponseWriter, r *http.Request) {
	lists, err := srv.store.GetLists()
	if err != nil {
		logError(r, "get lists: %v", err)
	}
	srv.render(w, "lists-table", lists)
}

func (srv *Server) listsAdd(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	rawURL := r.FormValue("url")
	if name == "" || rawURL == "" {
		http.Error(w, "name and url required", http.StatusBadRequest)
		return
	}
	if err := fetcher.ValidateURL(rawURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := srv.store.CreateList(name, rawURL); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	lists, err := srv.store.GetLists()
	if err != nil {
		logError(r, "get lists: %v", err)
	}
	srv.render(w, "lists-table", lists)
}

func (srv *Server) listsToggle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	enabled := r.FormValue("enabled") == "true"
	if err := srv.store.UpdateListEnabled(id, enabled); err != nil {
		logError(r, "toggle list: %v", err)
	}
	lists, err := srv.store.GetLists()
	if err != nil {
		logError(r, "get lists: %v", err)
	}
	srv.render(w, "lists-table", lists)
}

func (srv *Server) listsDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := srv.store.DeleteList(id); err != nil {
		logError(r, "delete list: %v", err)
	}
	lists, err := srv.store.GetLists()
	if err != nil {
		logError(r, "get lists: %v", err)
	}
	srv.render(w, "lists-table", lists)
}

func (srv *Server) whitelistPage(w http.ResponseWriter, r *http.Request) {
	entries, err := srv.store.GetWhitelist()
	if err != nil {
		logError(r, "get whitelist: %v", err)
	}
	srv.render(w, "whitelist", map[string]any{
		"Title":   "Whitelist",
		"Entries": entries,
	})
}

func (srv *Server) whitelistTable(w http.ResponseWriter, r *http.Request) {
	entries, err := srv.store.GetWhitelist()
	if err != nil {
		logError(r, "get whitelist: %v", err)
	}
	srv.render(w, "whitelist-table", entries)
}

func (srv *Server) whitelistAdd(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	if pattern == "" {
		http.Error(w, "pattern required", http.StatusBadRequest)
		return
	}
	if err := srv.store.AddWhitelist(pattern); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	entries, err := srv.store.GetWhitelist()
	if err != nil {
		logError(r, "get whitelist: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	srv.render(w, "whitelist-table", entries)
}

func (srv *Server) whitelistBulk(w http.ResponseWriter, r *http.Request) {
	text := r.FormValue("patterns")
	if text == "" {
		http.Error(w, "no patterns provided", http.StatusBadRequest)
		return
	}
	lines := strings.Split(text, "\n")
	added := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := srv.store.AddWhitelist(line); err != nil {
			logError(r, "bulk whitelist add %q: %v", line, err)
			continue
		}
		added++
	}
	entries, err := srv.store.GetWhitelist()
	if err != nil {
		logError(r, "get whitelist: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Reswap", "none")
	srv.render(w, "whitelist-table", entries)
}

func (srv *Server) whitelistDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := srv.store.RemoveWhitelist(id); err != nil {
		logError(r, "remove whitelist: %v", err)
	}
	entries, err := srv.store.GetWhitelist()
	if err != nil {
		logError(r, "get whitelist: %v", err)
		return
	}
	srv.render(w, "whitelist-table", entries)
}

func (srv *Server) blocksPage(w http.ResponseWriter, r *http.Request) {
	entries, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
	}
	srv.render(w, "blocks", map[string]any{
		"Title":   "Custom Blocks",
		"Entries": entries,
	})
}

func (srv *Server) blocksTable(w http.ResponseWriter, r *http.Request) {
	entries, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
	}
	srv.render(w, "blocks-table", entries)
}

func (srv *Server) blocksAdd(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	if pattern == "" {
		http.Error(w, "pattern required", http.StatusBadRequest)
		return
	}
	if err := srv.store.AddBlock(pattern); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	entries, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	srv.render(w, "blocks-table", entries)
}

func (srv *Server) blocksBulk(w http.ResponseWriter, r *http.Request) {
	text := r.FormValue("patterns")
	if text == "" {
		http.Error(w, "no patterns provided", http.StatusBadRequest)
		return
	}
	lines := strings.Split(text, "\n")
	added := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := srv.store.AddBlock(line); err != nil {
			logError(r, "bulk block add %q: %v", line, err)
			continue
		}
		added++
	}
	entries, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	srv.render(w, "blocks-table", entries)
}

func (srv *Server) blocksDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := srv.store.RemoveBlock(id); err != nil {
		logError(r, "remove block: %v", err)
	}
	entries, err := srv.store.GetBlocks()
	if err != nil {
		logError(r, "get blocks: %v", err)
		return
	}
	srv.render(w, "blocks-table", entries)
}

func (srv *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	srv.render(w, "search", map[string]any{
		"Title": "Search",
		"Results": map[string]any{
			"Query":   "",
			"Results": nil,
		},
	})
}

func (srv *Server) searchResults(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		srv.render(w, "search-results", map[string]any{
			"Query":   "",
			"Results": nil,
		})
		return
	}
	results, err := srv.store.SearchDomain(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules := srv.scheduler.GetRules()
	for i := range results {
		if rule, ok := rules[results[i].Domain]; ok {
			results[i].Rule = rule
		}
	}
	srv.render(w, "search-results", map[string]any{
		"Query":   query,
		"Results": results,
	})
}

func (srv *Server) refresh(w http.ResponseWriter, r *http.Request) {
	srv.scheduler.Refresh()
	http.Redirect(w, r, "/web", http.StatusSeeOther)
}

func (srv *Server) searchWhitelist(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	if pattern == "" {
		http.Error(w, "pattern required", http.StatusBadRequest)
		return
	}
	if err := srv.store.AddWhitelist(pattern); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	srv.render(w, "whitelisted-badge", nil)
}
