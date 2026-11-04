package access

import (
	"strconv"
	"strings"
)

// Provider checks access to telegram chats for specific user IDs and usernames.
type Provider struct {
	ids       map[int64]struct{}
	usernames map[string]struct{}
}

// NewProvider creates new access provider.
// Input string must be a list of telegram user ids and usernames separated by commas, spaces or semicolons.
func NewProvider(s string) *Provider {
	ap := &Provider{
		ids:       make(map[int64]struct{}),
		usernames: make(map[string]struct{}),
	}

	fieldFunc := func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	}

	for _, username := range strings.FieldsFunc(s, fieldFunc) {
		username = strings.TrimSpace(username)

		id, err := strconv.ParseInt(username, 10, 64)
		if err == nil {
			ap.ids[id] = struct{}{}
		} else {
			username = strings.TrimPrefix(username, "@")
			ap.usernames[username] = struct{}{}
		}
	}

	return ap
}

// CheckAccess checks access to telegram chat and returns true if access is granted.
func (ap *Provider) CheckAccess(id int64, username string) bool {
	if _, ok := ap.ids[id]; ok {
		return true
	}

	if _, ok := ap.usernames[username]; ok {
		return true
	}

	return false
}
