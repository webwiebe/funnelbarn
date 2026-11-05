package main

import (
	"fmt"
	"io"
)

// usageText lists the subcommands. Running the binary with no argument, or
// with `serve`, starts the server.
const usageText = "usage: funnelbarn [serve | version | worker-once | replay-dead-letter | user | project | apikey | recordings | help]"

// argAction is what the first command-line argument asks the process to do.
type argAction int

const (
	// actionServe starts the server (no argument, or `serve`).
	actionServe argAction = iota
	// actionSubcommand is one of the admin subcommands dispatched by run.
	actionSubcommand
	// actionHelp prints the usage and exits 0.
	actionHelp
	// actionUnknown prints the usage and exits 1.
	actionUnknown
)

// classifyArgs maps os.Args[1:] to an action. An argument the binary does not
// know never starts a server: a second server process started by accident
// (for example `kubectl exec ... funnelbarn help`) touches the live pod's
// spool and cursor (#332).
func classifyArgs(args []string) argAction {
	if len(args) == 0 {
		return actionServe
	}
	switch args[0] {
	case "serve":
		return actionServe
	case "help", "-h", "--help":
		return actionHelp
	}
	if _, ok := subcommands[args[0]]; ok {
		return actionSubcommand
	}
	return actionUnknown
}

// handleUsageArgs prints the usage for help and unknown arguments. It reports
// whether the arguments were consumed and the exit code to use when they were.
func handleUsageArgs(args []string, stdout, stderr io.Writer) (handled bool, code int) {
	switch classifyArgs(args) {
	case actionHelp:
		_, _ = fmt.Fprintln(stdout, usageText)
		return true, 0
	case actionUnknown:
		_, _ = fmt.Fprintf(stderr, "funnelbarn: unknown argument %q\n%s\n", args[0], usageText)
		return true, 1
	}
	return false, 0
}
