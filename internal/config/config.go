// Package config contains the structs to be processed from the configuration file
// It is also responsible for processing YAML config files and turning them in structs
package config

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

// defaultPort is what a jump or a master is reached on when the config names
// no port. It is the ssh port, as ssh is the only supported protocol.
const defaultPort = 22

var (
	supportedProtocols = []string{"ssh"}
	supportedMethods   = []string{"deploy"}
)

// Config type holds the Mode to run in, the Master owning the Subjects, the
// Jumps the master is reached through and the values the rest of the file
// falls back to.
type Config struct {
	Mode string `yaml:"mode"` // MUST be: report | sync | deploy

	// Protocol, User, Repo and Dir are the defaults for the file as a whole:
	// a jump, the master or a subject that leaves one of them out gets the
	// value from here, and one that sets it overrides it. They are what keeps
	// a deploy of the same repo, into the same dir, as the same user, over the
	// same protocol from repeating itself on every host.
	Protocol string `yaml:"protocol,omitempty"` // MUST be: ssh (for now)
	User     string `yaml:"user,omitempty"`
	Repo     string `yaml:"repo,omitempty"`
	Dir      string `yaml:"dir,omitempty"`
	Method   string `yaml:"metho,omitempty"`

	Jumps  []Jump `yaml:"jumps,omitempty"`
	Master Master `yaml:"master"`
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
	Protocol string    `yaml:"protocol,omitempty"` // MUST be: ssh (for now)
	Port     int       `yaml:"port,omitempty"`
	Creds    string    `yaml:"creds"` // while only ssh is supported, this must be a path to a key
}

type Subject struct {
	Host `yaml:",inline"`
}

// Jump is one hop on the way to the master, for when it cannot be reached
// straight from the machine running goship. Jumps are chained in the order
// they are listed: the first is dialed directly, each of the rest through the
// one before it, and the master through the last one.
//
// A jump is only ever connected through, never deployed to, so it carries what
// it takes to open the connection and nothing about what to leave behind. That
// is also why it does not embed Host: repo, dir, method and ignore mean
// nothing on a machine that is just the way in.
type Jump struct {
	IP       netip.Addr `yaml:"ip"`
	Name     string     `yaml:"name"`
	Protocol string     `yaml:"protocol,omitempty"` // MUST be: ssh (for now)
	Port     int        `yaml:"port,omitempty"`
	User     string     `yaml:"user,omitempty"`
	Creds    string     `yaml:"creds"` // while only ssh is supported, this must be a path to a key
}

// applyDefaults walks the config filling in what was left unsaid: the
// file-wide defaults reach the jumps and the master, and the master's own
// values reach its subjects, so each level only has to state what differs from
// the one above it.
func (c *Config) applyDefaults() {
	for i := range c.Jumps {
		jump := &c.Jumps[i]
		if len(jump.Protocol) == 0 {
			jump.Protocol = c.Protocol
		}
		if len(jump.User) == 0 {
			jump.User = c.User
		}
		if jump.Port == 0 {
			jump.Port = defaultPort
		}
	}

	master := &c.Master
	if len(master.Protocol) == 0 {
		master.Protocol = c.Protocol
	}
	if len(master.User) == 0 {
		master.User = c.User
	}
	if len(master.Repo) == 0 {
		master.Repo = c.Repo
	}
	if len(master.Dir) == 0 {
		master.Dir = c.Dir
	}
	if len(master.Method) == 0 {
		master.Method = c.Method
	}
	if master.Port == 0 {
		master.Port = defaultPort
	}

	// Done after the master has taken its own defaults, so a value stated only
	// at the top of the file still travels all the way down to the subjects.
	for i := range master.Subjects {
		master.Subjects[i].inheritFrom(&master.Host)
	}
}

// inheritFrom fills the fields left empty on s with the values of the master
// that owns it, so a config only has to state what differs per subject.
//
// Ignore is deliberately not inherited: an ignored master means "deploy through
// me, not to me", so propagating the flag down would skip the very subjects it
// exists to reach. A subject opts out only by setting ignore on itself.
func (s *Subject) inheritFrom(m *Host) {
	if len(s.Repo) == 0 {
		s.Repo = m.Repo
	}
	if len(s.Dir) == 0 {
		s.Dir = m.Dir
	}
	if len(s.Method) == 0 {
		s.Method = m.Method
	}
	if len(s.User) == 0 {
		s.User = m.User
	}
}

// Load func receives a path (string) to a YAML config file and returns either a parsed Config (struct) or an error
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if !slices.Contains(supportedMethods, cfg.Mode) {
		return nil, fmt.Errorf("review your config, task mode is %s and should be one of these: %v", cfg.Mode, supportedMethods)
	}

	// Everything below is checked after the defaults are in place, so a value
	// taken from the top of the file is validated exactly like a stated one.
	cfg.applyDefaults()

	for i := range cfg.Jumps {
		jump := &cfg.Jumps[i]
		if !slices.Contains(supportedProtocols, jump.Protocol) {
			return nil, fmt.Errorf("jump %s: method %s is not valid, valid options are: %v", jump.Name, jump.Protocol, supportedProtocols)
		}
		if !jump.IP.IsValid() {
			return nil, fmt.Errorf("jump %s: no ip to reach it at", jump.Name)
		}
		if len(jump.User) == 0 {
			return nil, fmt.Errorf("jump %s: no user to connect as, set one on the jump or as a default", jump.Name)
		}
		if len(jump.Creds) == 0 {
			return nil, fmt.Errorf("jump %s: no creds to connect with", jump.Name)
		}
	}

	master := &cfg.Master
	if !slices.Contains(supportedProtocols, master.Protocol) {
		return nil, fmt.Errorf("method %s is not supported, current options are: %v", master.Protocol, supportedProtocols)
	}
	if len(master.Subjects) == 0 {
		return nil, fmt.Errorf("you need at least 1 subject to run goship")
	}
	// Required either way: a master is connected to whether it is a deploy
	// target or only the jump host its subjects are reached through.
	if len(master.User) == 0 {
		return nil, fmt.Errorf("master %s: no user to connect as, set one on the master or as a default", master.Name)
	}
	// An ignored master receives nothing, so its repo, dir and method are
	// only defaults for its subjects and may be left out entirely.
	if !master.Ignore {
		if err := validMethod(master.Method); err != nil {
			return nil, fmt.Errorf("master %s: %w", master.Name, err)
		}
		if len(master.Repo) == 0 {
			return nil, fmt.Errorf("master %s: no repo to deploy, set one, set a default or mark the master as ignored", master.Name)
		}
		if len(master.Dir) == 0 {
			return nil, fmt.Errorf("master %s: no dir to deploy into, set one, set a default or mark the master as ignored", master.Name)
		}
	}
	for j := range master.Subjects {
		subject := &master.Subjects[j]

		// Nothing is deployed to an ignored subject, so what it would have
		// been deployed with does not have to add up.
		if subject.Ignore {
			continue
		}

		if err := validMethod(subject.Method); err != nil {
			return nil, fmt.Errorf("subject %s: %w", subject.Name, err)
		}
		if len(subject.User) == 0 {
			return nil, fmt.Errorf("subject %s: no user to connect as, set it on the subject, on its master or as a default", subject.Name)
		}
		if len(subject.Repo) == 0 {
			return nil, fmt.Errorf("subject %s: no repo to deploy, set it on the subject, on its master or as a default", subject.Name)
		}
		if len(subject.Dir) == 0 {
			return nil, fmt.Errorf("subject %s: no dir to deploy into, set it on the subject, on its master or as a default", subject.Name)
		}
	}

	return &cfg, nil
}

// validMethod reports whether m is a transfer method the deploy package supports.
func validMethod(m string) error {
	if m != "rsync" && m != "scp" {
		return fmt.Errorf("method is %q and should be either rsync or scp", m)
	}
	return nil
}

// Print func just prints the current configuration, it takes a Config (struct) and does not return anything
func Print(cfg *Config) {
	fmt.Println("\nCurrent configuration:")
	fmt.Printf("Mode: %s\n", cfg.Mode)
	fmt.Println("\tDefaults:")
	fmt.Printf("\t\tProtocol: %s\n", cfg.Protocol)
	fmt.Printf("\t\tUser: %s\n", cfg.User)
	fmt.Printf("\t\tRepo: %s\n", cfg.Repo)
	fmt.Printf("\t\tDir: %s\n", cfg.Dir)
	for i, jump := range cfg.Jumps {
		fmt.Printf("\tJump %d of %d: %s\n", i+1, len(cfg.Jumps), jump.Name)
		fmt.Printf("\t\tIP: %s\n", jump.IP)
		fmt.Printf("\t\tProtocol: %s\n", jump.Protocol)
		fmt.Printf("\t\tPort: %d\n", jump.Port)
		fmt.Printf("\t\tUser: %s\n", jump.User)
		fmt.Printf("\t\tCreds: %s\n", jump.Creds)
	}
	master := &cfg.Master
	fmt.Printf("\tMaster: %s\n", master.Name)
	fmt.Printf("\t\tIP: %s\n", master.IP)
	fmt.Printf("\t\tIgnore: %t\n", master.Ignore)
	fmt.Printf("\t\tProtocol: %s\n", master.Protocol)
	fmt.Printf("\t\tPort: %d\n", master.Port)
	fmt.Printf("\t\tCreds: %s\n", master.Creds)
	fmt.Printf("\t\tRepo: %s\n", master.Repo)
	fmt.Printf("\t\tDir: %s\n", master.Dir)
	fmt.Printf("\t\tMethod: %s\n", master.Method)
	fmt.Printf("\t\tUser: %s\n", master.User)
	for _, subject := range master.Subjects {
		fmt.Printf("\t\tSubject: %s\n", subject.Name)
		fmt.Printf("\t\t\tIP: %s\n", subject.IP)
		fmt.Printf("\t\t\tIgnore: %t\n", subject.Ignore)
		fmt.Printf("\t\t\tRepo: %s\n", subject.Repo)
		fmt.Printf("\t\t\tDir: %s\n", subject.Dir)
		fmt.Printf("\t\t\tMethod: %s\n", subject.Method)
		fmt.Printf("\t\t\tUser: %s\n", subject.User)
	}
}

// Confirm func just asks the user to confirm if he wants to use the loaded configuration before proceeding
// It takes a confirmation prompt of type string and returns if the user consented or not (type bool) and an error
func Confirm(prompt string) (res bool, err error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [y/N]", prompt)
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
