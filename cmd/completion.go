package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var (
	completionCmd = &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}

	shells = map[string]func(io.Writer) error{
		"bash": rootCmd.GenBashCompletion,
		"zsh":  rootCmd.GenZshCompletion,
	}
)

func init() {
	rootCmd.AddCommand(completionCmd)

	for shell, generator := range shells {
		completionCmd.AddCommand(&cobra.Command{
			Use:     shell,
			Short:   fmt.Sprintf("Generate a %s completion script", shell),
			Example: fmt.Sprintf("source <(tumblr-scraper completion %s)", shell),
			RunE:    makeCompletionCommandRunner(generator),
		})
	}
}

func makeCompletionCommandRunner(generator func(io.Writer) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return generator(os.Stdout)
	}
}
