package ws

import (
	"context"
	"fmt"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/gui"
	"github.com/spf13/cobra"
)

var (
	guiConfigFile string
	guiNoBrowser  bool
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Web GUI for managing environments and secrets",
	Long: `Launch a local web GUI for creating, reading, updating, and deleting secrets.

The server listens on 127.0.0.1 only (default port 11911) and opens your default browser.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if guiConfigFile == "" {
			var err error
			guiConfigFile, err = config.FindConfigFile()
			if err != nil {
				return fmt.Errorf("failed to find configuration file: %w", err)
			}
		}
		return gui.Run(context.Background(), gui.Options{
			ConfigPath: guiConfigFile,
			NoBrowser:  guiNoBrowser,
		})
	},
}

func init() {
	guiCmd.Flags().StringVarP(&guiConfigFile, "config", "c", "", "Path to ws.yaml configuration file")
	guiCmd.Flags().BoolVar(&guiNoBrowser, "no-browser", false, "Do not open the default browser")
	rootCmd.AddCommand(guiCmd)
}
