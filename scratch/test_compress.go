package main

import (
	"fmt"
	"strings"
)

func main() {
	allRules := map[string]string{
		"example.com": "||example.com^",
		"ads.example.com": "||ads.example.com^",
		"tracking.ads.example.com": "||tracking.ads.example.com^",
		"other.com": "||other.com^",
		"sub.other.com": "other.com##.ad",
		"co.uk": "||co.uk^",
		"test.co.uk": "||test.co.uk^",
	}

	removed := 0
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

	for k, v := range allRules {
		fmt.Printf("%s: %s\n", k, v)
	}
	fmt.Printf("Removed: %d\n", removed)
}
