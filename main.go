package main

import (
	"fmt"
	"os"

	"goship/internal/check"
	"goship/internal/config"
	"goship/internal/fetch"
	"goship/internal/ship"
)

const report string = "./goship.report"

// cleanup removes whatever the run has created so far. It is replaced once
// there are cloned repos to remove, and fail invokes it on every exit path.
var cleanup = func() {}

// fail reports the message on stderr, cleans up and exits with code. os.Exit
// skips deferred calls, so routing every exit through here is what keeps the
// cleanup from being forgotten. 1XX codes indicate network errors, 5XX local
// ones.
func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	cleanup()
	os.Exit(code)
}

func main() {
	if len(os.Args) < 2 {
		fail(1, "not enough arguments to run: a path to a config file is required")
	}
	cfg, err := config.Load(os.Args[1])
	if err != nil {
		fail(1, "%v", err)
	}

	fmt.Println("Configuration file loaded correctly")

	config.Print(cfg)

	/* COMENTADO PARA HACER PRUEBAS
	res, err := config.Confirm("Proceed with current configuration?")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	if !res {
		fmt.Println("Exiting")
		os.Exit(1)
	}
	*/

	checked, err := check.Repos(cfg)
	if err != nil {
		fail(101, "%v", err)
	}

	if err := check.Hosts(cfg); err != nil {
		fail(102, "%v", err)
	}

	// downRepos maps each repo URL to the temp dir it was cloned into.
	downRepos := make(map[string]string)
	cleanup = func() {
		for _, dir := range downRepos {
			if err := os.RemoveAll(dir); err != nil {
				fmt.Fprintf(os.Stderr, "error cleaning up the downloaded repo: %v\n", err)
				os.Exit(501)
			}
		}
	}
	defer cleanup()

	for url := range checked {
		dir, err := fetch.Repo(url)
		if err != nil {
			fail(103, "problem fetching one of the repos listed in the configuration: %v", err)
		}
		fmt.Printf("repo %s is fetched in %s\n", url, dir)
		downRepos[url] = dir
	}

	switch cfg.Mode {
	case "report":

	case "sync":
		// sync.Diff still compares two local dirs and has no way to reach a
		// subject's remote tree yet, so there is nothing to wire it to here.
		fmt.Fprintln(os.Stderr, "sync mode is not implemented yet")

	case "deploy":
		if err := ship.All(cfg, downRepos); err != nil {
			fmt.Fprintf(os.Stderr, "error deploying directories: %v\n", err)
		}
	}
}
