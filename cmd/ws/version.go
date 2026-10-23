package ws

import (
	"fmt"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "withsecrets Version",
	Long:  "Displays the current version of withsecrets CLI",
	Run: func(cmd *cobra.Command, files []string) {
		fmt.Println(version.VERSION)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.PersistentFlags().BoolVar(&cfg.Flags.Version, "version", false, "withsecrets version")
}
