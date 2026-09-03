package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/oorrwullie/kolchak/internal/project"
)

const usage = `Kolchak — reliability testing for AI agents

Usage:
  kolchak init [directory]
  kolchak help
`

func Run(args []string, stdout, _ io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(stdout, usage)
		return err
	}

	switch args[0] {
	case "init":
		if len(args) > 2 {
			return errors.New("usage: kolchak init [directory]")
		}
		dir := "."
		if len(args) == 2 {
			dir = args[1]
		}
		if err := project.Init(dir); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "Initialized Kolchak in %s\n", dir)
		return err
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func Main(args []string) int {
	if err := Run(args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "kolchak:", err)
		return 1
	}
	return 0
}
