package ws

import "github.com/spf13/cobra"

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create withsecrets resources",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}
