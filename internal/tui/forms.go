package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

func (m *Model) newEditForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Secret value").
				Value(&m.editValue),
			huh.NewConfirm().
				Title("Save changes?").
				Affirmative("Save").
				Negative("Cancel").
				Value(&m.editSave),
		),
	).WithTheme(huh.ThemeFunc(themeVHSEra))
}

func (m *Model) newPathEditForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Paths (one per line)").
				Value(&m.editPaths).
				Validate(func(s string) error {
					if len(config.ParsePathLines(s)) == 0 {
						return fmt.Errorf("at least one path is required")
					}
					return nil
				}),
			huh.NewConfirm().
				Title("Save changes?").
				Affirmative("Save").
				Negative("Cancel").
				Value(&m.editSave),
		),
	).WithTheme(huh.ThemeFunc(themeVHSEra))
}

func (m *Model) newDeleteForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Delete secret").
				Description(mdEscape(m.confirmText)),
			huh.NewConfirm().
				Title("Proceed?").
				Affirmative("Delete").
				Negative("Cancel").
				Value(&m.deleteYes),
		),
	).WithTheme(huh.ThemeFunc(themeVHSEra))
}

func (m *Model) newErrorForm(title, text string) *huh.Form {
	if strings.TrimSpace(title) == "" {
		title = "Error"
	}
	if strings.TrimSpace(text) == "" {
		text = "Unknown error"
	}
	back := "back"
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(title).
				Description(text),
			huh.NewSelect[string]().
				Title("").
				Options(huh.NewOption("Back", "back")).
				Value(&back),
		),
	).WithTheme(huh.ThemeFunc(themeVHSEra))
}

func (m *Model) newCreateForm() *huh.Form {
	if m.createKind == "" {
		m.createKind = "secret-key"
	}

	kindOpts := []huh.Option[string]{
		huh.NewOption("secret-key", "secret-key"),
		huh.NewOption("secret-path", "secret-path"),
	}
	if m.selectedEnv != nil && m.selectedEnv.Provider == "aws" {
		kindOpts = append(kindOpts, huh.NewOption("param-path", "param-path"))
	}

	mainFields := []huh.Field{
		huh.NewSelect[string]().
			Title("Kind").
			Options(kindOpts...).
			Value(&m.createKind),
		huh.NewInput().
			Title("Env var").
			Placeholder("ENV_VAR_NAME or *").
			Value(&m.createEnvVar).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("env var is required")
				}
				if strings.TrimSpace(s) == "*" && m.createKind == "secret-key" {
					return fmt.Errorf(`"*" is only valid with secret-path or param-path`)
				}
				return nil
			}),
		huh.NewInput().
			TitleFunc(func() string {
				if isPathKind(m.createKind) {
					return "Secret key/id (secret-key only)"
				}
				return "Secret key/id"
			}, &m.createKind).
			Placeholder("provider secret key/id/path").
			Value(&m.createSecretKey).
			Validate(func(s string) error {
				if isPathKind(m.createKind) {
					return nil
				}
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("secret key/id is required")
				}
				return nil
			}),
		huh.NewText().
			TitleFunc(func() string {
				if isPathKind(m.createKind) {
					return "Paths (one per line)"
				}
				return "Paths (secret-path / param-path only)"
			}, &m.createKind).
			Value(&m.createPaths).
			Validate(func(s string) error {
				if !isPathKind(m.createKind) {
					return nil
				}
				if len(config.ParsePathLines(s)) == 0 {
					return fmt.Errorf("at least one path is required")
				}
				return nil
			}),
		huh.NewText().
			TitleFunc(func() string {
				if isPathKind(m.createKind) {
					return "Value (secret-key only)"
				}
				return "Value"
			}, &m.createKind).
			Value(&m.createValue),
		huh.NewInput().
			Title("Description (optional)").
			Value(&m.createDesc),
	}

	// Keep GCP region selection on the *same page* so it's hard to miss.
	if m.selectedEnv != nil && m.selectedEnv.Provider == "gcp" {
		mainFields = append(mainFields,
			huh.NewSelect[string]().
				Title("Replication").
				Options(
					huh.NewOption("Global (automatic replication)", "global"),
					huh.NewOption("User-managed (choose locations)", "user-managed"),
				).
				Value(&m.createReplication),
			huh.NewMultiSelect[string]().
				TitleFunc(func() string {
					if m.createReplication == "global" {
						return "Locations (set replication to user-managed to choose)"
					}
					return "Locations"
				}, &m.createReplication).
				OptionsFunc(func() []huh.Option[string] {
					if m.createReplication == "global" {
						return nil
					}
					opts := make([]huh.Option[string], 0, len(m.gcpLocations))
					for _, l := range m.gcpLocations {
						opts = append(opts, huh.NewOption(l, l))
					}
					return opts
				}, &m.createReplication).
				Value(&m.createLocations).
				Validate(func(v []string) error {
					if isPathKind(m.createKind) {
						return nil
					}
					if m.createReplication == "user-managed" && len(v) == 0 {
						return fmt.Errorf("select at least one location")
					}
					return nil
				}),
		)
	}

	summary := func() string {
		if m.selectedEnv == nil {
			return ""
		}

		envVar := strings.TrimSpace(m.createEnvVar)
		secretKey := strings.TrimSpace(m.createSecretKey)
		desc := strings.TrimSpace(m.createDesc)
		kind := m.createKind
		if kind == "" {
			kind = "secret-key"
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Environment: %s\n", mdEscape(m.selectedEnvName)))
		b.WriteString(fmt.Sprintf("Provider: %s\n", mdEscape(m.selectedEnv.Provider)))
		b.WriteString(fmt.Sprintf("Project: %s\n", mdEscape(m.selectedEnv.Project)))
		b.WriteString(fmt.Sprintf("Kind: %s\n", mdEscape(kind)))
		if envVar != "" {
			b.WriteString(fmt.Sprintf("Env var: %s\n", mdEscape(envVar)))
		}
		if isPathKind(kind) {
			for _, p := range config.ParsePathLines(m.createPaths) {
				b.WriteString("  - " + mdEscape(p) + "\n")
			}
		} else if secretKey != "" {
			b.WriteString(fmt.Sprintf("Secret key/id: %s\n", mdEscape(secretKey)))
		}
		if desc != "" && !isPathKind(kind) {
			b.WriteString(fmt.Sprintf("Description: %s\n", mdEscape(desc)))
		}

		if m.selectedEnv.Provider == "gcp" && !isPathKind(kind) {
			b.WriteString("Replication: ")
			if m.createReplication == "user-managed" {
				b.WriteString("User-managed\n")
				if len(m.createLocations) > 0 {
					b.WriteString("Locations:\n")
					// Show stable order.
					locs := append([]string(nil), m.createLocations...)
					sort.Strings(locs)
					for _, l := range locs {
						b.WriteString("  - " + mdEscape(l) + "\n")
					}
				} else {
					b.WriteString("Locations: (none)\n")
				}
			} else {
				b.WriteString("Global\n")
			}
		}

		return strings.TrimSpace(b.String())
	}

	mainFields = append(mainFields,
		huh.NewNote().
			Title("Summary").
			DescriptionFunc(summary, &m.createSummaryTick),
		huh.NewSelect[string]().
			Title("Action").
			Options(
				huh.NewOption("Create mapping", "create"),
				huh.NewOption("Cancel", "cancel"),
			).
			Value(&m.createAction),
	)

	return huh.NewForm(huh.NewGroup(mainFields...)).WithTheme(huh.ThemeFunc(themeVHSEra))
}
