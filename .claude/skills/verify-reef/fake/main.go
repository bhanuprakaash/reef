// Command fake is a scripted stand-in for the claude CLI, used by verify-reef so its
// recipes do not need a real model call. It replays recorded stream-json transcripts
// (fake.go) or, in record mode, runs the real claude and saves what it said (record.go).
package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("0.0.0 (Claude Code) fake")
		return
	}
	if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
		fmt.Println(`{"loggedIn":true,"authMethod":"fake"}`)
		return
	}

	var err error
	if os.Getenv("REEF_VERIFY_CLAUDE") == "record" {
		err = runRecord(args)
	} else {
		err = runFake(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude:", err)
		os.Exit(1)
	}
}

// cliArgs is the subset of claude's flags the fake reads from argv.
type cliArgs struct {
	mcpConfig string
	resume    string
}

func parseArgs(args []string) cliArgs {
	var c cliArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mcp-config":
			if i+1 < len(args) {
				i++
				c.mcpConfig = args[i]
			}
		case "--resume":
			if i+1 < len(args) {
				i++
				c.resume = args[i]
			}
		case "--permission-mode", "--allowedTools", "--disallowedTools":
			i++ // value is unused by the fake
		}
	}
	return c
}
