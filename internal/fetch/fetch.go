// Package fetch pulls the configured repos onto local scratch space so they can
// be deployed to the subjects.
package fetch

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

// Repo clones url into a fresh temporary directory and returns its path.
// The caller owns the directory and must remove it when done, typically with
// defer os.RemoveAll(path).
func Repo(url string) (string, error) {
	// An empty root means os.TempDir(), so TMPDIR is honoured, matching how
	// the ship package creates its own scratch files.
	dir, err := os.MkdirTemp("", "goship-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir for %s: %w", url, err)
	}

	opts := &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		SingleBranch:  true,
		Depth:         1,
		Tags:          plumbing.NoTags,
	}

	if _, err := git.PlainClone(dir, opts); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("cloning %s into %s: %w", url, dir, err)
	}

	return dir, nil
}
