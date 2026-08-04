// Package cmd wires the quillit command tree (github.com/spf13/cobra).
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "quillit",
	Short: "Annotated markdown notes for running your D&D game.",
	Long: `Quillit collapses a DM's full notes, player-safe notes, and flash cards
into one annotated markdown file per entry. render/export commands derive
any of the three views (dm, player, card <facet>) from that single source
of truth.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command; called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
