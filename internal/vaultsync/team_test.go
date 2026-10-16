package vaultsync

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func TestTeamEnvelopeSyncAndRotation(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	defer ts.Close()
	ownerAccess, _ := login(t, ts, "team-owner")
	memberAccess, _ := login(t, ts, "team-member")
	owner := NewClient(ts.URL, ownerAccess)
	member := NewClient(ts.URL, memberAccess)
	ownerMe, err := owner.Me()
	if err != nil {
		t.Fatal(err)
	}
	memberMe, err := member.Me()
	if err != nil {
		t.Fatal(err)
	}
	org, err := owner.CreateOrg("Acme")
	if err != nil {
		t.Fatal(err)
	}
	token, err := owner.Invite(org.ID, "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := member.AcceptInvite(token); err != nil {
		t.Fatal(err)
	}
	vault, err := owner.CreateTeamVault(org.ID, "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	ownerKeys, err := vaultcrypto.NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	memberKeys, err := vaultcrypto.NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = owner.EnrollDevice(ownerKeys.SignPub, ownerKeys.Box.PublicKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	memberDev, memberStatus, err := member.EnrollDevice(memberKeys.SignPub, memberKeys.Box.PublicKey().Bytes())
	if err != nil || memberDev == "" || memberStatus != "active" {
		t.Fatalf("enroll %s %s %v", memberDev, memberStatus, err)
	}
	listed, err := owner.ListMembers(org.ID)
	if err != nil {
		t.Fatal(err)
	}
	var box []byte
	for _, person := range listed {
		if person.ID == memberMe.ID && len(person.Devices) == 1 && person.Devices[0].ID == memberDev {
			box = person.Devices[0].BoxPub
		}
	}
	if !bytes.Equal(box, memberKeys.Box.PublicKey().Bytes()) {
		t.Fatalf("device key %x", box)
	}
	if err := owner.Grant(vault.ID, memberMe.ID, "edit"); err != nil {
		t.Fatal(err)
	}
	vaultKey, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(vaultKey)
	env, err := vaultcrypto.SealForDevice(vaultKey, vault.ID, ownerMe.Instance, 1, memberKeys.Box.PublicKey(), ownerKeys.Sign)
	if err != nil {
		t.Fatal(err)
	}
	packed, err := vaultcrypto.MarshalDeviceEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PutEnvelope(vault.ID, memberDev, 1, packed, env.Signature); err != nil {
		t.Fatal(err)
	}
	remote, err := member.ListEnvelopes(vault.ID)
	if err != nil || len(remote) != 1 {
		t.Fatalf("envelopes %v %d", err, len(remote))
	}
	openedEnv, err := vaultcrypto.UnmarshalDeviceEnvelope(remote[0].Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	openedKey, err := vaultcrypto.OpenForDevice(openedEnv, memberMe.Instance, memberKeys.Box)
	if err != nil || !bytes.Equal(openedKey, vaultKey) {
		t.Fatalf("open key %v", err)
	}

	itemID := "item0000000000tt"
	sealed, err := vaultcrypto.SealItem(vaultKey, vaultcrypto.AAD{
		AccountID: ownerMe.ID, InstanceID: ownerMe.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 1,
	}, []byte(`{"title":"shared"}`))
	if err != nil {
		t.Fatal(err)
	}
	pushed, err := Reconcile(owner, Snapshot{
		VaultID: vault.ID,
		Items:   []LocalItem{{ID: itemID, Ciphertext: sealed, SealedRevision: 1, Dirty: true}},
	})
	if err != nil || pushed.Pushed[itemID] != 1 {
		t.Fatalf("push %v %#v", err, pushed.Pushed)
	}
	pulled, err := Reconcile(member, Snapshot{VaultID: vault.ID})
	if err != nil || len(pulled.Pulled) != 1 {
		t.Fatalf("pull %v %#v", err, pulled.Pulled)
	}
	plain, err := vaultcrypto.OpenItem(openedKey, vaultcrypto.AAD{
		AccountID: ownerMe.ID, InstanceID: ownerMe.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 1,
	}, pulled.Pulled[0].Ciphertext)
	if err != nil || string(plain) != `{"title":"shared"}` {
		t.Fatalf("shared item %v %s", err, plain)
	}

	if err := owner.Revoke(vault.ID, memberMe.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := member.GetItem(itemID); err == nil {
		t.Fatal("revoked member still read the item")
	}
	nextKey, err := vaultcrypto.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	defer vaultcrypto.Wipe(nextKey)
	rotated, err := vaultcrypto.SealItem(nextKey, vaultcrypto.AAD{
		AccountID: ownerMe.ID, InstanceID: ownerMe.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 2,
	}, []byte(`{"title":"after"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vaultcrypto.OpenItem(openedKey, vaultcrypto.AAD{
		AccountID: ownerMe.ID, InstanceID: ownerMe.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 2,
	}, rotated); err == nil {
		t.Fatal("old key opened the rotated item")
	}
	if plain, err := vaultcrypto.OpenItem(nextKey, vaultcrypto.AAD{
		AccountID: ownerMe.ID, InstanceID: ownerMe.Instance, VaultID: vault.ID, ItemID: itemID, Revision: 2,
	}, rotated); err != nil || string(plain) != `{"title":"after"}` {
		t.Fatalf("new key %v %s", err, plain)
	}
}
