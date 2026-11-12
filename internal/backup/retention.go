package backup

import (
	"time"

	"github.com/nico9julio/PGuardian/internal/database"
)

// SelectLocalDeletions determines which completed local backups (ordered newest first) can be safely pruned.
// SAFETY INVARIANT: Never delete a local file whose cloud_status is pending, uploading, or retrying,
// and always preserve at least the single newest completed backup.
func SelectLocalDeletions(copiesNewestFirst []database.BackupExecution, mode string, value int, now time.Time) []database.BackupExecution {
	if value <= 0 || len(copiesNewestFirst) <= 1 {
		return nil
	}

	var toDelete []database.BackupExecution
	cutoff := now.AddDate(0, 0, -value)

	for idx, ex := range copiesNewestFirst {
		if idx == 0 {
			// Always retain at least the latest valid copy
			continue
		}
		if isPendingCloudSync(ex.CloudStatus) {
			// Protect local copy until Google Drive upload succeeds
			continue
		}

		switch mode {
		case "days":
			t, err := time.Parse(time.RFC3339, ex.StartedAt)
			if err == nil && t.Before(cutoff) {
				toDelete = append(toDelete, ex)
			}
		default: // "count"
			if idx >= value {
				toDelete = append(toDelete, ex)
			}
		}
	}
	return toDelete
}

// SelectCloudDeletions determines which completed Google Drive backups (ordered newest first) should be pruned.
func SelectCloudDeletions(copiesNewestFirst []database.BackupExecution, mode string, value int, now time.Time) []database.BackupExecution {
	if value <= 0 || len(copiesNewestFirst) <= 1 {
		return nil
	}

	var toDelete []database.BackupExecution
	cutoff := now.AddDate(0, 0, -value)

	for idx, ex := range copiesNewestFirst {
		if idx == 0 {
			continue
		}
		switch mode {
		case "days":
			t, err := time.Parse(time.RFC3339, ex.StartedAt)
			if err == nil && t.Before(cutoff) {
				toDelete = append(toDelete, ex)
			}
		default: // "count"
			if idx >= value {
				toDelete = append(toDelete, ex)
			}
		}
	}
	return toDelete
}

func isPendingCloudSync(cloudStatus string) bool {
	return cloudStatus == "pending" || cloudStatus == "uploading" || cloudStatus == "retrying"
}
