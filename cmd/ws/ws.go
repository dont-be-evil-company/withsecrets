package ws

import (
	"fmt"
	"os"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/version"
	"github.com/spf13/cobra"
)

var cfg = config.NewConfig(config.Config{
	Flags: config.ConfigFlags{},
})

var rootCmd = &cobra.Command{
	Use:   cliNameWS,
	Short: "withsecrets CLI",
	Long:  "ws is the CLI for withsecrets - access secrets and environment variables from GCP, AWS, Azure, Bitwarden, OpenBao, and more.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		log.SetDebugMode(cfg.Flags.Debug)
	},
	Run: func(cmd *cobra.Command, files []string) {
		if cfg.Flags.Version {
			fmt.Println(version.VERSION)
			return
		}
	},
}

func Execute() {
	rootCmd.Use = CLIName()
	err := rootCmd.Execute()
	if err != nil {
		osExit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&cfg.Flags.Debug, "debug", "d", false, "Enable debug mode for verbose logging")
}

// osExit is a variable to allow overriding in tests
var osExit = os.Exit
