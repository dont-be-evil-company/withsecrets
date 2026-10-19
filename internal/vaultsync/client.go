package vaultsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls the vault HTTP API with a bearer token.
type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

// Vault is one personal vault the server returned.
type Vault struct {
	ID             string
	NameHint       string
	OwnerID        string
	OrgID          string
	Revision       int64
	AccountKeyWrap []byte
}

// Item is one ciphertext row.
type Item struct {
	ID         string
	VaultID    string
	Ciphertext []byte
	Revision   int64
	Deleted    bool
}

// Change is one sync-feed entry.
type Change struct {
	Seq      int64
	VaultID  string
	Kind     string
	ObjectID string
	Revision int64
}

// Account is the signed-in user and server instance.
type Account struct {
	ID       string
	Instance string
}

// RemoteSlot is an encrypted unlock slot stored for the account.
type RemoteSlot struct {
	ID           string
	AccountID    string
	GenerationID string
	Type         string
	Label        string
	AlgVersion   int
	PublicParams []byte
	Wrap         []byte
	State        string
}

// StatusError is a non-2xx API response.
type StatusError struct {
	Status int
	Body   []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("vaultsync: http %d", e.Status)
}

// NewClient returns a client for one server and access token.
func NewClient(base, token string) *Client {
	return &Client{
		Base:  strings.TrimRight(base, "/"),
		Token: token,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Me reads the signed-in account and instance id.
func (c *Client) Me() (Account, error) {
	var body struct {
		ID       string `json:"id"`
		Instance string `json:"instanceId"`
	}
	if err := c.do(http.MethodGet, "/api/v1/accounts/me", nil, &body); err != nil {
		return Account{}, err
	}
	if body.ID == "" || body.Instance == "" {
		return Account{}, fmt.Errorf("vaultsync: account")
	}
	return Account{ID: body.ID, Instance: body.Instance}, nil
}

// Org is an organization the caller belongs to.
type Org struct {
	ID      string
	Name    string
	OwnerID string
}

// MemberDevice is one active device public key.
type MemberDevice struct {
	ID     string `json:"id"`
	BoxPub []byte `json:"boxPub"`
}

// Member is one organization member.
type Member struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"displayName"`
	Role        string         `json:"role"`
	Devices     []MemberDevice `json:"devices"`
}

// RemoteEnvelope is a vault-key wrap stored for one device.
type RemoteEnvelope struct {
	VaultID           string
	Generation        int64
	RecipientDeviceID string
	Ciphertext        []byte
	Signature         []byte
}

func (c *Client) vaultByID(id string) (Vault, error) {
	vaults, err := c.ListVaults()
	if err != nil {
		return Vault{}, err
	}
	for _, vault := range vaults {
		if vault.ID == id {
			return vault, nil
		}
	}
	return Vault{}, fmt.Errorf("vaultsync: vault")
}

// ListVaults returns every vault the caller can access.
func (c *Client) ListVaults() ([]Vault, error) {
	var body struct {
		Vaults []Vault `json:"vaults"`
	}
	if err := c.do(http.MethodGet, "/api/v1/vaults", nil, &body); err != nil {
		return nil, err
	}
	return body.Vaults, nil
}

// CreateOrg creates an organization owned by the caller.
func (c *Client) CreateOrg(name string) (Org, error) {
	var org Org
	if err := c.do(http.MethodPost, "/api/v1/organizations", map[string]string{"name": name}, &org); err != nil {
		return Org{}, err
	}
	if org.ID == "" {
		return Org{}, fmt.Errorf("vaultsync: organization")
	}
	return org, nil
}

// CreateTeamVault creates an organization vault.
func (c *Client) CreateTeamVault(orgID, name string) (Vault, error) {
	var vault Vault
	err := c.do(http.MethodPost, "/api/v1/vaults", map[string]string{"nameHint": name, "orgId": orgID}, &vault)
	if err != nil {
		return Vault{}, err
	}
	if vault.ID == "" {
		return Vault{}, fmt.Errorf("vaultsync: vault")
	}
	return vault, nil
}

// Invite creates a one-time organization invite token.
func (c *Client) Invite(orgID, role string) (string, error) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.do(http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations", map[string]string{"role": role}, &body); err != nil {
		return "", err
	}
	if body.Token == "" {
		return "", fmt.Errorf("vaultsync: invite")
	}
	return body.Token, nil
}

// AcceptInvite joins the organization for a token.
func (c *Client) AcceptInvite(token string) error {
	return c.do(http.MethodPost, "/api/v1/invitations/"+token+"/accept", map[string]string{}, nil)
}

// ListMembers returns members and their active device keys.
func (c *Client) ListMembers(orgID string) ([]Member, error) {
	var body struct {
		Members []Member `json:"members"`
	}
	if err := c.do(http.MethodGet, "/api/v1/organizations/"+orgID+"/members", nil, &body); err != nil {
		return nil, err
	}
	return body.Members, nil
}

// EnrollDevice registers this desktop's public keys.
func (c *Client) EnrollDevice(signPub, boxPub []byte) (id, status string, err error) {
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	err = c.do(http.MethodPost, "/api/v1/devices/enrollment-request", map[string]string{
		"signPub": b64(signPub), "boxPub": b64(boxPub),
	}, &body)
	return body.ID, body.Status, err
}

// ApproveDevice marks one of the caller's devices active.
func (c *Client) ApproveDevice(id string) error {
	return c.do(http.MethodPost, "/api/v1/devices/"+id+"/approve", map[string]string{}, nil)
}

// PutEnvelope stores a vault-key wrap for one device.
func (c *Client) PutEnvelope(vaultID, deviceID string, generation int64, ciphertext, signature []byte) error {
	return c.do(http.MethodPost, "/api/v1/vaults/"+vaultID+"/key-envelopes", map[string]any{
		"recipientDeviceId": deviceID,
		"ciphertext":        b64(ciphertext),
		"signature":         b64(signature),
		"generation":        generation,
	}, nil)
}

// ListEnvelopes returns vault-key wraps for a vault.
func (c *Client) ListEnvelopes(vaultID string) ([]RemoteEnvelope, error) {
	var body struct {
		Envelopes []RemoteEnvelope `json:"envelopes"`
	}
	if err := c.do(http.MethodGet, "/api/v1/vaults/"+vaultID+"/key-envelopes", nil, &body); err != nil {
		return nil, err
	}
	return body.Envelopes, nil
}

// Grant gives a person access to a vault.
func (c *Client) Grant(vaultID, principalID, permission string) error {
	return c.do(http.MethodPost, "/api/v1/vaults/"+vaultID+"/members", map[string]string{
		"principalId": principalID, "permission": permission,
	}, nil)
}

// Revoke removes a person from a vault.
func (c *Client) Revoke(vaultID, principalID string) error {
	return c.do(http.MethodDelete, "/api/v1/vaults/"+vaultID+"/members/"+principalID, nil, nil)
}

// EnsurePersonal returns the caller's personal vault, creating it when missing.
// prefer is used when that vault is still in the list.
func (c *Client) EnsurePersonal(prefer string) (Vault, error) {
	me, err := c.Me()
	if err != nil {
		return Vault{}, err
	}
	var body struct {
		Vaults []Vault `json:"vaults"`
	}
	if err := c.do(http.MethodGet, "/api/v1/vaults", nil, &body); err != nil {
		return Vault{}, err
	}
	if prefer != "" {
		for _, v := range body.Vaults {
			if v.ID == prefer && v.OrgID == "" && v.OwnerID == me.ID {
				return v, nil
			}
		}
	}
	for _, v := range body.Vaults {
		if v.OrgID == "" && v.NameHint == "personal" && v.OwnerID == me.ID {
			return v, nil
		}
	}
	var created Vault
	if err := c.do(http.MethodPost, "/api/v1/vaults", map[string]string{"nameHint": "personal"}, &created); err != nil {
		return Vault{}, err
	}
	if created.ID == "" {
		return Vault{}, fmt.Errorf("vaultsync: vault")
	}
	return created, nil
}

// PutAccountWrap stores the account-wrapped vault key.
func (c *Client) PutAccountWrap(vaultID string, wrap []byte) error {
	return c.do(http.MethodPut, "/api/v1/vaults/"+vaultID+"/account-key", map[string]string{
		"accountKeyWrap": b64(wrap),
	}, nil)
}

// Changes returns one page of sync events after cursor.
func (c *Client) Changes(cursor int64) ([]Change, error) {
	var body struct {
		Changes []Change `json:"changes"`
	}
	path := fmt.Sprintf("/api/v1/sync/changes?cursor=%d", cursor)
	if err := c.do(http.MethodGet, path, nil, &body); err != nil {
		return nil, err
	}
	return body.Changes, nil
}

// GetItem loads one item.
func (c *Client) GetItem(id string) (Item, error) {
	var item Item
	if err := c.do(http.MethodGet, "/api/v1/items/"+id, nil, &item); err != nil {
		return Item{}, err
	}
	return item, nil
}

// CreateItem inserts ciphertext under a client-chosen id.
func (c *Client) CreateItem(vaultID, id string, ciphertext []byte) (Item, error) {
	var item Item
	err := c.do(http.MethodPost, "/api/v1/vaults/"+vaultID+"/items", map[string]any{
		"id": id, "ciphertext": b64(ciphertext), "cryptoVersion": 1, "expectedRevision": 0,
	}, &item)
	return item, err
}

// UpdateItem compare-and-swaps one item.
func (c *Client) UpdateItem(id string, ciphertext []byte, expected int64) (Item, error) {
	var item Item
	err := c.do(http.MethodPut, "/api/v1/items/"+id, map[string]any{
		"ciphertext": b64(ciphertext), "cryptoVersion": 1, "expectedRevision": expected,
	}, &item)
	return item, err
}

// DeleteItem tombstones one item.
func (c *Client) DeleteItem(id string, expected int64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/v1/items/%s?expectedRevision=%d", id, expected), nil, nil)
}

// Enroll registers the account recovery policy so unlock slots can be stored.
func (c *Client) Enroll(policy, generation string) error {
	if generation == "" {
		generation = "g1"
	}
	return c.do(http.MethodPost, "/api/v1/accounts/enroll", map[string]any{
		"recoveryPolicy": policy, "generationId": generation, "sequence": 1,
	}, nil)
}

// PutSlot uploads one encrypted unlock slot.
func (c *Client) PutSlot(slot RemoteSlot) error {
	return c.do(http.MethodPost, "/api/v1/accounts/unlock-slots", map[string]any{
		"id":               slot.ID,
		"generationId":     slot.GenerationID,
		"slotType":         slot.Type,
		"publicLabel":      slot.Label,
		"algVersion":       slot.AlgVersion,
		"publicParams":     string(slot.PublicParams),
		"encryptedKeyWrap": b64(slot.Wrap),
	}, nil)
}

// ListSlots returns encrypted unlock slots for the signed-in account.
func (c *Client) ListSlots() ([]RemoteSlot, error) {
	var body struct {
		Slots []RemoteSlot `json:"slots"`
	}
	if err := c.do(http.MethodGet, "/api/v1/accounts/unlock-slots", nil, &body); err != nil {
		return nil, err
	}
	return body.Slots, nil
}

func (c *Client) do(method, path string, body any, dst any) error {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.Base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ht := c.HTTP
	if ht == nil {
		ht = http.DefaultClient
	}
	resp, err := ht.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StatusError{Status: resp.StatusCode, Body: raw}
	}
	if dst == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func b64(b []byte) string {
	return base64Encode(b)
}
