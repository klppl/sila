package parser

import (
	"bufio"
	"regexp"
	"sort"
	"strings"
)

var (
	hostsLineRe   = regexp.MustCompile(`^(?:0\.0\.0\.0|127\.0\.0\.1)\s+(.+)`)
	abpRe         = regexp.MustCompile(`^\|\|(.+?)\^`)
	cosmeticRe    = regexp.MustCompile(`^(.+?)#[#@\$?]`)
	validDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)+$`)
)

type Result struct {
	Domains    []string          // canonical domains (for DB storage, search, whitelist)
	Rules      map[string]string // domain → full rule line (for output)
	Standalone []string          // rules with no domain (regex patterns, path rules, etc.)
}

func Parse(raw string) *Result {
	seen := make(map[string]bool)
	rules := make(map[string]string)
	var standalone []string

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line[0] == '#' || line[0] == '!' || line[0] == '[' {
			continue
		}

		var domain, rule string
		isStandalone := false

		switch {
		case strings.HasPrefix(line, "@@||"):
			continue
		case strings.HasPrefix(line, "/"):
			standalone = append(standalone, line)
			continue
		case strings.HasPrefix(line, "||"):
			m := abpRe.FindStringSubmatch(line)
			if m != nil {
				domain = m[1]
				rule = line
			} else {
				standalone = append(standalone, line)
				continue
			}
		case strings.HasPrefix(line, "0.0.0.0") || strings.HasPrefix(line, "127.0.0.1"):
			m := hostsLineRe.FindStringSubmatch(line)
			if m != nil {
				parts := strings.Fields(m[1])
				domain = parts[0]
				rule = "||" + domain + "^"
			}
		case cosmeticRe.MatchString(line):
			m := cosmeticRe.FindStringSubmatch(line)
			if m != nil {
				domain = m[1]
				rule = line
			} else {
				isStandalone = true
				rule = line
			}
		default:
			if strings.Contains(line, "$") {
				standalone = append(standalone, line)
				continue
			}
			if strings.Contains(line, " ") {
				domain = strings.Fields(line)[0]
				rule = "||" + domain + "^"
			} else {
				domain = line
				rule = "||" + domain + "^"
			}
		}

		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			if isStandalone && rule != "" {
				standalone = append(standalone, rule)
			}
			continue
		}
		if !validDomainRe.MatchString(domain) {
			if rule != "" {
				standalone = append(standalone, rule)
			}
			continue
		}
		if seen[domain] {
			continue
		}
		seen[domain] = true
		rules[domain] = rule
	}

	domains := make([]string, 0, len(seen))
	for d := range seen {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	return &Result{Domains: domains, Rules: rules, Standalone: standalone}
}
