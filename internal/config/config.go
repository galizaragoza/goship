// Package config contains the structs to be processed from the configuration file
// It is also responsible for processing YAML config files and turning them in structs
package config

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"

	"gopkg.in/yaml.v3"
)

// Config type has 2 fields: the Mode to run in and a list of 1 or more Masters, each one owning its own Subjects
type Config struct {
	Mode    string   `yaml:"mode"` // MUST be: report | sync | deploy
	Masters []Master `yaml:"master"`
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
	Ignore bool   `yaml:"ignore"`
	Repo   string `yaml:"repo"`
	Dir    string `yaml:"dir"`
	Method string `yaml:"method"` // MUST be: rsync | scp
	User   string `yaml:"user"`
}

type Master struct {
	Host     `yaml:",inline"`
	Subjects []Subject `yaml:"subjects"`
	Pathway  string    `yaml:"pathway"` // MUST be: ssh (for now)
	Port     int       `yaml:"port"`
	Creds    string    `yaml:"creds"` // while only ssh is supported, this must be a path to a key
}

type Subject struct {
	Host `yaml:",inline"`
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

	if cfg.Mode != "report" && cfg.Mode != "sync" && cfg.Mode != "deploy" {
		return nil, fmt.Errorf("review your config, task is %s and should be either report, sync or deploy", cfg.Mode)
	}

	for i := range cfg.Masters {
		master := &cfg.Masters[i]
		if master.Pathway != "ssh" {
			return nil, fmt.Errorf("only ssh is supported right now")
		}
		if len(master.Subjects) == 0 {
			return nil, fmt.Errorf("you need at least 1 subject for each master to run goship")
		}
		if master.Port == 0 {
			master.Port = 22
		}
		// Required either way: a master is connected to whether it is a deploy
		// target or only the jump host its subjects are reached through.
		if len(master.User) == 0 {
			return nil, fmt.Errorf("master %s: no user to connect as", master.Name)
		}
		// An ignored master receives nothing, so its repo, dir and method are
		// only defaults for its subjects and may be left out entirely.
		if !master.Ignore {
			if err := validMethod(master.Method); err != nil {
				return nil, fmt.Errorf("master %s: %w", master.Name, err)
			}
			if len(master.Repo) == 0 {
				return nil, fmt.Errorf("master %s: no repo to deploy, set one or mark the master as ignored", master.Name)
			}
			if len(master.Dir) == 0 {
				return nil, fmt.Errorf("master %s: no dir to deploy into, set one or mark the master as ignored", master.Name)
			}
		}
		for j := range master.Subjects {
			subject := &master.Subjects[j]
			subject.inheritFrom(&master.Host)

			// Nothing is deployed to an ignored subject, so what it would have
			// been deployed with does not have to add up.
			if subject.Ignore {
				continue
			}

			// Checked after inheritance so subjects falling back to the
			// master's values are validated too.
			if err := validMethod(subject.Method); err != nil {
				return nil, fmt.Errorf("subject %s: %w", subject.Name, err)
			}
			if len(subject.User) == 0 {
				return nil, fmt.Errorf("subject %s: no user to connect as, set it on the subject or its master", subject.Name)
			}
			if len(subject.Repo) == 0 {
				return nil, fmt.Errorf("subject %s: no repo to deploy, set it on the subject or its master", subject.Name)
			}
			if len(subject.Dir) == 0 {
				return nil, fmt.Errorf("subject %s: no dir to deploy into, set it on the subject or its master", subject.Name)
			}
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
	for _, master := range cfg.Masters {
		fmt.Printf("\tMaster: %s\n", master.Name)
		fmt.Printf("\t\tIP: %s\n", master.IP)
		fmt.Printf("\t\tIgnore: %t\n", master.Ignore)
		fmt.Printf("\t\tPathway: %s\n", master.Pathway)
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
