package backup

import (
	"testing"
	"time"

	"github.com/nico9julio/PGuardian/internal/database"
)

func TestSelectLocalDeletionsProtectsPendingCloudUploads(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	copies := []database.BackupExecution{
		{ID: 5, StartedAt: now.Add(-1 * time.Hour).Format(time.RFC3339), CloudStatus: "completed"},
		{ID: 4, StartedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), CloudStatus: "completed"},
		{ID: 3, StartedAt: now.Add(-3 * time.Hour).Format(time.RFC3339), CloudStatus: "retrying"}, // MUST BE PROTECTED
		{ID: 2, StartedAt: now.Add(-4 * time.Hour).Format(time.RFC3339), CloudStatus: "pending"},  // MUST BE PROTECTED
		{ID: 1, StartedAt: now.Add(-5 * time.Hour).Format(time.RFC3339), CloudStatus: "completed"},
	}

	// Keep last 2 copies
	toDelete := SelectLocalDeletions(copies, "count", 2, now)
	if len(toDelete) != 1 || toDelete[0].ID != 1 {
		t.Fatalf("expected only ID=1 to be deleted (protecting ID=3 retrying and ID=2 pending), got %+v", toDelete)
	}
}

func TestSelectDeletionsByDaysAlwaysKeepsNewest(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	copies := []database.BackupExecution{
		{ID: 2, StartedAt: now.AddDate(0, 0, -20).Format(time.RFC3339), CloudStatus: "not_applicable"},
		{ID: 1, StartedAt: now.AddDate(0, 0, -30).Format(time.RFC3339), CloudStatus: "not_applicable"},
	}

	// Even if all copies are older than 7 days, newest copy (ID=2) is preserved
	toDelete := SelectLocalDeletions(copies, "days", 7, now)
	if len(toDelete) != 1 || toDelete[0].ID != 1 {
		t.Fatalf("expected only older ID=1 to be deleted while preserving newest ID=2, got %+v", toDelete)
	}
}
