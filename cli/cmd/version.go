package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set via -ldflags at build time; defaults to "dev".
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the quillit version and installed binary path.",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := os.Executable()
		if err != nil {
			path = "(unknown)"
		}
		fmt.Printf("quillit %s\n", Version)
		fmt.Printf("installed at %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
