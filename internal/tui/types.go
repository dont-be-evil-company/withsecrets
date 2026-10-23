package tui

import (
	"charm.land/bubbles/v2/list"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
)

func isPathKind(kind string) bool {
	return kind == "secret-path" || kind == "param-path"
}

func canMutateRow(r secretRow) bool {
	if r.refKind == "value" {
		return false
	}
	if r.refKind == "secret-key" || r.refKind == "param-key" {
		return r.isMapping
	}
	return isPathKind(r.refKind)
}

type Screen int

const (
	screenEnvs Screen = iota
	screenSecrets
	screenView
	screenEdit
	screenCreate
	screenConfirmDelete
	screenError
	screenBusy
)

type envItem struct{ name string }

func (e envItem) Title() string       { return e.name }
func (e envItem) Description() string { return "" }
func (e envItem) FilterValue() string { return e.name }

type secretRow struct {
	envVar      string
	value       string
	item        config.EnvItem
	provider    string
	project     string
	refKind     string // secret-key | secret-path | param-key | param-path | value
	ref         string // secret-key, secret-path, param-key, or param-path
	paths       []string
	providerKey string
	isMapping   bool
}

// Messages emitted by async operations.
type createDoneMsg struct{ err error }
type editDoneMsg struct{ err error }
type deleteDoneMsg struct{ err error }

// Compile-time assertion for list.Item.
var _ list.Item = envItem{}
