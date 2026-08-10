# TL;DR

`nlgmonship` is an early-stage Go CLI for shipping monitoring-client repositories to a fleet
of hosts. It loads a YAML configuration describing repos and their target hosts, verifies that
each repo URL is reachable and each host responds, and can compute SHA-256 hashes of files
(the basis for a future config-diff step). The actual "ship" delivery logic is not implemented
yet (`internal/ship` is a stub).

Module: `nlgmonship` (Go 1.26.4). Entry point: `main.go`.

# Structure

```
nlgmonship/
├── main.go                  # entry point: load config, print it, run checks/diff
├── config.yaml              # example configuration
├── internal/
│   ├── config/config.go     # config structs + YAML loading, printing, confirmation prompt
│   ├── check/check.go       # reachability checks (repos, hosts) + file hashing/diff
│   └── ship/ship.go         # stub package (delivery logic — not implemented)
├── todo.md                  # roadmap + code-review findings
├── go.mod / go.sum
```

Packages:

- **`main`** — wires everything together: `config.Load` → `config.Print` → (`check.Repos`,
  `check.Hosts` currently commented out) → `check.Diff`. Defines `baseDir = "/apps/"`.
- **`internal/config`** — configuration data model and YAML parsing.
- **`internal/check`** — verification logic (repo URLs, host TCP reachability) and file hashing.
- **`internal/ship`** — placeholder for the repo-delivery logic; currently only an unused
  `baseDir` const.

Key external dependency: `github.com/go-git/go-git/v6` (used to list remote refs and confirm a
repo URL is reachable). YAML parsing via `gopkg.in/yaml.v3`.

# Types

All defined in `internal/config`.

### `Config`

| Field          | Type     | YAML key        | Meaning                                              |
| -------------- | -------- | --------------- | ---------------------------------------------------- |
| `Title`        | `string` | `title`         | Human-readable name for this configuration.          |
| `Repos`        | `[]Repo` | `repos`         | One or more repositories to ship.                    |
| `CheckTimeout` | `int`    | `check-timeout` | Timeout in **seconds** for host reachability checks. |

### `Repo`

| Field   | Type     | YAML key   | Meaning                              |
| ------- | -------- | ---------- | ------------------------------------ |
| `URL`   | `string` | `repo-url` | Git URL of the repo to ship.         |
| `Hosts` | `[]Host` | `hosts`    | Target hosts that receive this repo. |

### `Host`

| Field        | Type         | YAML key      | Meaning                                                             |
| ------------ | ------------ | ------------- | ------------------------------------------------------------------- |
| `IP`         | `netip.Addr` | `ip`          | IP address of the host (a single address; "range" is aspirational). |
| `Name`       | `string`     | `name`        | Host identifier/label.                                              |
| `Class`      | `string`     | `class`       | `"master"` (monitoring) or `"slave"` (being monitored).             |
| `SkipChecks` | `string`     | `skip-checks` | Flag to skip checks for this host.                                  |

# Functions

### `internal/config`

**`Load(path string) (*Config, error)`**
Reads the YAML file at `path` and unmarshals it into a `*Config`. Returns an error wrapping the
underlying failure on read (`reading config file: %w`) or unmarshal (`unmarshaling config: %w`).

**`Print(cfg *Config)`**
Prints the current configuration to stdout (title, then each repo with its hosts' name, IP and
class). No return value. Note: does not currently print `CheckTimeout` or `Host.SkipChecks`.

**`Confirm(prompt string) (res bool, err error)`**
Prompts the user `<prompt> [y/N]` on stdin and loops until a `y/Y` or `n/N` line is entered,
returning the corresponding boolean. Returns an error if stdin reading fails
(`reading user confirmation: %w`).

### `internal/check`

**`Repos(cfg *config.Config) error`**
For each repo in the config, lists the remote refs via go-git to confirm the URL is reachable.
Returns `checking repo %s: %w` on the first unreachable repo, otherwise `nil`.

**`Hosts(cfg *config.Config) error`**
For each host of each repo, attempts a TCP `DialTimeout` (timeout = `CheckTimeout` seconds).
Returns `host %s (%s) not responding: %w` on the first unreachable host, otherwise `nil`.

**`Diff(basePath string) (map[string]string, error)`**
Walks the directory tree under `basePath` and returns a map of `path → SHA-256 hex digest` for
every regular file encountered. Returns `walking directories: %w` (or a per-entry / hashing
error) on failure.
_Current behavior note:_ the function overrides `basePath` to `"./"` internally (a debug line
flagged for removal), so the argument is currently ignored.

**`getHash(path string) (string, error)`** _(unexported)_
Opens the file at `path`, streams it through `sha256`, and returns the hex-encoded digest.
Errors: `opening file for hashing: %w`, `hashing file: %w`.

# Config file

Annotated example (`config.yaml`):

```yaml
title: Example config # Config.Title
repos:
  - repo-url: https://repo.workondata.com/.../wod-monitor-client.git # Repo.URL
    hosts:
      - ip: 10.0.0.10 # Host.IP
        name: monitor-master # Host.Name
        class: master # Host.Class — monitoring node
      - ip: 10.0.0.11
        name: client-01
        class: slave # Host.Class — monitored node
      # ... more hosts
  # ... more repos
```

`check-timeout` (maps to `Config.CheckTimeout`, seconds) may also be set at the top level; it is
absent from the current example file.
