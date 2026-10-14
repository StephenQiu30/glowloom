// Command identity runs bounded identity maintenance without starting application services.
package main

import (
	"fmt"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityoperator"
	"os"
)

func main() {
	if err := identityoperator.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "identity:", err)
		os.Exit(1)
	}
}
