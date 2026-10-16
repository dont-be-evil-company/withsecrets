package vaultsync

import (
	"encoding/base64"
	"encoding/json"
)

func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func (o *Org) UnmarshalJSON(raw []byte) error {
	var body struct {
		ID      string `json:"ID"`
		Name    string `json:"Name"`
		OwnerID string `json:"OwnerID"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*o = Org{ID: body.ID, Name: body.Name, OwnerID: body.OwnerID}
	return nil
}

func (m *Member) UnmarshalJSON(raw []byte) error {
	var body struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Devices     []struct {
			ID     string `json:"id"`
			BoxPub []byte `json:"boxPub"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*m = Member{ID: body.ID, DisplayName: body.DisplayName, Role: body.Role}
	for _, d := range body.Devices {
		m.Devices = append(m.Devices, MemberDevice{ID: d.ID, BoxPub: d.BoxPub})
	}
	return nil
}

func (e *RemoteEnvelope) UnmarshalJSON(raw []byte) error {
	var body struct {
		VaultID           string `json:"VaultID"`
		Generation        int64  `json:"Generation"`
		RecipientDeviceID string `json:"RecipientDeviceID"`
		Ciphertext        []byte `json:"Ciphertext"`
		Signature         []byte `json:"Signature"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*e = RemoteEnvelope{
		VaultID: body.VaultID, Generation: body.Generation, RecipientDeviceID: body.RecipientDeviceID,
		Ciphertext: body.Ciphertext, Signature: body.Signature,
	}
	return nil
}

func (v *Vault) UnmarshalJSON(raw []byte) error {
	var body struct {
		ID             string `json:"ID"`
		NameHint       string `json:"NameHint"`
		OwnerID        string `json:"OwnerID"`
		OrgID          string `json:"OrgID"`
		Revision       int64  `json:"Revision"`
		AccountKeyWrap []byte `json:"AccountKeyWrap"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*v = Vault{
		ID: body.ID, NameHint: body.NameHint, OwnerID: body.OwnerID, OrgID: body.OrgID,
		Revision: body.Revision, AccountKeyWrap: body.AccountKeyWrap,
	}
	return nil
}

func (it *Item) UnmarshalJSON(raw []byte) error {
	var body struct {
		ID         string `json:"id"`
		VaultID    string `json:"vaultId"`
		Ciphertext []byte `json:"ciphertext"`
		Revision   int64  `json:"revision"`
		Deleted    bool   `json:"deleted"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*it = Item{ID: body.ID, VaultID: body.VaultID, Ciphertext: body.Ciphertext, Revision: body.Revision, Deleted: body.Deleted}
	return nil
}

func (c *Change) UnmarshalJSON(raw []byte) error {
	var body struct {
		Seq      int64  `json:"Seq"`
		VaultID  string `json:"VaultID"`
		Kind     string `json:"Kind"`
		ObjectID string `json:"ObjectID"`
		Revision int64  `json:"Revision"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*c = Change{Seq: body.Seq, VaultID: body.VaultID, Kind: body.Kind, ObjectID: body.ObjectID, Revision: body.Revision}
	return nil
}

func (s *RemoteSlot) UnmarshalJSON(raw []byte) error {
	var body struct {
		ID           string `json:"ID"`
		AccountID    string `json:"AccountID"`
		GenerationID string `json:"GenerationID"`
		Type         string `json:"Type"`
		Label        string `json:"Label"`
		AlgVersion   int    `json:"AlgVersion"`
		PublicParams []byte `json:"PublicParams"`
		Wrap         []byte `json:"Wrap"`
		State        string `json:"State"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	*s = RemoteSlot{
		ID: body.ID, AccountID: body.AccountID, GenerationID: body.GenerationID, Type: body.Type,
		Label: body.Label, AlgVersion: body.AlgVersion, PublicParams: body.PublicParams, Wrap: body.Wrap, State: body.State,
	}
	return nil
}
