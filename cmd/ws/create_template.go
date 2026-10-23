package ws

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/templates"
	"github.com/spf13/cobra"
)

var execCommand = exec.Command

var createTemplateCmd = &cobra.Command{
	Use:   "template <template_name>",
	Short: "Create or edit a user template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if _, err := templates.EnsureTemplatesDir(); err != nil {
			return err
		}

		path, exists, err := templates.ExistingTemplatePath(name)
		if err != nil {
			return err
		}

		if !exists {
			body, err := templates.DefaultTemplate()
			if err != nil {
				return fmt.Errorf("failed to load embedded default template: %w", err)
			}
			if err := os.WriteFile(path, body, 0644); err != nil {
				return fmt.Errorf("failed to create template '%s': %w", name, err)
			}
			fmt.Printf("Created template: %s\n", path)
		}

		return openInEditor(path)
	},
}

func openInEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return fmt.Errorf("no default editor is set. please set VISUAL or EDITOR to create or edit templates")
	}

	cmd := execCommand(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor '%s': %w", editor, err)
	}
	return nil
}

func init() {
	createCmd.AddCommand(createTemplateCmd)
}
