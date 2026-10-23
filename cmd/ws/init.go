package ws

import (
	"fmt"
	"os"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/fileutils"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/dont-be-evil-company/withsecrets/internal/templates"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [template]",
	Short: "Create a default configuration file",
	Long:  "This command initializes a ws.yaml configuration file, optionally using a named template.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := log.NewLogger()
		logger.Debug("Initializing ws configuration")

		target := config.DefaultConfigFileName
		if fileutils.FileExists(target) {
			logger.Debug("Configuration file already exists, no action taken")
			return nil
		}

		if _, err := templates.EnsureTemplatesDir(); err != nil {
			return err
		}

		requestedTemplate := ""
		if len(args) == 1 {
			requestedTemplate = args[0]
		}
		body, source, err := templates.ResolveInitTemplate(requestedTemplate)
		if err != nil {
			available, listErr := templates.ListTemplateNames()
			if listErr != nil {
				return fmt.Errorf("%w (failed to list templates: %v)", err, listErr)
			}
			return fmt.Errorf("%w\navailable templates: %s", err, stringsOrNone(available))
		}

		if err := os.WriteFile(target, body, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
		logger.Debug("Configuration file created successfully", "source", source)
		return nil
	},
}

func stringsOrNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

func init() {
	rootCmd.AddCommand(initCmd)
}
