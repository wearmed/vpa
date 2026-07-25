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
