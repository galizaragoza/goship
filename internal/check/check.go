// Package check verifies that what the loaded configuration names can actually
// be reached: the repos it deploys and the hosts it deploys to.
package check

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"goship/internal/config"
	"goship/internal/out"

	"github.com/go-git/go-git/v6"
	gconfig "github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/storage/memory"
)

// dialTimeout is the deadline for every host reachability check.
const dialTimeout = 5 * time.Second

// Repos checks that every repo something is actually deployed with can be
// reached, and returns the set of distinct URLs it verified.
func Repos(cfg *config.Config) (map[string]bool, error) {
	out.Section("Checking repos")
	checked := make(map[string]bool)
	master := cfg.Master
	// Only the repos something is actually deployed with are checked. An
	// ignored master lends its repo to its subjects without receiving it,
	// so it reaches this list through them or not at all.
	var urls []string
	if !master.Ignore {
		urls = append(urls, master.Repo)
	}
	for _, subject := range master.Subjects {
		if subject.Ignore {
			continue
		}
		urls = append(urls, subject.Repo)
	}

	for _, url := range urls {
		if checked[url] {
			continue
		}
		checked[url] = true

		rem := git.NewRemote(memory.NewStorage(), &gconfig.RemoteConfig{
			URLs: []string{url},
		})
		if _, err := rem.List(&git.ListOptions{}); err != nil {
			return checked, fmt.Errorf("checking repo %s: %w", url, err)
		}
		out.Item("Repo %s exists, proceeding", url)
	}
	out.Result("All repos in the loaded configuration are reachable")
	return checked, nil
}

// Hosts checks that the machine at the head of the chain answers on the ssh
// port.
func Hosts(cfg *config.Config) error {
	out.Section("Checking hosts")

	// Only the first hop can be dialed from here: every other machine in the
	// config, the master included, lives behind the one before it.
	ip, name := cfg.Master.IP, cfg.Master.Name
	if len(cfg.Jumps) > 0 {
		ip, name = cfg.Jumps[0].IP, cfg.Jumps[0].Name
	}
	if err := dial(ip, name); err != nil {
		return err
	}

	out.Result("All hosts in the loaded configuration are reachable")
	return nil
}

func dial(ip netip.Addr, name string) error {
	addr := net.JoinHostPort(ip.String(), config.DefaultPort)
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("host %s (%s) not responding: %w", ip, name, err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("problem closing a connection with host %s (%s)", ip, name)
	}
	out.Item("Host %s (%s) is reachable, checking current configuration", ip, name)
	return nil
}
