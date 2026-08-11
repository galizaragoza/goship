// Package deploy pushes the fetched repos onto every master and its subjects.
package deploy

import (
	"fmt"
	"net/netip"

	"nlgmonship/internal/config"

	"github.com/mlafeldt/chef-runner/rsync"
)

// All deploys the master and, under it, each of its subjects. downRepos maps
// a repo URL to the local directory it was cloned into, as returned by
// fetch.Repo.
func All(cfg *config.Config, downRepos map[string]string) error {
	master := &cfg.Master
	shell := remoteShell(master)

	// An ignored master is the way in, not a destination: its subjects are
	// still reached through it below, but nothing is written to it.
	if master.Ignore {
		fmt.Printf("Not deploying to %s, used as a jump host only\n", master.Name)
	} else if err := copyTo(&master.Host, shell, downRepos); err != nil {
		return err
	}

	for j := range master.Subjects {
		subject := &master.Subjects[j]
		if subject.Ignore {
			fmt.Printf("Not deploying to %s, ignored in the configuration\n", subject.Name)
			continue
		}
		if err := copyTo(&subject.Host, shell, downRepos); err != nil {
			return err
		}
	}
	return nil
}

// remoteShell builds the ssh command rsync runs to reach the hosts under m.
// When m is ignored it is a jump host rather than a target, so every transfer
// is routed through it with ProxyJump; otherwise subjects are expected to be
// reachable directly. The key is always the master's, as subjects carry none.
func remoteShell(m *config.Master) string {
	shell := "ssh -i " + m.Creds
	if m.Ignore {
		shell += fmt.Sprintf(" -J %s@%s:%d", m.User, hostFor(m.IP), m.Port)
	}
	return shell
}

// copyTo pushes the local clone of h.Repo onto h over rsync, using shell as the
// transport rsync hands to its -e flag.
func copyTo(h *config.Host, shell string, downRepos map[string]string) error {
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
		RemoteHost:  h.User + "@" + hostFor(h.IP),
	}

	// The trailing slash makes rsync copy the contents of src rather
	// than nesting the clone directory itself under h.Dir.
	if err := clt.Copy(h.Dir, src+"/"); err != nil {
		return fmt.Errorf("deploying %s to %s: %w", h.Repo, h.Name, err)
	}
	fmt.Printf("Deployed %s to %s:%s\n", h.Repo, h.Name, h.Dir)
	return nil
}

// hostFor renders addr for use in rsync's "host:path" target, bracketing IPv6
// literals so their colons are not mistaken for the path separator.
func hostFor(addr netip.Addr) string {
	if addr.Is6() && !addr.Is4In6() {
		return "[" + addr.String() + "]"
	}
	return addr.String()
}
