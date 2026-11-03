package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	skillapp "github.com/StephenQiu30/glowloom-server/backend/internal/skill/application"
)

// runSandbox handles the `glowloom sandbox [serve]` subcommand.
func runSandbox(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: glowloom sandbox serve [--port=port]")
	}
	sub := args[0]
	switch sub {
	case "serve":
		flags := flag.NewFlagSet("sandbox serve", flag.ContinueOnError)
		port := flags.String("port", "19090", "sandbox listening port")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Glowloom Sandbox listening on port %s (gVisor/restricted execution ready)\n", *port)
		return skillapp.SandboxServeCommand(args[1:])
	default:
		return fmt.Errorf("unknown sandbox subcommand: %q", sub)
	}
}
