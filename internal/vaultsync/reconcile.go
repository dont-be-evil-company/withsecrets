package vaultsync

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
)

const pageSize = 100

// LocalItem is one item the desktop already has.
type LocalItem struct {
	ID             string
	Ciphertext     []byte
	SealedRevision int64
	BaseRevision   int64
	Dirty          bool
	PendingDelete  bool
	ConflictAt     int64
}

// Snapshot is the local vault at the start of a sync.
type Snapshot struct {
	VaultID string
	Cursor  int64
	Wrap    []byte
	Items   []LocalItem
}

// Result is what the desktop should apply after a successful sync.
type Result struct {
	VaultID   string
	Cursor    int64
	Pulled    []Item
	Removed   []string
	Pushed    map[string]int64
	Conflicts []Item
}

type row struct {
	LocalItem
	skip bool
	seen bool
}

// Reconcile pulls changes, pushes dirty local items, and reports conflicts.
// It does not decrypt. The caller seals ciphertext before this runs.
func Reconcile(c *Client, snap Snapshot) (Result, error) {
	var vault Vault
	var err error
	if snap.VaultID != "" {
		vault, err = c.vaultByID(snap.VaultID)
	} else {
		vault, err = c.EnsurePersonal("")
	}
	if err != nil {
		return Result{}, err
	}
	out := Result{VaultID: vault.ID, Cursor: snap.Cursor, Pushed: map[string]int64{}}
	if len(snap.Wrap) > 0 && !bytes.Equal(snap.Wrap, vault.AccountKeyWrap) {
		if err := c.PutAccountWrap(vault.ID, snap.Wrap); err != nil {
			return Result{}, err
		}
	}
	rows := map[string]*row{}
	for _, item := range snap.Items {
		cp := item
		cp.Ciphertext = append([]byte(nil), item.Ciphertext...)
		rows[item.ID] = &row{LocalItem: cp}
	}
	cursor := snap.Cursor
	var drop []Item
	for {
		changes, err := c.Changes(cursor)
		if err != nil {
			return Result{}, err
		}
		if len(changes) == 0 {
			break
		}
		for _, ch := range changes {
			if ch.Seq > cursor {
				cursor = ch.Seq
			}
			if ch.Kind != "item" || ch.VaultID != vault.ID {
				continue
			}
			item, err := c.GetItem(ch.ObjectID)
			if err != nil {
				var status *StatusError
				if errors.As(err, &status) && status.Status == http.StatusNotFound {
					item = Item{ID: ch.ObjectID, VaultID: vault.ID, Revision: ch.Revision, Deleted: true}
				} else {
					return Result{}, err
				}
			}
			mergeItem(&out, rows, item, &drop)
		}
		if len(changes) < pageSize {
			break
		}
	}
	for _, item := range drop {
		if err := c.DeleteItem(item.ID, item.Revision); err != nil {
			var status *StatusError
			if errors.As(err, &status) && status.Status == http.StatusNotFound {
				continue
			}
			return Result{}, err
		}
	}
	for id, item := range rows {
		if item.skip {
			continue
		}
		if item.PendingDelete {
			if item.BaseRevision == 0 {
				out.Removed = append(out.Removed, id)
				continue
			}
			if err := c.DeleteItem(id, item.BaseRevision); err != nil {
				var status *StatusError
				if errors.As(err, &status) && status.Status == http.StatusConflict {
					out.Conflicts = append(out.Conflicts, Item{ID: id, Revision: item.BaseRevision + 1})
					continue
				}
				return Result{}, err
			}
			out.Removed = append(out.Removed, id)
			continue
		}
		if !item.Dirty {
			continue
		}
		pushed, conflict, err := pushItem(c, vault.ID, item.LocalItem)
		if err != nil {
			return Result{}, err
		}
		if conflict != nil {
			out.Conflicts = append(out.Conflicts, *conflict)
			continue
		}
		out.Pushed[id] = pushed
	}
	out.Cursor = cursor
	return out, nil
}

func mergeItem(out *Result, rows map[string]*row, item Item, drop *[]Item) {
	current := rows[item.ID]
	if current != nil && current.seen {
		return
	}
	if current == nil {
		if item.Deleted {
			return
		}
		// The ciphertext is sealed to its original item id. A server that
		// minted a new id stored a copy that cannot be opened under that id.
		if match := localByCiphertext(rows, item.Ciphertext); match != nil && match.SealedRevision <= 1 {
			appendDrop(drop, item)
			match.BaseRevision = 0
			match.Dirty = true
			match.PendingDelete = false
			match.skip = false
			return
		}
		out.Pulled = append(out.Pulled, item)
		rows[item.ID] = &row{LocalItem: LocalItem{
			ID: item.ID, Ciphertext: append([]byte(nil), item.Ciphertext...),
			BaseRevision: item.Revision, SealedRevision: item.Revision,
		}, skip: true, seen: true}
		return
	}
	current.seen = true
	if item.Deleted {
		if current.Dirty || current.PendingDelete {
			out.Conflicts = append(out.Conflicts, Item{ID: item.ID, Revision: item.Revision, Deleted: true})
			current.skip = true
			return
		}
		out.Removed = append(out.Removed, item.ID)
		delete(rows, item.ID)
		return
	}
	if bytes.Equal(current.Ciphertext, item.Ciphertext) {
		current.Dirty = false
		current.PendingDelete = false
		current.BaseRevision = item.Revision
		current.SealedRevision = item.Revision
		current.skip = true
		out.Pushed[item.ID] = item.Revision
		return
	}
	if !current.Dirty && !current.PendingDelete {
		out.Pulled = append(out.Pulled, item)
		current.Ciphertext = append([]byte(nil), item.Ciphertext...)
		current.BaseRevision = item.Revision
		current.SealedRevision = item.Revision
		current.skip = true
		return
	}
	if current.ConflictAt > 0 && current.ConflictAt >= item.Revision {
		current.skip = true
		return
	}
	_, conflict := Decide(item.ID, current.BaseRevision, current.SealedRevision, item.Revision, current.Ciphertext, item.Ciphertext)
	if conflict != nil {
		out.Conflicts = append(out.Conflicts, item)
		current.skip = true
		return
	}
}

func appendDrop(drop *[]Item, item Item) {
	for i, existing := range *drop {
		if existing.ID == item.ID {
			(*drop)[i] = item
			return
		}
	}
	*drop = append(*drop, item)
}

func localByCiphertext(rows map[string]*row, ciphertext []byte) *row {
	for _, item := range rows {
		if bytes.Equal(item.Ciphertext, ciphertext) {
			return item
		}
	}
	return nil
}

func pushItem(c *Client, vaultID string, item LocalItem) (int64, *Item, error) {
	if item.BaseRevision == 0 {
		created, err := c.CreateItem(vaultID, item.ID, item.Ciphertext)
		if err == nil {
			if created.ID != item.ID {
				_ = c.DeleteItem(created.ID, created.Revision)
				return 0, nil, fmt.Errorf("vaultsync: server stored item %s as %s", item.ID, created.ID)
			}
			return created.Revision, nil, nil
		}
		var status *StatusError
		if errors.As(err, &status) && status.Status == http.StatusConflict {
			return adoptOrConflict(c, item)
		}
		return 0, nil, err
	}
	updated, err := c.UpdateItem(item.ID, item.Ciphertext, item.BaseRevision)
	if err == nil {
		return updated.Revision, nil, nil
	}
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusConflict {
		return adoptOrConflict(c, item)
	}
	return 0, nil, err
}

func adoptOrConflict(c *Client, item LocalItem) (int64, *Item, error) {
	current, err := c.GetItem(item.ID)
	if err != nil {
		return 0, nil, err
	}
	if bytes.Equal(current.Ciphertext, item.Ciphertext) {
		return current.Revision, nil, nil
	}
	return 0, &current, nil
}
