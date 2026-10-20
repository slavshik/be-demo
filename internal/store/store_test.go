package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenPathWithSpecialCharacters checks that characters with a meaning
// in URIs stay part of the filename, and that the pragmas still apply.
func TestOpenPathWithSpecialCharacters(t *testing.T) {
	for _, name := range []string{"game?test.db", "game#1.db", "100%.db", "with space.db", "a&b=c.db"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, name)

			st, err := Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer st.Close()

			for pragma, want := range map[string]string{
				"foreign_keys": "1",
				"journal_mode": "wal",
				"busy_timeout": "5000",
			} {
				var got string
				if err := st.db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
					t.Fatalf("read %s: %v", pragma, err)
				}
				if got != want {
					t.Errorf("%s = %q, want %q", pragma, got, want)
				}
			}

			if _, err := os.Stat(path); err != nil {
				t.Fatalf("database not created at %q: %v", path, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				// Only the database and its -wal/-shm companions.
				if !strings.HasPrefix(e.Name(), name) {
					t.Errorf("unexpected file %q next to %q", e.Name(), name)
				}
			}
		})
	}
}
