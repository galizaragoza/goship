// Package ship pushes the fetched repos onto every master and its subjects.
package ship

import (
	"fmt"
	"os"

	"goship/internal/config"

	"github.com/mlafeldt/chef-runner/rsync"
)

// All deploys the master and, under it, each of its subjects. downRepos maps
// a repo URL to the local directory it was cloned into, as returned by
// fetch.Repo.
func All(cfg *config.Config, downRepos map[string]string) error {
	master := &cfg.Master

	path, err := hopFile(cfg)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	shell := fmt.Sprintf("ssh -F %q", path)

	// An ignored master is the way in, not a destination: its subjects are
	// still reached through it below, but nothing is written to it.
	if master.Ignore {
		fmt.Printf("Not deploying to %s, used as a jump host only\n", master.Name)
	} else if err := rsyncTo(&master.Host, masterAlias, shell, downRepos); err != nil {
		return err
	}

	for j := range master.Subjects {
		subject := &master.Subjects[j]
		if subject.Ignore {
			fmt.Printf("Not deploying to %s, ignored in the configuration\n", subject.Name)
			continue
		}
		if err := rsyncTo(&subject.Host, subjectAlias(j), shell, downRepos); err != nil {
			return err
		}
	}
	return nil
}

// rsyncTo pushes the local clone of h.Repo onto the machine known as alias in
// this deploy's ssh config, using shell as the transport rsync hands to its -e
// flag.
func rsyncTo(h *config.Host, alias, shell string, downRepos map[string]string) error {
	src, ok := downRepos[h.Repo]
	if !ok {
		return fmt.Errorf("no local clone for repo %s (host %s)", h.Repo, h.Name)
	}

	clt := rsync.Client{
		Archive:     true,
		Compress:    true,
		Verbose:     true,
		Exclude:     []string{".git"},
		RemoteShell: shell,
		// The alias carries the address, the user and the way in, so rsync
		// only has to name it. It also keeps IPv6 literals out of rsync's
		// "host:path", where their colons would read as the separator.
		RemoteHost: alias,
	}

	// The trailing slash makes rsync copy the contents of src rather
	// than nesting the clone directory itself under h.Dir.
	if err := clt.Copy(h.Dir, src+"/"); err != nil {
		return fmt.Errorf("deploying %s to %s: %w", h.Repo, h.Name, err)
	}
	fmt.Printf("Deployed %s to %s:%s\n", h.Repo, h.Name, h.Dir)
	return nil
}

func scpTo(h *config.Host, alias, shell string, downRepos map[string]string) error {
	return nil
}
