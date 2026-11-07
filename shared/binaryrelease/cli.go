package binaryrelease

import (
	"flag"
	"fmt"
	"io"
)

// CLI overrides are scoped to a single fixed-operation process invocation. Map
// membership preserves explicitly empty values instead of falling back to env.
var cliInputs map[string]string

// RunCLI parses only the named public inputs supplied by a dedicated entrypoint.
// Tokens are never CLI inputs. Parser diagnostics intentionally omit arguments.
func RunCLI(version string, args, names []string, stdout io.Writer, run func() error) error {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "--tool-version") {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	flags := flag.NewFlagSet("action", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	values := map[string]*string{}
	for _, name := range names {
		if name == "token" {
			return fmt.Errorf("secret CLI input is forbidden")
		}
		values[name] = flags.String(name, "", "named action input")
	}
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return fmt.Errorf("invalid command-line arguments")
	}
	overrides := map[string]string{}
	flags.Visit(func(f *flag.Flag) { overrides[f.Name] = *values[f.Name] })
	previous := cliInputs
	cliInputs = overrides
	defer func() { cliInputs = previous }()
	return run()
}
