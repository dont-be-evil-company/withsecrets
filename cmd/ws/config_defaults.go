package ws

import (
	"fmt"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configDefaultsCmd = &cobra.Command{
	Use:   "defaults",
	Short: "Get/set default values used by withsecrets",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var configDefaultsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get configured defaults",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, _ := cmd.Flags().GetString("provider")
		gc, err := config.LoadGlobalConfig()
		if err != nil {
			return fmt.Errorf("failed to load global config: %w", err)
		}

		out := map[string]any{}
		if provider == "" {
			if gc.Defaults != nil {
				out["defaults"] = gc.Defaults
			} else {
				out["defaults"] = &config.DefaultsConfig{}
			}
		} else {
			var pd config.ProviderDefaults
			if gc.Defaults != nil && gc.Defaults.Providers != nil {
				pd = gc.Defaults.Providers[provider]
			}
			out["provider"] = provider
			out["regions"] = pd.Regions
		}

		b, err := yaml.Marshal(out)
		if err != nil {
			return err
		}
		fmt.Print(string(b))
		return nil
	},
}

var configDefaultsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set provider defaults (regions)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, _ := cmd.Flags().GetString("provider")
		regionsStr, _ := cmd.Flags().GetString("regions")
		clear, _ := cmd.Flags().GetBool("clear")
		if strings.TrimSpace(provider) == "" {
			return fmt.Errorf("--provider is required")
		}

		gc, err := config.LoadGlobalConfig()
		if err != nil {
			return fmt.Errorf("failed to load global config: %w", err)
		}
		if gc.Defaults == nil {
			gc.Defaults = &config.DefaultsConfig{}
		}
		if gc.Defaults.Providers == nil {
			gc.Defaults.Providers = map[string]config.ProviderDefaults{}
		}

		if clear {
			delete(gc.Defaults.Providers, provider)
		} else {
			regions := []string{}
			for _, r := range strings.Split(regionsStr, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					regions = append(regions, r)
				}
			}
			gc.Defaults.Providers[provider] = config.ProviderDefaults{Regions: regions}
		}

		if err := config.SaveGlobalConfig(gc); err != nil {
			return fmt.Errorf("failed to save global config: %w", err)
		}

		fmt.Println("Defaults updated.")
		return nil
	},
}

func init() {
	configCmd.AddCommand(configDefaultsCmd)
	configDefaultsCmd.AddCommand(configDefaultsGetCmd)
	configDefaultsCmd.AddCommand(configDefaultsSetCmd)

	configDefaultsGetCmd.Flags().String("provider", "", "Provider name (e.g. gcp)")

	configDefaultsSetCmd.Flags().String("provider", "", "Provider name (e.g. gcp)")
	configDefaultsSetCmd.Flags().String("regions", "", "Comma-separated regions/locations (e.g. us-central1,europe-west1)")
	configDefaultsSetCmd.Flags().Bool("clear", false, "Clear defaults for the provider")
}
