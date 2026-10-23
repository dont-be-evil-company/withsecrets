package guiapi

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/secrets"
)

func uniqueSorted(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	sort.Strings(ss)
	out := ss[:0]
	var prev string
	for i, s := range ss {
		if i == 0 || s != prev {
			out = append(out, s)
			prev = s
		}
	}
	return out
}

func applyGCPDefaults(globalCfg *config.GlobalConfig, provider string, allLocations []string) (replication string, locations, selected []string) {
	replication = "global"
	locations = append([]string(nil), allLocations...)
	if globalCfg == nil || globalCfg.Defaults == nil || globalCfg.Defaults.Providers == nil {
		return replication, locations, nil
	}
	pd, ok := globalCfg.Defaults.Providers[provider]
	if !ok || len(pd.Regions) == 0 || provider != "gcp" {
		return replication, locations, nil
	}

	supported := map[string]bool{}
	for _, l := range allLocations {
		supported[l] = true
	}

	matchers := make([]func(string) bool, 0, len(pd.Regions))
	for _, raw := range pd.Regions {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		looksRegex := strings.ContainsAny(p, `|.*+?()[]{}^$\`)
		if looksRegex {
			if re, err := regexp.Compile(p); err == nil {
				matchers = append(matchers, re.MatchString)
				continue
			}
		}
		matchers = append(matchers, func(s string) bool { return s == p })
	}

	filtered := []string{}
	if len(allLocations) > 0 {
		for _, loc := range allLocations {
			for _, matches := range matchers {
				if matches(loc) {
					filtered = append(filtered, loc)
					break
				}
			}
		}
	} else {
		for _, r := range pd.Regions {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if len(supported) == 0 || supported[r] {
				filtered = append(filtered, r)
			}
		}
	}

	if len(filtered) > 0 {
		filtered = uniqueSorted(filtered)
		return "user-managed", filtered, append([]string(nil), filtered...)
	}
	if len(allLocations) > 0 {
		return replication, append([]string(nil), allLocations...), nil
	}
	return replication, locations, nil
}

// GetCreateOptions returns pre-filled create form options for an environment.
func GetCreateOptions(ctx context.Context, configPath, envName string) (*CreateOptions, error) {
	cfg, err := config.LoadSecretsConfig(configPath)
	if err != nil {
		return nil, err
	}
	env, err := cfg.GetEnvironment(envName)
	if err != nil {
		return nil, err
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}

	opts := &CreateOptions{
		Provider:    env.Provider,
		Replication: "global",
	}

	if env.Provider != "gcp" {
		return opts, nil
	}

	allLocs, err := GCPLocations(ctx, env.Project)
	if err != nil {
		return nil, err
	}
	opts.SupportsReplication = true
	opts.AllLocations = allLocs
	replication, locations, selected := applyGCPDefaults(globalCfg, env.Provider, allLocs)
	opts.Replication = replication
	opts.Locations = locations
	opts.SelectedLocations = selected
	return opts, nil
}

// GCPLocations returns supported GCP Secret Manager locations for a project.
func GCPLocations(ctx context.Context, project string) ([]string, error) {
	factory := secrets.NewSecretManagerFactory()
	sm, err := factory.CreateSecretManager(ctx, "gcp", project, "")
	if err != nil {
		return nil, err
	}
	defer sm.Close()

	gcpSM, ok := sm.(*secrets.GCPSecretManager)
	if !ok {
		return nil, errUnexpectedGCPManager
	}
	return gcpSM.SupportedLocations(project)
}
