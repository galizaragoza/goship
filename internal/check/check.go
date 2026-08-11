// Package check contains logic to check and verify parameters in the loaded configuration, like IP addresses or Repo URLs
package check

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"nlgmonship/internal/config"

	"github.com/go-git/go-git/v6"
	gconfig "github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/storage/memory"
)

const (
	// dialTimeout is the deadline for every host reachability check.
	dialTimeout = 5 * time.Second
	// sshPort is dialed on every host, as ssh is the only supported pathway.
	sshPort = "22"
)

func Repos(cfg *config.Config) (checked map[string]bool, error error) {
	fmt.Println("Checking if all the repos loaded in the config are reachable...")
	checked = make(map[string]bool)
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
		ref, err := rem.List(&git.ListOptions{})
		if err != nil {
			return checked, fmt.Errorf("checking repo %s: %w", url, err)
		}
		fmt.Printf("Repo %s exists, proceeding\n", url)
		fmt.Printf("ref is: %#v\n", ref) // ONLY FOR DEBUGGING
	}
	fmt.Println("All repos in the loaded configuration are reachable")
	return checked, nil
}

func Hosts(cfg *config.Config) error {
	fmt.Println("\nChecking all the hosts in the loaded configuration...")
	// Only the first hop can be dialed from here: every other machine in the
	// config, the master included, lives behind the one before it.
	if len(cfg.Jumps) > 0 {
		first := cfg.Jumps[0]
		return dial(first.IP, first.Name)
	}
	if err := dial(cfg.Master.IP, cfg.Master.Name); err != nil {
		return err
	}
	fmt.Println("All hosts in the loaded configuration are reachable")
	return nil
}

func dial(ip netip.Addr, name string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip.String(), sshPort), dialTimeout)
	if err != nil {
		return fmt.Errorf("host %s (%s) not responding: %w", ip, name, err)
	}
	err = conn.Close()
	if err != nil {
		return fmt.Errorf("problem closing a connection with host %s (%s)", ip, name)
	}
	fmt.Printf("Host %s (%s) is reachable, checking current configuration\n", ip, name)
	return nil
}

// THIS SHOULD NOT TAKE ANY PARAMS AFTER TESTING
func Diff(local string, repo string) (unsync map[string]string, orphans []string, error error) {
	unsync = make(map[string]string)
	orphans = make([]string, 0, 10) // len 0, cap = heuristic estimate

	localHashes, err := fileHashes(local) // ONLY FOR TESTING
	if err != nil {
		return nil, nil, fmt.Errorf("error getting the hashes from the machine: %w", err)
	}
	repoHashes, err := fileHashes(repo) // ONLY FOR TESTING
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

func fileHashes(basePath string) (map[string]string, error) {
	fileHashes := make(map[string]string) // RESEARCH HINT CAP

	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking entry: %w", err)
		}

		rel, err := filepath.Rel(basePath, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}

		// Skip the backups dir so prior backups aren't treated as orphaned files.
		if d.IsDir() && rel == "versions" {
			return filepath.SkipDir
		}

		if !d.IsDir() {
			hash, err := getHash(path)
			if err != nil {
				return err
			}
			fileHashes[rel] = hash
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directories: %w", err)
	}

	return fileHashes, nil
}

func getHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hashing: %w", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
