package fetcher

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"https://small.oisd.nl", true},
		{"https://raw.githubusercontent.com/user/repo/main/list.txt", true},
		{"http://example.com/list.txt", true},
		{"https://192.168.1.1/blocklist", false},
		{"https://127.0.0.1/list", false},
		{"https://localhost/list", false},
		{"https://0.0.0.0/list", false},
		{"https://10.0.0.1/list", false},
		{"https://172.16.0.1/list", false},
		{"https://169.254.1.1/list", false},
		{"https://[::1]/list", false},
		{"ftp://example.com/list", false},
		{"file:///etc/passwd", false},
		{"", false},
		{"/relative/path", false},
		{"not-a-url", false},
	}

	for _, tc := range tests {
		err := ValidateURL(tc.url)
		if tc.valid && err != nil {
			t.Errorf("ValidateURL(%q) should be valid, got: %v", tc.url, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("ValidateURL(%q) should be invalid, got no error", tc.url)
		}
	}
}
