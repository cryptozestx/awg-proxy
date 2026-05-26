package dns

import "testing"

func TestDomainRuleMatchesDelimobilWildcard(t *testing.T) {
	rule := DomainRule{Pattern: "*.delimobil.*"}

	matches := []string{
		"git.delimobil.ru",
		"ride-frontend-mobile.st.delimobil.ru",
		"GIT.DELIMOBIL.RU.",
	}
	for _, host := range matches {
		if !rule.Matches(host) {
			t.Fatalf("Matches(%q) = false, want true", host)
		}
	}

	nonMatches := []string{
		"ya.ru",
		"openai.com",
		"delimobil.ru",
		"git.delimobil",
		"git.delimobil.ru.evil.com",
		"x.delimobil.attacker.com",
		".delimobil.ru",
		"git.delimobil.ru..",
	}
	for _, host := range nonMatches {
		if rule.Matches(host) {
			t.Fatalf("Matches(%q) = true, want false", host)
		}
	}
}
