package aurapi

import "testing"

func TestValidPackageName(t *testing.T) {
	valid := []string{
		"firefox", "pipes.sh", "brave-origin-bin", "gtk+3",
		"python3-pip", "a", "foo_bar", "pkg@1", "0ad",
	}
	for _, n := range valid {
		if !ValidPackageName(n) {
			t.Errorf("ValidPackageName(%q) = false, want true", n)
		}
	}

	// These become directory components under the build cache.
	invalid := []string{
		"", ".", "..", "../evil", "a/b", `a\b`, "/abs",
		"../../etc/passwd", "-leading-dash", ".hidden",
		"has space", "semi;colon", "dollar$sign", "nul\x00byte",
	}
	for _, n := range invalid {
		if ValidPackageName(n) {
			t.Errorf("ValidPackageName(%q) = true, want false", n)
		}
	}
}

// The AUR RPC rejects a one-character query with "Query arg too small".
// Sending it anyway turned a normal search into a visible warning, even
// though the Void results were fine.
func TestSearchSkipsTooShortTerms(t *testing.T) {
	for _, term := range []string{"", " ", "a", "  x  "[2:3]} {
		got, err := Search(term)
		if err != nil {
			t.Errorf("Search(%q) returned an error: %v", term, err)
		}
		if got != nil {
			t.Errorf("Search(%q) hit the network, want a skip", term)
		}
	}
}
