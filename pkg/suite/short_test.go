package suite

import (
	"flag"
	"testing"
)

func TestRunSkipsInShortMode(t *testing.T) {
	short := flag.Lookup("test.short")
	previous := short.Value.String()
	if err := flag.Set("test.short", "true"); err != nil {
		t.Fatalf("setting short mode: %v", err)
	}
	t.Cleanup(func() {
		if err := flag.Set("test.short", previous); err != nil {
			t.Errorf("restoring short mode: %v", err)
		}
	})

	var run *testing.T
	t.Run("suite", func(t *testing.T) {
		run = t
		New().Run(t, "short mode")
	})

	if !run.Skipped() {
		t.Errorf("Run() did not skip the suite in short mode")
	}
}
