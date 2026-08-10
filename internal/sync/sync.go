// Package sync is responsible for the logic, that is: delivering the repos specified in the config file to the given IPs
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"
)

// MakeBackup copies every not-synced local file into a versioned backup before
// it gets overwritten by the repo version. notSynced is keyed by paths relative
// to baseDir (the local mirror root, e.g. /apps/icinga). Each file at
// <baseDir>/<rel> is copied to <baseDir>/versions/<rel>.<YYYYMMDD>, mirroring
// the source tree so same-named files at different levels never collide.
func MakeBackup(baseDir string, notSynced map[string]string) error {
	var eg errgroup.Group
	eg.SetLimit(8)

	date := time.Now().Format("20060102")
	for rel := range notSynced {
		eg.Go(func() error {
			return backupFile(baseDir, rel, date)
		})
	}
	return eg.Wait()
}

func backupFile(baseDir, rel, date string) error {
	src := filepath.Join(baseDir, rel)
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading original file %s: %w", src, err)
	}

	dst := filepath.Join(baseDir, "versions", rel) + "." + date
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating backup dir for %s: %w", dst, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("writing backup file %s: %w", dst, err)
	}
	return nil
}
