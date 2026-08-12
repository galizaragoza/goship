// Package config defines the configuration model and loads it from YAML:
// parsing the file, filling in what it left unsaid, and validating the result.
package config

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strconv"

	"goship/internal/out"

	"gopkg.in/yaml.v3"
)

// DefaultPort is what a jump or a master is reached on when the config names
// no port. It is the ssh port, as ssh is the only supported protocol.
const DefaultPort = 22

var (
	supportedProtocols = []string{"ssh"}
	supportedModes     = []string{"ship", "report", "sync"}
	supportedMethods   = []string{"rsync", "scp"}
)

// Config is one whole configuration file: the Mode to run in, the Master
// owning the Subjects, the Jumps it is reached through, and the Defaults the
// rest of the file falls back to.
type Config struct {
	Mode string `yaml:"mode"` // MUST be: report | sync | ship

	Defaults `yaml:",inline"`

	Jumps  []Jump `yaml:"jumps,omitempty"`
	Master Master `yaml:"master"`
}

// Defaults are the file-wide fallbacks: a jump, the master or a subject that
// leaves one of them out gets the value from here, and one that sets it
// overrides it. They are what keeps a deploy of the same repo, into the same
// dir, as the same user, over the same protocol from repeating itself on every
// host.
type Defaults struct {
	Protocol string `yaml:"protocol,omitempty"` // MUST be: ssh (for now)
	User     string `yaml:"user,omitempty"`
	Repo     string `yaml:"repo,omitempty"`
	Dir      string `yaml:"dir,omitempty"`
	Method   string `yaml:"method,omitempty"`
}

// asHost renders the defaults as the Host the master inherits from, so the top
// of the file and a master both feed the same inheritFrom.
func (d Defaults) asHost() Host {
	return Host{Repo: d.Repo, Dir: d.Dir, Method: d.Method, User: d.User}
}

// Host holds the fields a master and a subject have in common: where to reach
// the machine and what to put on it. It is embedded with yaml:",inline" so its
// keys stay flat in the config file, at the same level as the parent's own.
type Host struct {
	IP   netip.Addr `yaml:"ip"`
	Name string     `yaml:"name"`
	// Ignore takes the host out of the deploy without taking it out of the
	// topology. On a master it means "jump host only": its subjects are still
	// reached through it, but nothing is written to it. On a subject it simply
	// skips that machine.
	Ignore bool   `yaml:"ignore,omitempty"`
	Repo   string `yaml:"repo,omitempty"`
	Dir    string `yaml:"dir,omitempty"`
	Method string `yaml:"method,omitempty"` // MUST be: rsync | scp
	User   string `yaml:"user,omitempty"`
}

type Master struct {
	Host     `yaml:",inline"`
	Subjects []Subject `yaml:"subjects"`
	Protocol string    `yaml:"protocol,omitempty"` // MUST be "ssh"
	Port     int       `yaml:"port,omitempty"`
	Creds    string    `yaml:"creds"` // must be a path to a key
}

type Subject struct {
	Host `yaml:",inline"`
}

// Jump is one hop on the way to the master, for when it cannot be reached
// straight from the machine running goship. Jumps are chained in the order they
// are listed: the first is dialed directly, each of the rest through the one
// before it, and the master through the last one.
//
// It does not embed Host because repo, dir, method and ignore mean nothing on a
// machine that is only ever connected through, never deployed to.
type Jump struct {
	IP       netip.Addr `yaml:"ip"`
	Name     string     `yaml:"name"`
	Protocol string     `yaml:"protocol,omitempty"` // MUST be: ssh (for now)
	Port     int        `yaml:"port,omitempty"`
	User     string     `yaml:"user,omitempty"`
	Creds    string     `yaml:"creds"` // while only ssh is supported, this must be a path to a key
}

// orElse sets dst to src when dst is still at its zero value, which is how the
// config says a value was left unstated.
func orElse[T comparable](dst *T, src T) {
	var zero T
	if *dst == zero {
		*dst = src
	}
}

// inheritFrom fills the fields left empty on h with o's, so each level of the
// config only has to state what differs from the one above it.
//
// Ignore is deliberately not inherited: an ignored master means "deploy through
// me, not to me", so propagating the flag down would skip the very subjects it
// exists to reach. A subject opts out only by setting ignore on itself.
func (h *Host) inheritFrom(o *Host) {
	orElse(&h.Repo, o.Repo)
	orElse(&h.Dir, o.Dir)
	orElse(&h.Method, o.Method)
	orElse(&h.User, o.User)
}

// applyDefaults walks the config filling in what was left unsaid: the file-wide
// defaults reach the jumps and the master, and the master's own values reach
// its subjects.
func (c *Config) applyDefaults() {
	for i := range c.Jumps {
		jump := &c.Jumps[i]
		orElse(&jump.Protocol, c.Protocol)
		orElse(&jump.User, c.User)
		orElse(&jump.Port, DefaultPort)
	}

	base := c.Defaults.asHost()
	master := &c.Master
	master.inheritFrom(&base)
	// Protocol and Port sit on Master rather than on Host, so they fall back
	// separately from the fields inheritFrom covers.
	orElse(&master.Protocol, c.Protocol)
	orElse(&master.Port, DefaultPort)

	// Done after the master has taken its own defaults, so a value stated only
	// at the top of the file still travels all the way down to the subjects.
	for i := range master.Subjects {
		master.Subjects[i].inheritFrom(&master.Host)
	}
}

// requireFields returns an error for the first of the given pairs whose value
// is empty, naming the scope it belongs to. Pairs read (what is missing, value).
func requireFields(scope string, kv ...string) error {
	for i := 0; i < len(kv); i += 2 {
		if len(kv[i+1]) == 0 {
			return fmt.Errorf("%s: no %s set", scope, kv[i])
		}
	}
	return nil
}

// validMethod reports whether m is a transfer method the ship package supports.
func validMethod(m string) error {
	if !slices.Contains(supportedMethods, m) {
		return fmt.Errorf("method is %q and should be one of: %v", m, supportedMethods)
	}
	return nil
}

// Load reads the YAML config at path and returns it parsed, with the file-wide
// defaults applied and every resulting value validated.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if !slices.Contains(supportedModes, cfg.Mode) {
		return nil, fmt.Errorf("review your config, task mode is %s and should be one of these: %v", cfg.Mode, supportedModes)
	}

	// Everything below is checked after the defaults are in place, so a value
	// taken from the top of the file is validated exactly like a stated one.
	cfg.applyDefaults()

	for i := range cfg.Jumps {
		jump := &cfg.Jumps[i]
		scope := "jump " + jump.Name
		if !slices.Contains(supportedProtocols, jump.Protocol) {
			return nil, fmt.Errorf("%s: protocol %s is not valid, valid options are: %v", scope, jump.Protocol, supportedProtocols)
		}
		if !jump.IP.IsValid() {
			return nil, fmt.Errorf("%s: no ip to reach it at", scope)
		}
		if err := requireFields(scope,
			"user to connect as", jump.User,
			"creds to connect with", jump.Creds); err != nil {
			return nil, err
		}
	}

	master := &cfg.Master
	scope := "master " + master.Name
	if !slices.Contains(supportedProtocols, master.Protocol) {
		return nil, fmt.Errorf("%s: protocol %s is not supported, current options are: %v", scope, master.Protocol, supportedProtocols)
	}
	if len(master.Subjects) == 0 {
		return nil, fmt.Errorf("you need at least 1 subject to run goship")
	}
	// Required either way: a master is connected to whether it is a deploy
	// target or only the jump host its subjects are reached through.
	if err := requireFields(scope, "user to connect as", master.User); err != nil {
		return nil, err
	}
	// An ignored master receives nothing, so its repo, dir and method are only
	// defaults for its subjects and may be left out entirely.
	if !master.Ignore {
		if err := validMethod(master.Method); err != nil {
			return nil, fmt.Errorf("%s: %w", scope, err)
		}
		if err := requireFields(scope,
			"repo to deploy", master.Repo,
			"dir to deploy into", master.Dir); err != nil {
			return nil, err
		}
	}

	for i := range master.Subjects {
		subject := &master.Subjects[i]

		// Nothing is deployed to an ignored subject, so what it would have been
		// deployed with does not have to add up.
		if subject.Ignore {
			continue
		}

		scope := "subject " + subject.Name
		if err := validMethod(subject.Method); err != nil {
			return nil, fmt.Errorf("%s: %w", scope, err)
		}
		if err := requireFields(scope,
			"user to connect as", subject.User,
			"repo to deploy", subject.Repo,
			"dir to deploy into", subject.Dir); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

// section writes a titled block at the given indent depth, followed by its
// name/value pairs one level deeper and a blank line, so that neighbouring
// blocks do not run into one another.
func section(depth int, title string, kv ...string) {
	out.Line(depth, "%s", title)
	for i := 0; i < len(kv); i += 2 {
		out.Line(depth+1, "%s: %s", kv[i], kv[i+1])
	}
	fmt.Println()
}

// Print writes the resolved configuration to stdout, under whatever section
// the caller has opened.
func Print(cfg *Config) {
	out.Item("Mode: %s", cfg.Mode)
	fmt.Println()

	section(1, "Defaults:",
		"Protocol", cfg.Protocol,
		"User", cfg.User,
		"Repo", cfg.Repo,
		"Dir", cfg.Dir)

	for i, jump := range cfg.Jumps {
		section(1, fmt.Sprintf("Jump %d of %d: %s", i+1, len(cfg.Jumps), jump.Name),
			"IP", jump.IP.String(),
			"Protocol", jump.Protocol,
			"Port", strconv.Itoa(jump.Port),
			"User", jump.User,
			"Creds", jump.Creds)
	}

	master := &cfg.Master
	section(1, "Master: "+master.Name,
		"IP", master.IP.String(),
		"Ignore", strconv.FormatBool(master.Ignore),
		"Protocol", master.Protocol,
		"Port", strconv.Itoa(master.Port),
		"Creds", master.Creds,
		"Repo", master.Repo,
		"Dir", master.Dir,
		"Method", master.Method,
		"User", master.User)

	for _, subject := range master.Subjects {
		section(2, "Subject: "+subject.Name,
			"IP", subject.IP.String(),
			"Ignore", strconv.FormatBool(subject.Ignore),
			"Repo", subject.Repo,
			"Dir", subject.Dir,
			"Method", subject.Method,
			"User", subject.User)
	}
}

// Confirm asks the user to approve prompt, looping until they answer. An empty
// line counts as yes.
func Confirm(prompt string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("  %s [y/N] ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("reading user confirmation: %w", err)
		}

		switch input {
		case "y\n", "Y\n", "\n":
			fmt.Printf("\n\n")
			return true, nil
		case "n\n", "N\n":
			fmt.Printf("\n\n")
			return false, nil
		}
	}
}
