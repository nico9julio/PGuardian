package postgres

import "testing"

func TestDiscoverPGDumpBinaries(t *testing.T) {
	bins := DiscoverPGDumpBinaries()
	// Should never panic; if PostgreSQL is installed locally, verifies version extraction.
	for _, b := range bins {
		if b.Path == "" {
			t.Fatal("discovered binary has empty path")
		}
		t.Logf("Discovered pg_dump: %s (%s)", b.Path, b.Version)
	}
}
