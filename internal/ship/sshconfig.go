package ship

import (
	"fmt"
	"os"
	"strings"

	"goship/internal/config"
)

// Machines are addressed by alias rather than by IP: ssh applies -l, -p and -i
// to the final destination only, so a per-machine Host block is the one place
// each one's user, port and key can be stated.
const masterAlias = "goship-master"

func jumpAlias(i int) string    { return fmt.Sprintf("goship-jump-%d", i+1) }
func subjectAlias(i int) string { return fmt.Sprintf("goship-subject-%d", i+1) }

// hop is what one Host block is built from: where the machine is, how to log
// into it, and which other block it is reached through. An empty via means it
// is dialed straight from here.
type hop struct {
	addr  string
	user  string
	port  int
	creds string
	via   string
}

// hopFile writes the ssh config every transfer of this deploy runs against and
// returns its path, which the caller owns and is expected to remove.
//
// One file covers the whole chain: ssh hands its own -F down to each ProxyJump
// command it spawns, so a machine three hops deep is still resolved against
// these same blocks.
func hopFile(cfg *config.Config) (string, error) {
	file, err := os.CreateTemp("", "goship-ssh-config-*")
	if err != nil {
		return "", fmt.Errorf("creating the ssh config for this deploy: %w", err)
	}

	if _, err := file.WriteString(hopConfig(cfg)); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", fmt.Errorf("writing the ssh config for this deploy: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("closing the ssh config for this deploy: %w", err)
	}
	return file.Name(), nil
}

// hopConfig renders the ssh config describing how every machine in cfg is
// reached: one Host block for each jump, the master and each subject, each of
// them naming the machine it is dialed through.
func hopConfig(cfg *config.Config) string {
	var b strings.Builder

	// Jumps are chained in the order they are listed, so only the first is
	// dialed directly and each of the rest hangs off the one before it.
	last := ""
	for i := range cfg.Jumps {
		jump := &cfg.Jumps[i]
		alias := jumpAlias(i)
		writeHost(&b, alias, hop{
			addr:  jump.IP.String(),
			user:  jump.User,
			port:  jump.Port,
			creds: jump.Creds,
			via:   last,
		})
		last = alias
	}

	master := &cfg.Master
	writeHost(&b, masterAlias, hop{
		addr:  master.IP.String(),
		user:  master.User,
		port:  master.Port,
		creds: master.Creds,
		via:   last,
	})

	// An ignored master exists to be the way into its subjects, so they are
	// reached through it. Otherwise they are expected to be reachable from
	// wherever the master itself is dialed from, which is the end of the chain.
	via := last
	if master.Ignore {
		via = masterAlias
	}
	for i := range master.Subjects {
		subject := &master.Subjects[i]
		writeHost(&b, subjectAlias(i), hop{
			addr: subject.IP.String(),
			user: subject.User,
			// Subjects carry no key of their own, so they are opened with the
			// master's, and no port either, so ssh falls back to 22.
			creds: master.Creds,
			via:   via,
		})
	}

	return b.String()
}

// writeHost appends to b the Host block that names h alias.
func writeHost(b *strings.Builder, alias string, h hop) {
	fmt.Fprintf(b, "Host %s\n", alias)
	fmt.Fprintf(b, "  HostName %s\n", h.addr)
	if len(h.user) > 0 {
		fmt.Fprintf(b, "  User %s\n", h.user)
	}
	if h.port > 0 {
		fmt.Fprintf(b, "  Port %d\n", h.port)
	}
	if len(h.creds) > 0 {
		// Quoted so a path with spaces in it survives, and IdentitiesOnly so
		// the configured key is the one actually offered: without it ssh tries
		// everything the agent holds first and can be turned away for too many
		// attempts before ever reaching this one.
		fmt.Fprintf(b, "  IdentityFile %q\n", h.creds)
		fmt.Fprint(b, "  IdentitiesOnly yes\n")
	}
	if len(h.via) > 0 {
		fmt.Fprintf(b, "  ProxyJump %s\n", h.via)
	}
	fmt.Fprintln(b)
}
