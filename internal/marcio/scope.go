package marcio

import (
	"fmt"
	"strings"
)

// Scope is where a search looks. The two indexes cost very different amounts:
// the list key is built once at load, the full-text key walks every field of
// every record, so the cheap one stays the default.
type Scope int

const (
	// ScopeTitles searches the list key: control number, title, author, year.
	ScopeTitles Scope = iota
	// ScopeRecord searches every subfield value and control field.
	ScopeRecord
	// ScopeBoth matches either.
	ScopeBoth
)

func (s Scope) String() string {
	switch s {
	case ScopeRecord:
		return "record"
	case ScopeBoth:
		return "both"
	default:
		return "titles"
	}
}

// Scopes lists every scope in cycling order.
func Scopes() []Scope { return []Scope{ScopeTitles, ScopeRecord, ScopeBoth} }

// Next is the following scope, wrapping.
func (s Scope) Next() Scope { return (s + 1) % Scope(len(Scopes())) }

// NeedsFullText reports whether the scope reads the full-text index.
func (s Scope) NeedsFullText() bool { return s == ScopeRecord || s == ScopeBoth }

// ParseScope maps a command-line value to a Scope.
func ParseScope(s string) (Scope, error) {
	want := strings.ToLower(strings.TrimSpace(s))
	for _, sc := range Scopes() {
		if sc.String() == want {
			return sc, nil
		}
	}
	return ScopeTitles, fmt.Errorf("unknown search scope %q (want titles, record or both)", s)
}

// Matches reports whether a record matches query in this scope. searchKey and
// fullKey are the precomputed, lowercased indexes; query must be lowercase.
func (s Scope) Matches(searchKey, fullKey, query string) bool {
	if query == "" {
		return true
	}
	switch s {
	case ScopeRecord:
		return strings.Contains(fullKey, query)
	case ScopeBoth:
		return strings.Contains(searchKey, query) || strings.Contains(fullKey, query)
	default:
		return strings.Contains(searchKey, query)
	}
}
