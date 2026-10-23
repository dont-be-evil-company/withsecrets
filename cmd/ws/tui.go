package ws

import (
	"context"
	"fmt"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/tui"
	"github.com/spf13/cobra"
)

var (
	tuiConfigFile string
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for environments and secrets",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if tuiConfigFile == "" {
			var err error
			tuiConfigFile, err = config.FindConfigFile()
			if err != nil {
				return fmt.Errorf("failed to find configuration file: %w", err)
			}
		}
		return tui.Run(context.Background(), tuiConfigFile)
	},
}

func init() {
	tuiCmd.Flags().StringVarP(&tuiConfigFile, "config", "c", "", "Path to ws.yaml configuration file")
	rootCmd.AddCommand(tuiCmd)
}
