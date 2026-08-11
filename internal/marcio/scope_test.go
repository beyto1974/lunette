package marcio

import "testing"

func TestScopeMatches(t *testing.T) {
	const (
		key  = "rec-0002 gegevensbescherming vandaele, dariusz 2018"
		full = "rec-0002 gegevensbescherming vandaele, dariusz privacy"
	)

	tests := []struct {
		scope Scope
		query string
		want  bool
	}{
		{ScopeTitles, "vandaele", true},
		{ScopeTitles, "privacy", false}, // only in the record body
		{ScopeRecord, "privacy", true},
		{ScopeRecord, "2018", false}, // only in the list key
		{ScopeBoth, "privacy", true},
		{ScopeBoth, "2018", true},
		{ScopeBoth, "zzzz", false},
		{ScopeTitles, "", true}, // an empty query excludes nothing
	}
	for _, tt := range tests {
		if got := tt.scope.Matches(key, full, tt.query); got != tt.want {
			t.Errorf("%v.Matches(%q) = %v, want %v", tt.scope, tt.query, got, tt.want)
		}
	}
}

func TestScopeNames(t *testing.T) {
	for _, s := range Scopes() {
		got, err := ParseScope(s.String())
		if err != nil || got != s {
			t.Errorf("ParseScope(%q) = %v, %v", s.String(), got, err)
		}
	}
	if _, err := ParseScope("everything"); err == nil {
		t.Error("ParseScope accepted an unknown scope")
	}
	if len(Scopes()) != 3 {
		t.Errorf("Scopes() has %d entries, want 3", len(Scopes()))
	}
}

// Cycling reaches every scope and returns to where it started.
func TestScopeNext(t *testing.T) {
	seen := map[Scope]bool{}
	s := ScopeTitles
	for i := 0; i < len(Scopes()); i++ {
		seen[s] = true
		s = s.Next()
	}
	if s != ScopeTitles {
		t.Errorf("cycling did not return to the start, ended at %v", s)
	}
	if len(seen) != len(Scopes()) {
		t.Errorf("cycling visited %d scopes, want %d", len(seen), len(Scopes()))
	}
}

// Only the record and both scopes need the expensive full-text index.
func TestScopeNeedsFullText(t *testing.T) {
	if ScopeTitles.NeedsFullText() {
		t.Error("the titles scope should not need the full-text index")
	}
	if !ScopeRecord.NeedsFullText() || !ScopeBoth.NeedsFullText() {
		t.Error("record and both scopes need the full-text index")
	}
}
