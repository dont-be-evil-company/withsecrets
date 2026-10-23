package ws

import (
	"fmt"

	"github.com/charmbracelet/glamour"
	"github.com/dont-be-evil-company/withsecrets/internal/changelog"
	"github.com/spf13/cobra"
)

var changelogCmd = &cobra.Command{
	Use:   "changelog [latest|version]",
	Short: "Show the baked-in changelog",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content := changelog.Markdown
		if len(args) == 1 {
			section, err := changelog.Select(changelog.Markdown, args[0])
			if err != nil {
				return err
			}
			content = section
		}

		out, err := glamour.RenderWithEnvironmentConfig(content)
		if err != nil {
			return fmt.Errorf("failed to render changelog: %w", err)
		}
		fmt.Print(out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(changelogCmd)
}
