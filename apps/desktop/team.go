package main

import (
	"net/http"

	"github.com/mistweaverco/withsecrets/apps/desktop/app"
	"github.com/mistweaverco/withsecrets/internal/vaultsync"
)

func (h *host) syncTeams(client *vaultsync.Client, me vaultsync.Account) {
	if err := h.ensureDevice(client, me); err != nil {
		return
	}
	vaults, err := client.ListVaults()
	if err != nil {
		return
	}
	keep := map[string]bool{}
	for _, vault := range vaults {
		if vault.OrgID == "" {
			continue
		}
		keep[vault.ID] = true
		h.svc.RememberTeam(vault.ID, vault.NameHint, vault.OrgID, vault.OwnerID, vault.OwnerID == me.ID)
		if err := h.openTeamKey(client, vault); err != nil {
			h.svc.SetTeamStatus(vault.ID, "error")
			continue
		}
		if !h.svc.TeamHasKey(vault.ID) {
			h.svc.MarkTeamWaiting(vault.ID)
			continue
		}
		h.reconcileTeam(client, vault.ID)
	}
	h.svc.PruneTeams(keep)
}

func (h *host) ensureDevice(client *vaultsync.Client, me vaultsync.Account) error {
	pub, enrolled, err := h.svc.EnsureDevice(me.ID, me.Instance)
	if err != nil || enrolled {
		return err
	}
	id, status, err := client.EnrollDevice(pub.Sign, pub.Box)
	if err != nil {
		return err
	}
	if status == "pending" {
		if err := client.ApproveDevice(id); err != nil {
			return err
		}
	}
	h.svc.SetDeviceID(id)
	return nil
}

func (h *host) openTeamKey(client *vaultsync.Client, vault vaultsync.Vault) error {
	envs, err := client.ListEnvelopes(vault.ID)
	if err != nil {
		return err
	}
	deviceID := h.svc.DeviceID()
	var best vaultsync.RemoteEnvelope
	for _, env := range envs {
		if env.RecipientDeviceID != deviceID {
			continue
		}
		if env.Generation >= best.Generation {
			best = env
		}
	}
	if len(best.Ciphertext) > 0 && best.Generation >= h.svc.TeamGeneration(vault.ID) {
		key, generation, err := h.svc.OpenTeamEnvelope(vault.ID, best.Ciphertext)
		if err != nil {
			return err
		}
		if generation > h.svc.TeamGeneration(vault.ID) || !h.svc.TeamHasKey(vault.ID) {
			if err := h.svc.InstallTeamKey(vault.ID, key, generation); err != nil {
				return err
			}
		}
		return nil
	}
	if h.svc.TeamHasKey(vault.ID) {
		sealed, err := h.svc.CreateTeamKey(vault.ID)
		if err != nil {
			return nil
		}
		return client.PutEnvelope(vault.ID, sealed.DeviceID, sealed.Generation, sealed.Ciphertext, sealed.Signature)
	}
	sealed, err := h.svc.CreateTeamKey(vault.ID)
	if err != nil {
		return nil
	}
	return client.PutEnvelope(vault.ID, sealed.DeviceID, sealed.Generation, sealed.Ciphertext, sealed.Signature)
}

func (h *host) reconcileTeam(client *vaultsync.Client, id string) {
	for attempt := 0; attempt < 2; attempt++ {
		epoch, snap, ok := h.svc.TeamSnapshot(id)
		if !ok {
			return
		}
		result, err := vaultsync.Reconcile(client, snap)
		if err != nil {
			h.svc.SetTeamStatus(id, "error")
			return
		}
		err = h.svc.ApplyTeam(id, epoch, result)
		if err == nil {
			return
		}
		if err != app.ErrEpoch {
			h.svc.SetTeamStatus(id, "error")
			return
		}
	}
	h.svc.SetTeamStatus(id, "error")
}

func (h *host) bindPersonal(client *vaultsync.Client, me vaultsync.Account) error {
	vault, err := client.EnsurePersonal(h.svc.ServerVaultID())
	if err != nil {
		return err
	}
	upload, err := h.svc.BindIdentity(me.ID, me.Instance, vault.ID, vault.AccountKeyWrap)
	if err != nil {
		return err
	}
	if upload {
		return client.PutAccountWrap(vault.ID, h.svc.AccountWrap())
	}
	return nil
}

func (h *host) teamClient() (*vaultsync.Client, vaultsync.Account, error) {
	server, access, ok := h.sessionToken()
	if !ok || !h.svc.Unlocked() {
		return nil, vaultsync.Account{}, app.ErrEpoch
	}
	client := vaultsync.NewClient(server, access)
	me, err := client.Me()
	if err != nil {
		return nil, vaultsync.Account{}, err
	}
	return client, me, nil
}

func (h *host) createTeam(w http.ResponseWriter, r *http.Request) {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	var body struct {
		Org   string `json:"org"`
		Vault string `json:"vault"`
	}
	if err := read(r, &body); err != nil || body.Org == "" || body.Vault == "" {
		http.Error(w, "Enter an organization name and a vault name.", http.StatusBadRequest)
		return
	}
	client, me, err := h.teamClient()
	if err != nil {
		http.Error(w, "Sign in and unlock the vault before creating a team.", http.StatusConflict)
		return
	}
	if err := h.bindPersonal(client, me); err != nil {
		http.Error(w, "The personal vault could not be bound to this account.", http.StatusBadGateway)
		return
	}
	if err := h.ensureDevice(client, me); err != nil {
		http.Error(w, "This device could not be enrolled.", http.StatusBadGateway)
		return
	}
	org, err := client.CreateOrg(body.Org)
	if err != nil {
		http.Error(w, "The organization could not be created.", http.StatusBadGateway)
		return
	}
	vault, err := client.CreateTeamVault(org.ID, body.Vault)
	if err != nil {
		http.Error(w, "The team vault could not be created.", http.StatusBadGateway)
		return
	}
	h.svc.RememberTeam(vault.ID, vault.NameHint, vault.OrgID, vault.OwnerID, true)
	if err := h.openTeamKey(client, vault); err != nil {
		http.Error(w, "The team vault key could not be stored.", http.StatusBadGateway)
		return
	}
	_ = h.persist()
	write(w, map[string]any{"id": vault.ID, "name": vault.NameHint, "orgId": org.ID})
}

func (h *host) inviteTeam(w http.ResponseWriter, r *http.Request) {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	var body struct {
		OrgID string `json:"orgId"`
	}
	if err := read(r, &body); err != nil || body.OrgID == "" {
		http.Error(w, "Choose a team first.", http.StatusBadRequest)
		return
	}
	client, _, err := h.teamClient()
	if err != nil {
		http.Error(w, "Sign in and unlock the vault before inviting someone.", http.StatusConflict)
		return
	}
	token, err := client.Invite(body.OrgID, "member")
	if err != nil {
		http.Error(w, "The invite could not be created.", http.StatusBadGateway)
		return
	}
	write(w, map[string]string{"token": token})
}

func (h *host) acceptTeam(w http.ResponseWriter, r *http.Request) {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	var body struct {
		Token string `json:"token"`
	}
	if err := read(r, &body); err != nil || body.Token == "" {
		http.Error(w, "Paste the invite token.", http.StatusBadRequest)
		return
	}
	client, me, err := h.teamClient()
	if err != nil {
		http.Error(w, "Sign in and unlock the vault before accepting an invite.", http.StatusConflict)
		return
	}
	if err := h.bindPersonal(client, me); err != nil {
		http.Error(w, "The personal vault could not be bound to this account.", http.StatusBadGateway)
		return
	}
	if err := client.AcceptInvite(body.Token); err != nil {
		http.Error(w, "That invite could not be accepted.", http.StatusBadRequest)
		return
	}
	h.syncTeams(client, me)
	_ = h.persist()
	write(w, map[string]string{"status": "accepted"})
}

func (h *host) teamMembers(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	client, _, err := h.teamClient()
	if err != nil {
		http.Error(w, "Sign in and unlock the vault to see members.", http.StatusConflict)
		return
	}
	members, err := client.ListMembers(orgID)
	if err != nil {
		http.Error(w, "Members could not be loaded.", http.StatusBadGateway)
		return
	}
	write(w, map[string]any{"members": members})
}

func (h *host) grantTeam(w http.ResponseWriter, r *http.Request) {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	var body struct {
		VaultID     string `json:"vaultId"`
		PrincipalID string `json:"principalId"`
		DeviceID    string `json:"deviceId"`
		BoxPub      []byte `json:"boxPub"`
	}
	if err := read(r, &body); err != nil || body.VaultID == "" || body.PrincipalID == "" || body.DeviceID == "" || len(body.BoxPub) == 0 {
		http.Error(w, "Choose a member device to share with.", http.StatusBadRequest)
		return
	}
	client, me, err := h.teamClient()
	if err != nil {
		http.Error(w, "Sign in and unlock the vault before sharing.", http.StatusConflict)
		return
	}
	if err := h.bindPersonal(client, me); err != nil {
		http.Error(w, "The personal vault could not be bound to this account.", http.StatusBadGateway)
		return
	}
	if err := client.Grant(body.VaultID, body.PrincipalID, "edit"); err != nil {
		http.Error(w, "Access could not be granted.", http.StatusBadGateway)
		return
	}
	sealed, err := h.svc.SealTeamKey(body.VaultID, body.DeviceID, body.BoxPub)
	if err != nil {
		http.Error(w, "The vault key could not be wrapped for that device.", http.StatusBadGateway)
		return
	}
	if err := client.PutEnvelope(body.VaultID, sealed.DeviceID, sealed.Generation, sealed.Ciphertext, sealed.Signature); err != nil {
		http.Error(w, "The vault key could not be uploaded.", http.StatusBadGateway)
		return
	}
	write(w, map[string]string{"status": "shared"})
}

func (h *host) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	var body struct {
		VaultID     string `json:"vaultId"`
		OrgID       string `json:"orgId"`
		PrincipalID string `json:"principalId"`
	}
	if err := read(r, &body); err != nil || body.VaultID == "" || body.PrincipalID == "" || body.OrgID == "" {
		http.Error(w, "Choose a member to remove.", http.StatusBadRequest)
		return
	}
	client, me, err := h.teamClient()
	if err != nil {
		http.Error(w, "Sign in and unlock the vault before removing someone.", http.StatusConflict)
		return
	}
	if err := h.bindPersonal(client, me); err != nil {
		http.Error(w, "The personal vault could not be bound to this account.", http.StatusBadGateway)
		return
	}
	members, err := client.ListMembers(body.OrgID)
	if err != nil {
		http.Error(w, "Members could not be loaded.", http.StatusBadGateway)
		return
	}
	if err := client.Revoke(body.VaultID, body.PrincipalID); err != nil {
		http.Error(w, "That member could not be removed.", http.StatusBadGateway)
		return
	}
	var recipients []app.TeamRecipient
	if id := h.svc.DeviceID(); id != "" {
		if box := h.svc.DeviceBox(); len(box) > 0 {
			recipients = append(recipients, app.TeamRecipient{DeviceID: id, BoxPub: box})
		}
	}
	for _, member := range members {
		if member.ID == body.PrincipalID || member.ID == me.ID {
			continue
		}
		for _, device := range member.Devices {
			recipients = append(recipients, app.TeamRecipient{DeviceID: device.ID, BoxPub: device.BoxPub})
		}
	}
	sealed, err := h.svc.RotateTeam(body.VaultID, recipients)
	if err != nil {
		http.Error(w, "The vault key could not be rotated.", http.StatusBadGateway)
		return
	}
	for _, env := range sealed {
		if err := client.PutEnvelope(body.VaultID, env.DeviceID, env.Generation, env.Ciphertext, env.Signature); err != nil {
			http.Error(w, "The new vault key could not be shared.", http.StatusBadGateway)
			return
		}
	}
	h.reconcileTeam(client, body.VaultID)
	_ = h.persist()
	write(w, map[string]string{"status": "removed"})
}

func (h *host) selectVault(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "Choose a vault.", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetActiveVault(body.ID); err != nil {
		http.Error(w, "That vault is not on this device.", http.StatusNotFound)
		return
	}
	write(w, map[string]string{"id": body.ID})
}
