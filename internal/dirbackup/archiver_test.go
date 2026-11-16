//go:build windows

package dirbackup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestArchiveDirectoryToZipExclusionsAndChecksum(t *testing.T) {
	src := t.TempDir()
	dstDir := filepath.Join(src, "backups_output") // Inside src to also verify SkipSubtrees self-recursion guard
	if err := os.MkdirAll(dstDir, 0750); err != nil {
		t.Fatal(err)
	}

	mustWriteFile(t, filepath.Join(src, "app.json"), `{"ok":true}`)
	mustWriteFile(t, filepath.Join(src, "sub", "report.txt"), "hello world")
	mustWriteFile(t, filepath.Join(src, "sub", "temp.tmp"), "ignore me")
	mustWriteFile(t, filepath.Join(src, "node_modules", "pkg", "index.js"), "ignored")
	mustWriteFile(t, filepath.Join(dstDir, "old.zip"), "self-output")

	partPath := filepath.Join(dstDir, "test_archive.zip.part")
	res, err := ArchiveDirectoryToZip(context.Background(), ArchiveOptions{
		SourceDir:       src,
		DestPartPath:    partPath,
		ExcludePatterns: []string{"*.tmp", "node_modules"},
		SkipSubtrees:    []string{dstDir},
	})
	if err != nil {
		t.Fatalf("ArchiveDirectoryToZip failed: %v", err)
	}
	if res.FilesArchived != 2 {
		t.Fatalf("expected 2 files archived, got %d", res.FilesArchived)
	}
	if len(res.SkippedFiles) != 0 || res.WarningSummary != "" {
		t.Fatalf("expected zero skipped files, got %+v (%q)", res.SkippedFiles, res.WarningSummary)
	}

	// Verify SHA-256 matches actual file bytes
	rawBytes, err := os.ReadFile(partPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(rawBytes)
	if hex.EncodeToString(sum[:]) != res.SHA256Checksum {
		t.Fatalf("checksum mismatch: got %s, want %s", res.SHA256Checksum, hex.EncodeToString(sum[:]))
	}

	// Verify zip contents
	zr, err := zip.OpenReader(partPath)
	if err != nil {
		t.Fatalf("zip.OpenReader failed: %v", err)
	}
	defer zr.Close()

	entries := make(map[string]string)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[f.Name] = string(b)
	}
	if entries["app.json"] != `{"ok":true}` || entries["sub/report.txt"] != "hello world" {
		t.Fatalf("unexpected zip entries: %+v", entries)
	}
	if len(entries) != 2 {
		t.Fatalf("expected exactly 2 file entries in zip, got %+v", entries)
	}

	exists, sizeBytes, count := InspectDirectory(context.Background(), src, []string{"*.tmp", "node_modules"}, []string{dstDir})
	if !exists || count != 2 || sizeBytes != int64(len(`{"ok":true}`)+len("hello world")) {
		t.Fatalf("InspectDirectory mismatch: exists=%v size=%d count=%d", exists, sizeBytes, count)
	}
}

func TestArchiveDirectorySkipsExclusivelyLockedFileWithWarning(t *testing.T) {
	src := t.TempDir()
	dstDir := t.TempDir()

	mustWriteFile(t, filepath.Join(src, "healthy.txt"), "safe content")
	lockedPath := filepath.Join(src, "exclusive.lock")
	mustWriteFile(t, lockedPath, "locked content")

	// Acquire an exclusive OS lock (shareMode = 0) on exclusive.lock
	p16, err := windows.UTF16PtrFromString(lockedPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock CreateFile failed: %v", err)
	}
	defer windows.CloseHandle(h)

	partPath := filepath.Join(dstDir, "locked_test.zip.part")
	res, err := ArchiveDirectoryToZip(context.Background(), ArchiveOptions{
		SourceDir:    src,
		DestPartPath: partPath,
	})
	if err != nil {
		t.Fatalf("expected backup to succeed despite locked file, got error: %v", err)
	}
	if res.FilesArchived != 1 {
		t.Fatalf("expected 1 archived file, got %d", res.FilesArchived)
	}
	if len(res.SkippedFiles) != 1 || res.SkippedFiles[0] != "exclusive.lock" {
		t.Fatalf("expected exclusive.lock in SkippedFiles, got %+v", res.SkippedFiles)
	}
	if !strings.Contains(res.WarningSummary, "exclusive.lock") {
		t.Fatalf("expected WarningSummary to mention exclusive.lock, got %q", res.WarningSummary)
	}

	zr, err := zip.OpenReader(partPath)
	if err != nil {
		t.Fatalf("zip.OpenReader failed: %v", err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "healthy.txt" {
		t.Fatalf("expected only healthy.txt in archive, got %+v", zr.File)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
}
