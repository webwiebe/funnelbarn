package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestClassifyArgsStartsTheServerOnlyForNoArgumentOrServe(t *testing.T) {
	cases := []struct {
		args []string
		want argAction
	}{
		{nil, actionServe},
		{[]string{"serve"}, actionServe},
		{[]string{"version"}, actionSubcommand},
		{[]string{"--version"}, actionSubcommand},
		{[]string{"worker-once"}, actionSubcommand},
		{[]string{"replay-dead-letter", "--all"}, actionSubcommand},
		{[]string{"user", "list"}, actionSubcommand},
		{[]string{"project"}, actionSubcommand},
		{[]string{"apikey"}, actionSubcommand},
		{[]string{"recordings", "scan-url-params"}, actionSubcommand},
		{[]string{"help"}, actionHelp},
		{[]string{"-h"}, actionHelp},
		{[]string{"--help"}, actionHelp},
		{[]string{"bogus"}, actionUnknown},
		{[]string{"Serve"}, actionUnknown},
		{[]string{"-x"}, actionUnknown},
	}
	for _, c := range cases {
		if got := classifyArgs(c.args); got != c.want {
			t.Errorf("classifyArgs(%q) = %d, want %d", c.args, got, c.want)
		}
	}
}

func TestHandleUsageArgsExitCodes(t *testing.T) {
	var out, errOut bytes.Buffer

	if handled, code := handleUsageArgs([]string{"help"}, &out, &errOut); !handled || code != 0 {
		t.Errorf("help: handled=%v code=%d, want handled with exit 0", handled, code)
	}
	if !strings.Contains(out.String(), "usage: funnelbarn") || errOut.Len() != 0 {
		t.Errorf("help wrote stdout=%q stderr=%q, want the usage on stdout only", out.String(), errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if handled, code := handleUsageArgs([]string{"bogus"}, &out, &errOut); !handled || code != 1 {
		t.Errorf("unknown: handled=%v code=%d, want handled with exit 1", handled, code)
	}
	if !strings.Contains(errOut.String(), `"bogus"`) || !strings.Contains(errOut.String(), "usage: funnelbarn") || out.Len() != 0 {
		t.Errorf("unknown wrote stdout=%q stderr=%q, want the argument and usage on stderr only", out.String(), errOut.String())
	}

	for _, args := range [][]string{nil, {"serve"}, {"user", "list"}} {
		if handled, _ := handleUsageArgs(args, &out, &errOut); handled {
			t.Errorf("%q must fall through to the server or its subcommand", args)
		}
	}
}

// Every subcommand the binary runs is accepted on the command line and named
// in the usage text. The recordings subcommand shipped once without the
// first, so it was refused as an unknown argument.
func TestEverySubcommandIsAcceptedAndListed(t *testing.T) {
	for name := range subcommands {
		if got := classifyArgs([]string{name}); got != actionSubcommand {
			t.Errorf("classifyArgs(%q) = %d, want actionSubcommand", name, got)
		}
		if strings.HasPrefix(name, "-") {
			continue
		}
		if !strings.Contains(usageText, " "+name+" ") {
			t.Errorf("usage text does not list %q: %s", name, usageText)
		}
	}
}
