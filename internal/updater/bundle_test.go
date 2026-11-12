package updater

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSemVer(t *testing.T) {
	if CompareSemVer("v0.2.0", "0.1.9") != 1 {
		t.Fatal("expected v0.2.0 > 0.1.9")
	}
	if CompareSemVer("1.0.0", "1.0.0") != 0 {
		t.Fatal("expected 1.0.0 == 1.0.0")
	}
	if CompareSemVer("0.1.0", "v0.1.1") != -1 {
		t.Fatal("expected 0.1.0 < v0.1.1")
	}
}

func TestExtractAndVerifyBundle(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "PGuardianUpdate_v0.1.1.zip")

	dummyPayload := []byte("mock-binary-bytes-for-sha256-verification")
	sum := sha256.Sum256(dummyPayload)
	sumHex := hex.EncodeToString(sum[:])

	manifest := Manifest{
		Version:   "0.1.1",
		Commit:    "abc1234",
		BuildDate: "2026-11-11",
		Files: map[string]string{
			"PGuardianService.exe": sumHex,
		},
	}

	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)

	mw, _ := zw.Create("manifest.json")
	mBytes, _ := json.Marshal(manifest)
	// Include UTF-8 BOM prefix to ensure compatibility with PowerShell 5.1 generated files
	bomWithJSON := append([]byte{0xEF, 0xBB, 0xBF}, mBytes...)
	_, _ = mw.Write(bomWithJSON)

	bw, _ := zw.Create("PGuardianService.exe")
	_, _ = bw.Write(dummyPayload)

	_ = zw.Close()
	_ = zf.Close()

	parsed, stagedDir, err := ExtractAndVerifyBundle(zipPath, filepath.Join(tmp, "staging"))
	if err != nil {
		t.Fatalf("ExtractAndVerifyBundle failed: %v", err)
	}
	if parsed.Version != "0.1.1" {
		t.Fatalf("expected version 0.1.1, got %s", parsed.Version)
	}
	if _, err := os.Stat(filepath.Join(stagedDir, "PGuardianService.exe")); err != nil {
		t.Fatalf("expected staged binary to exist: %v", err)
	}
}
