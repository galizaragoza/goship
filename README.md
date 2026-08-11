# goship

An early-stage Go CLI for shipping a git repository onto a fleet of machines over
SSH. It reads a YAML file describing the fleet, checks that every repo and the
first host are reachable, clones the repos to local scratch space, and pushes
them out with rsync.

```
goship path/to/config.yaml
```

The API surface is documented in the source. Run `go doc ./internal/...` for it;
this file covers the parts godoc cannot — the topology, the inheritance rules and
the lab.

## Topology

Three kinds of machine, each reached through the one before it:

```
goship ──> jump 1 ──> jump 2 ──> master ──> subjects
           (chained, optional)      │
                                    └─ when marked `ignore: true`,
                                       the master is only the way in
```

- **Jumps** are hops on the way to the master, for when it cannot be dialed
  directly. They are chained in listed order: the first is dialed from here, each
  of the rest through the one before it, and the master through the last. A jump
  is only ever connected *through*, never deployed *to* — so it carries no repo,
  dir or method.
- **Master** owns the subjects. It is normally a deploy target itself.
- **Subjects** are the machines under a master. They carry no key or port of
  their own: they are opened with the master's credentials on the default port.

`ignore: true` takes a machine out of the deploy without taking it out of the
topology. On a **master** it means "jump host only" — its subjects are still
reached through it, but nothing is written to it. On a **subject** it simply
skips that machine. It is deliberately **not** inherited: an ignored master
propagating the flag down would skip the very subjects it exists to reach.

Every machine gets its own generated `Host` block in a temporary SSH config,
addressed by alias rather than by IP, because `ssh` applies `-l`, `-p` and `-i`
to the final destination only. One file covers the whole chain — `ssh` hands its
`-F` down to each `ProxyJump` it spawns.

## Configuration

Values cascade down three levels, so each one only states what differs from the
level above it:

```
file-wide defaults ──> master ──> subjects
                  └──> jumps
```

| Level | Inherits |
| --- | --- |
| Jump | `protocol`, `user` (plus `port`, defaulting to 22) |
| Master | `protocol`, `user`, `repo`, `dir`, `method` (plus `port`, defaulting to 22) |
| Subject | `repo`, `dir`, `method`, `user` — from its **master**, after the master has taken its own defaults |

```yaml
mode: ship # ship | report | sync

# File-wide defaults. Anything below that omits one of these gets it from here.
protocol: ssh # ssh is the only supported protocol
user: root
repo: https://github.com/you/yourrepo.git
dir: /test/testdir
method: rsync # rsync | scp

# Optional. Omit entirely when the master is directly reachable.
jumps:
  - ip: 10.0.0.1
    name: bastion
    creds: ./keys/bastion_ed25519 # required: path to a private key
    port: 2222 # optional, defaults to 22
    # protocol and user fall back to the defaults above

master:
  ip: 172.21.0.10
  name: master.lab
  creds: ./testing/keys/id_ed25519 # required: used for its subjects too
  # ignore: true                   # deploy THROUGH me, not TO me
  # repo/dir/method/user fall back to the defaults above

  subjects:
    - ip: 172.20.0.11
      name: slave1.lab
      # everything falls back to the master

    - ip: 172.20.0.12
      name: slave2.lab
      dir: /somewhere/else # override just this one
      ignore: false
```

Validation runs *after* defaults are applied, so an inherited value is checked
exactly like a stated one. A jump needs an `ip`, a `user` and `creds`; a master
needs a `user`, and — unless ignored — a `repo`, a `dir` and a valid `method`; a
subject needs all four, unless it is ignored.

## Exit codes

| Code | Meaning |
| --- | --- |
| `1` | Bad invocation or unloadable config |
| `101` | A repo in the config could not be reached |
| `102` | The first host in the chain did not answer |
| `103` | A repo could not be cloned |
| `501` | Local error cleaning up the scratch clones |

`1XX` is a network error, `5XX` a local one.

## The test lab

`testing/` brings up six Alpine containers on two docker networks: a master
reachable from the host on `lab_edge` (`172.21.0.10`, also published on
`localhost:2222`) driving five clients that only exist on `lab_internal`
(`172.20.0.11-15`).

```sh
docker compose -f testing/compose.yaml up -d
go run . config.yaml
docker compose -f testing/compose.yaml down
```

The keys under `testing/keys/` are throwaway lab credentials.

## Status

Early stage. What works today:

- Config loading, defaulting and validation
- Repo and host reachability checks
- Cloning to scratch space
- Deploying over rsync through an arbitrary jump chain

Not implemented: `scp` as a transfer method (`method: scp` validates but always
rsyncs), the `report` mode, and the `sync` mode — `internal/sync` has the diffing
and backup halves but nothing reaches a subject's remote tree to compare against.
