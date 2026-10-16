package app

import (
	"encoding/json"
	"testing"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaultsync"
)

func TestRotateTeamReplacesKey(t *testing.T) {
	svc := New()
	if _, err := svc.BootstrapPassphrase("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BindIdentity("acct-1", "inst-1", "personal-vault", nil); err != nil {
		t.Fatal(err)
	}
	if _, enrolled, err := svc.EnsureDevice("acct-1", "inst-1"); err != nil || enrolled {
		t.Fatal(err, enrolled)
	}
	svc.SetDeviceID("dev-owner")
	svc.RememberTeam("team-1", "Engineering", "org-1", "acct-1", true)
	if _, err := svc.CreateTeamKey("team-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetActiveVault("team-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{Kind: "login", Title: "Shared", Password: "SENTINEL-PLAINTEXT"}); err != nil {
		t.Fatal(err)
	}
	found := svc.Search("shared")
	if len(found) != 1 {
		t.Fatal(found)
	}
	itemID := found[0].ID
	thief, err := vaultcrypto.NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	leaked, err := svc.SealTeamKey("team-1", "dev-thief", thief.Box.PublicKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	packed, err := vaultcrypto.UnmarshalDeviceEnvelope(leaked.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := vaultcrypto.OpenForDevice(packed, "inst-1", thief.Box)
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(oldKey)
	rotated, err := svc.RotateTeam("team-1", []TeamRecipient{{DeviceID: "dev-owner", BoxPub: svc.DeviceBox()}})
	if err != nil || len(rotated) != 1 || rotated[0].Generation != 2 {
		t.Fatalf("rotate %#v %v", rotated, err)
	}
	state := svc.ExportVault()
	var ciphertext []byte
	var revision int64
	for _, team := range state.Teams {
		if team.ID != "team-1" {
			continue
		}
		for id, env := range team.Envelopes {
			ciphertext = env
			revision = team.Items[id].Sealed
		}
	}
	if len(ciphertext) == 0 || revision == 0 {
		t.Fatal("missing rotated item")
	}
	if _, err := vaultcrypto.OpenItem(oldKey, vaultcrypto.AAD{
		AccountID: "acct-1", InstanceID: "inst-1", VaultID: "team-1", ItemID: itemID, Revision: uint64(revision),
	}, ciphertext); err == nil {
		t.Fatal("removed member key opened the rotated item")
	}
	nextEnv, err := vaultcrypto.UnmarshalDeviceEnvelope(rotated[0].Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vaultcrypto.OpenForDevice(nextEnv, "inst-1", thief.Box); err == nil {
		t.Fatal("removed member opened the new envelope")
	}
}

func TestApplyTeamKeepsSyncWhenOneItemWillNotOpen(t *testing.T) {
	svc := New()
	if _, err := svc.BootstrapPassphrase("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BindIdentity("acct-1", "inst-1", "personal-vault", nil); err != nil {
		t.Fatal(err)
	}
	if _, enrolled, err := svc.EnsureDevice("acct-1", "inst-1"); err != nil || enrolled {
		t.Fatal(err, enrolled)
	}
	svc.SetDeviceID("dev-owner")
	svc.RememberTeam("team-1", "Engineering", "org-1", "acct-1", true)
	if _, err := svc.CreateTeamKey("team-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetActiveVault("team-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutDecryptedItem(ItemBody{Kind: "login", Title: "Shared", Password: "SENTINEL-PLAINTEXT"}); err != nil {
		t.Fatal(err)
	}
	local := svc.Search("shared")
	if len(local) != 1 {
		t.Fatal(local)
	}
	localID := local[0].ID
	key := teamVaultKey(t, svc)
	defer vaultcrypto.Wipe(key)
	goodID := "gooditem00000001"
	goodCT := sealTeamBody(t, key, goodID, "inst-1", 1, "Remote")
	legacyID := "legacyitem000001"
	legacyCT := sealTeamBody(t, key, legacyID, "local-dev", 1, "Legacy")
	epoch, _, ok := svc.TeamSnapshot("team-1")
	if !ok {
		t.Fatal("missing team")
	}
	err := svc.ApplyTeam("team-1", epoch, vaultsync.Result{
		Cursor: 12,
		Pulled: []vaultsync.Item{
			{ID: goodID, Revision: 1, Ciphertext: goodCT},
			{ID: legacyID, Revision: 1, Ciphertext: legacyCT},
			{ID: "garbageitem0001", Revision: 1, Ciphertext: []byte("not-an-envelope")},
		},
		Pushed: map[string]int64{localID: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.SyncStatus() != "synced" {
		t.Fatal(svc.SyncStatus())
	}
	if _, _, ok := svc.TeamSnapshot("team-1"); !ok {
		t.Fatal("missing team")
	}
	state := teamByID(t, svc, "team-1")
	if state.Cursor != 12 || state.SyncStatus != "synced" {
		t.Fatalf("cursor %d status %s", state.Cursor, state.SyncStatus)
	}
	if _, ok := state.Envelopes["garbageitem0001"]; ok {
		t.Fatal("kept an item that does not open")
	}
	if _, ok := state.Items[goodID]; !ok || state.Items[goodID].Base != 1 || state.Items[goodID].Dirty {
		t.Fatalf("good item %#v", state.Items[goodID])
	}
	pushed := state.Items[localID]
	if pushed.Base != 1 || pushed.Dirty || pushed.Sealed != 1 {
		t.Fatalf("pushed item %#v", pushed)
	}
	legacy := state.Items[legacyID]
	if !legacy.Dirty || legacy.Base != 1 || legacy.Sealed != 2 {
		t.Fatalf("legacy item %#v", legacy)
	}
	if _, err := vaultcrypto.OpenItem(key, vaultcrypto.AAD{
		AccountID: "acct-1", InstanceID: "inst-1", VaultID: "team-1", ItemID: legacyID, Revision: 2,
	}, state.Envelopes[legacyID]); err != nil {
		t.Fatal(err)
	}
	if err := svc.InstallTeamKey("team-1", key, 1); err != nil {
		t.Fatal(err)
	}
	if kept := teamByID(t, svc, "team-1"); kept.Cursor != 12 {
		t.Fatalf("same key reset cursor to %d", kept.Cursor)
	}
	next, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(next)
	if err := svc.InstallTeamKey("team-1", next, 2); err != nil {
		t.Fatal(err)
	}
	if reset := teamByID(t, svc, "team-1"); reset.Cursor != 0 {
		t.Fatalf("new key left cursor at %d", reset.Cursor)
	}
}

func teamVaultKey(t *testing.T, svc *Service) []byte {
	t.Helper()
	reader, err := vaultcrypto.NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := svc.SealTeamKey("team-1", "dev-reader", reader.Box.PublicKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	env, err := vaultcrypto.UnmarshalDeviceEnvelope(sealed.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	key, err := vaultcrypto.OpenForDevice(env, "inst-1", reader.Box)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func sealTeamBody(t *testing.T, key []byte, id, instance string, rev int64, title string) []byte {
	t.Helper()
	raw, err := json.Marshal(ItemBody{ID: id, Kind: "login", Title: title})
	if err != nil {
		t.Fatal(err)
	}
	env, err := vaultcrypto.SealItem(key, vaultcrypto.AAD{
		AccountID: "acct-1", InstanceID: instance, VaultID: "team-1", ItemID: id, Revision: uint64(rev),
	}, raw)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func teamByID(t *testing.T, svc *Service, id string) TeamState {
	t.Helper()
	for _, team := range svc.ExportVault().Teams {
		if team.ID == id {
			return team
		}
	}
	t.Fatal("missing team")
	return TeamState{}
}
