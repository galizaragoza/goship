package main

import (
	"fmt"
	"os"

	"nlgmonship/internal/check"
	"nlgmonship/internal/config"
	"nlgmonship/internal/deploy"
	"nlgmonship/internal/fetch"
)

const (
	logDir      string = "/var/tmp/nlgmonship/"
	orphanedLog string = logDir + "orphaned_files"
)

func main() {
	/*
		 err := os.Mkdir(logDir, 0o755)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
	*/

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "not enough arguments to run: a path to a config file is required")
		os.Exit(1)
	}
	cfg, err := config.Load(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
		fmt.Fprintln(os.Stderr, err)
	}

	err = check.Hosts(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// downRepos maps each repo URL to the temp dir it was cloned into.
	downRepos := make(map[string]string)
	// os.Exit skips deferred calls, so cleanup is also invoked explicitly on
	// every early exit past this point.
	cleanup := func() {
		for _, dir := range downRepos {
			err := os.RemoveAll(dir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error cleaning up the downloaded repo: %v\n", err)
				os.Exit(1)
			}
		}
	}
	defer cleanup()

	for url := range checked {
		dir, err := fetch.Repo(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "problem fetching one of the repos listed in the configuration: %v\n", err)
			cleanup()
			os.Exit(1)
		}
		fmt.Printf("repo %s is fetched in %s\n", url, dir)
		downRepos[url] = dir
	}

	switch cfg.Mode {
	case "report":

	case "sync":
		// check.Diff still compares two local dirs and has no way to reach a
		// subject's remote tree yet, so there is nothing to wire it to here.
		fmt.Fprintln(os.Stderr, "sync mode is not implemented yet")

	case "deploy":
		err := deploy.All(cfg, downRepos)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error deploying directories: %v\n", err)
		}
	}
}
