package dirbackup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ArchiveOptions struct {
	SourceDir       string
	DestPartPath    string
	ExcludePatterns []string
	SkipSubtrees    []string
	OnProgress      func(filesDone int, bytesDone int64)
}

type ArchiveResult struct {
	ArchiveSizeBytes int64
	SHA256Checksum   string
	FilesArchived    int
	SkippedFiles     []string
	WarningSummary   string
}

var precompressedExts = map[string]bool{
	".zip":    true,
	".gz":     true,
	".tgz":    true,
	".bz2":    true,
	".xz":     true,
	".7z":     true,
	".rar":    true,
	".zst":    true,
	".jpg":    true,
	".jpeg":   true,
	".png":    true,
	".webp":   true,
	".gif":    true,
	".mp3":    true,
	".mp4":    true,
	".mkv":    true,
	".avi":    true,
	".mov":    true,
	".pdf":    true,
	".docx":   true,
	".xlsx":   true,
	".pptx":   true,
	".backup": true,
}

func chooseZipMethod(filePath string) uint16 {
	ext := strings.ToLower(filepath.Ext(filePath))
	if precompressedExts[ext] {
		return zip.Store
	}
	return zip.Deflate
}

func normalizeSkipPaths(paths []string) []string {
	var out []string
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		abs, err := filepath.Abs(trimmed)
		if err == nil {
			out = append(out, strings.ToLower(filepath.Clean(abs)))
		} else {
			out = append(out, strings.ToLower(filepath.Clean(trimmed)))
		}
	}
	return out
}

func isInsideSkipSubtree(absPathLower string, skipRootsLower []string) bool {
	for _, root := range skipRootsLower {
		if absPathLower == root || strings.HasPrefix(absPathLower, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ShouldExclude evaluates whether a file or directory matches any user-configured exclusion pattern.
func ShouldExclude(relSlashPath, name string, isDir bool, patterns []string) bool {
	relLower := strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(relSlashPath, `\`, `/`), "./"))
	nameLower := strings.ToLower(name)

	for _, raw := range patterns {
		pat := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)))
		pat = strings.TrimPrefix(pat, "./")
		pat = strings.TrimPrefix(pat, "/")
		dirOnly := strings.HasSuffix(pat, "/")
		pat = strings.TrimSuffix(pat, "/")
		if pat == "" {
			continue
		}
		if dirOnly && !isDir {
			continue
		}
		if !strings.Contains(pat, "/") {
			if matched, _ := filepath.Match(pat, nameLower); matched {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pat, relLower); matched {
			return true
		}
		if relLower == pat || strings.HasPrefix(relLower, pat+"/") {
			return true
		}
	}
	return false
}

// InspectDirectory returns whether rootPath exists as a directory plus its live size and file count,
// respecting exclusion patterns, skip subtrees, and context deadline.
func InspectDirectory(ctx context.Context, rootPath string, excludePatterns []string, skipSubtrees []string) (exists bool, totalBytes int64, fileCount int) {
	cleanRoot := filepath.Clean(strings.TrimSpace(rootPath))
	if cleanRoot == "" || cleanRoot == "." {
		return false, 0, 0
	}
	info, err := os.Stat(cleanRoot)
	if err != nil || !info.IsDir() {
		return false, 0, 0
	}

	absRoot, err := filepath.Abs(cleanRoot)
	if err != nil {
		absRoot = cleanRoot
	}
	skipRootsLower := normalizeSkipPaths(skipSubtrees)

	_ = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != absRoot {
			if isInsideSkipSubtree(strings.ToLower(filepath.Clean(path)), skipRootsLower) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if isReparsePoint(path, d) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, rErr := filepath.Rel(absRoot, path)
			if rErr == nil && ShouldExclude(filepath.ToSlash(rel), d.Name(), d.IsDir(), excludePatterns) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() && d.Type().IsRegular() {
			if fi, fErr := d.Info(); fErr == nil {
				totalBytes += fi.Size()
				fileCount++
			}
		}
		return nil
	})
	return true, totalBytes, fileCount
}

// ArchiveDirectoryToZip creates a streaming .zip archive of opts.SourceDir at opts.DestPartPath
// while calculating SHA-256 on the fly. Files with exclusive OS locks are skipped and logged in WarningSummary.
func ArchiveDirectoryToZip(ctx context.Context, opts ArchiveOptions) (ArchiveResult, error) {
	var res ArchiveResult

	cleanSrc := filepath.Clean(strings.TrimSpace(opts.SourceDir))
	srcInfo, err := os.Stat(cleanSrc)
	if err != nil {
		return res, fmt.Errorf("source directory unavailable: %w", err)
	}
	if !srcInfo.IsDir() {
		return res, fmt.Errorf("source path is not a directory: %s", cleanSrc)
	}
	absSrc, err := filepath.Abs(cleanSrc)
	if err != nil {
		absSrc = cleanSrc
	}

	absDest, err := filepath.Abs(filepath.Clean(opts.DestPartPath))
	if err != nil {
		absDest = filepath.Clean(opts.DestPartPath)
	}
	skipRootsLower := normalizeSkipPaths(append(opts.SkipSubtrees, absDest))

	outFile, err := os.OpenFile(absDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return res, fmt.Errorf("create zip archive: %w", err)
	}
	closedOut := false
	defer func() {
		if !closedOut {
			_ = outFile.Close()
		}
	}()

	hasher := sha256.New()
	multi := io.MultiWriter(outFile, hasher)
	zw := zip.NewWriter(multi)
	closedZip := false
	defer func() {
		if !closedZip {
			_ = zw.Close()
		}
	}()

	copyBuf := make([]byte, 128*1024)
	var bytesProcessed int64
	var lastReportedBytes int64
	var lastReportedFiles int

	walkErr := filepath.WalkDir(absSrc, func(path string, d fs.DirEntry, wErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if path == absSrc {
			if wErr != nil {
				return wErr
			}
			return nil
		}

		rel, relErr := filepath.Rel(absSrc, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		if wErr != nil {
			if isLockOrPermissionError(wErr) || os.IsNotExist(wErr) {
				res.SkippedFiles = append(res.SkippedFiles, relSlash)
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return wErr
		}

		if isInsideSkipSubtree(strings.ToLower(filepath.Clean(path)), skipRootsLower) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if isReparsePoint(path, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if ShouldExclude(relSlash, d.Name(), d.IsDir(), opts.ExcludePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			hdr, hdrErr := zip.FileInfoHeader(info)
			if hdrErr != nil {
				return nil
			}
			hdr.Name = relSlash + "/"
			hdr.Method = zip.Store
			hdr.Flags |= 0x800
			_, err := zw.CreateHeader(hdr)
			return err
		}

		if !d.Type().IsRegular() {
			return nil
		}

		// Open BEFORE CreateHeader so an exclusively locked file never writes an empty zip entry.
		srcFile, openErr := openSharedRead(path)
		if openErr != nil {
			if isLockOrPermissionError(openErr) || os.IsNotExist(openErr) {
				res.SkippedFiles = append(res.SkippedFiles, relSlash)
				return nil
			}
			return fmt.Errorf("open %s: %w", relSlash, openErr)
		}

		fi, statErr := srcFile.Stat()
		if statErr != nil {
			fi, statErr = d.Info()
		}
		if statErr != nil {
			_ = srcFile.Close()
			res.SkippedFiles = append(res.SkippedFiles, relSlash)
			return nil
		}

		hdr, hdrErr := zip.FileInfoHeader(fi)
		if hdrErr != nil {
			_ = srcFile.Close()
			return hdrErr
		}
		hdr.Name = relSlash
		hdr.Flags |= 0x800
		hdr.Method = chooseZipMethod(path)

		entryWriter, createErr := zw.CreateHeader(hdr)
		if createErr != nil {
			_ = srcFile.Close()
			return createErr
		}

		written, copyErr := io.CopyBuffer(entryWriter, srcFile, copyBuf)
		_ = srcFile.Close()
		if copyErr != nil {
			if isLockOrPermissionError(copyErr) {
				res.SkippedFiles = append(res.SkippedFiles, relSlash+" (locked mid-read)")
				return nil
			}
			return fmt.Errorf("archive %s: %w", relSlash, copyErr)
		}

		res.FilesArchived++
		bytesProcessed += written
		if opts.OnProgress != nil && (res.FilesArchived-lastReportedFiles >= 25 || bytesProcessed-lastReportedBytes >= 4*1024*1024) {
			lastReportedFiles = res.FilesArchived
			lastReportedBytes = bytesProcessed
			opts.OnProgress(res.FilesArchived, bytesProcessed)
		}
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}

	if err := zw.Close(); err != nil {
		return res, fmt.Errorf("finalize zip central directory: %w", err)
	}
	closedZip = true

	if err := outFile.Close(); err != nil {
		return res, fmt.Errorf("close zip archive: %w", err)
	}
	closedOut = true

	st, err := os.Stat(absDest)
	if err != nil {
		return res, err
	}
	res.ArchiveSizeBytes = st.Size()
	res.SHA256Checksum = hex.EncodeToString(hasher.Sum(nil))
	res.WarningSummary = formatSkippedWarning(res.SkippedFiles)
	return res, nil
}

func formatSkippedWarning(skipped []string) string {
	if len(skipped) == 0 {
		return ""
	}
	const maxPreview = 5
	if len(skipped) <= maxPreview {
		return fmt.Sprintf("%d file(s) skipped (locked/in use): %s", len(skipped), strings.Join(skipped, ", "))
	}
	return fmt.Sprintf("%d file(s) skipped (locked/in use): %s (+%d more)", len(skipped), strings.Join(skipped[:maxPreview], ", "), len(skipped)-maxPreview)
}
