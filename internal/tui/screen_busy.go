package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

func (m *Model) updateBusy(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case createDoneMsg:
		m.busyText = ""
		m.screen = screenSecrets
		if msg.err != nil {
			// Re-open create form with current values intact.
			m.createForm = m.newCreateForm()
			m.screen = screenCreate
			return m.openError(screenCreate, "Create failed", msg.err.Error())
		}
		// Reload config + secrets
		if cfg, err := config.LoadSecretsConfig(m.configPath); err == nil {
			m.cfg = cfg
			if env, err := m.cfg.GetEnvironment(m.selectedEnvName); err == nil {
				m.selectedEnv = env
			}
		}
		_ = m.reloadSecrets()
		return m, nil
	case editDoneMsg:
		m.busyText = ""
		m.screen = screenSecrets
		if msg.err != nil {
			// Return to edit
			m.editForm = m.newEditForm()
			if m.editIsPath {
				m.editForm = m.newPathEditForm()
			}
			m.screen = screenEdit
			return m.openError(screenEdit, "Save failed", msg.err.Error())
		}
		_ = m.reloadSecrets()
		return m, nil
	case deleteDoneMsg:
		m.busyText = ""
		m.screen = screenSecrets
		if msg.err != nil {
			return m.openError(screenSecrets, "Delete failed", msg.err.Error())
		}
		_ = m.reloadSecrets()
		return m, nil
	case tea.KeyMsg:
		// Prevent accidental interaction while busy.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}
