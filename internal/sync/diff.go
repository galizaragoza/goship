package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Diff compares the tree at local against the one at repo and reports what has
// drifted: unsync maps each path present in both whose contents differ to the
// repo's hash, and orphans lists the paths that exist locally but not in the
// repo. Both are relative to their respective roots.
//
// The unsync map is what MakeBackup takes, so a diff feeds a backup directly.
func Diff(local string, repo string) (unsync map[string]string, orphans []string, err error) {
	unsync = make(map[string]string)
	orphans = make([]string, 0, 10) // len 0, cap = heuristic estimate

	localHashes, err := fileHashes(local)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting the hashes from the machine: %w", err)
	}
	repoHashes, err := fileHashes(repo)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting the hashes from the repo: %w", err)
	}

	for path, repoHash := range repoHashes {
		localHash, ok := localHashes[path]
		if !ok {
			continue
		}
		if localHash != repoHash {
			unsync[path] = repoHash
		}
	}

	for path := range localHashes {
		if _, ok := repoHashes[path]; !ok {
			orphans = append(orphans, path)
		}
	}

	return unsync, orphans, nil
}

// fileHashes walks the tree under basePath and maps every regular file, keyed
// by its path relative to basePath, to its SHA-256 digest.
func fileHashes(basePath string) (map[string]string, error) {
	hashes := make(map[string]string)

	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking entry: %w", err)
		}

		rel, err := filepath.Rel(basePath, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}

		// Skip the backups dir so prior backups aren't treated as orphaned files.
		if d.IsDir() && rel == backupDir {
			return filepath.SkipDir
		}

		if !d.IsDir() {
			hash, err := getHash(path)
			if err != nil {
				return err
			}
			hashes[rel] = hash
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directories: %w", err)
	}

	return hashes, nil
}

// getHash returns the hex-encoded SHA-256 digest of the file at path.
func getHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hashing: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
