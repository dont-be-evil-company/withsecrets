package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mistweaverco/withsecrets/apps/server/storage"
)

func (s *Server) canAccess(r *http.Request, userID, vaultID, need string) (storage.Vault, bool) {
	v, err := s.store.Vault(r.Context(), vaultID)
	if err != nil {
		return storage.Vault{}, false
	}
	if v.OwnerID == userID {
		return v, true
	}
	mem, ok := s.store.Membership(r.Context(), vaultID, userID)
	if !ok {
		return storage.Vault{}, false
	}
	rank := map[string]int{"view": 1, "edit": 2, "manage": 3}
	if rank[mem.Permission] < rank[need] {
		return storage.Vault{}, false
	}
	return v, true
}

func (s *Server) listVaults(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	vaults, err := s.store.ListVaults(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	out := make([]storage.Vault, 0, len(vaults))
	for _, v := range vaults {
		out = append(out, vaultView(user, v))
	}
	writeJSON(w, http.StatusOK, map[string]any{"vaults": out})
}

func vaultView(user string, v storage.Vault) storage.Vault {
	if v.OwnerID != user {
		v.AccountKeyWrap = nil
	}
	return v
}

func (s *Server) createVault(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		NameHint string `json:"nameHint"`
		OrgID    string `json:"orgId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.OrgID != "" {
		role, ok := s.store.OrgRole(r.Context(), body.OrgID, user)
		if !ok || (role != "owner" && role != "admin") {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	v := storage.Vault{ID: mustID(), NameHint: body.NameHint, OwnerID: user, OrgID: body.OrgID}
	if err := s.store.CreateVault(r.Context(), v); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) getVault(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	v, ok := s.canAccess(r, user, r.PathValue("id"), "view")
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, vaultView(user, v))
}

func (s *Server) putAccountKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	v, err := s.store.Vault(r.Context(), r.PathValue("id"))
	if err != nil || v.OwnerID != user {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if v.OrgID != "" {
		writeErr(w, http.StatusBadRequest, "personal_only")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || rejectPlaintext(raw) != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	var body struct {
		AccountKeyWrap string `json:"accountKeyWrap"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	wrap, err := base64.StdEncoding.DecodeString(body.AccountKeyWrap)
	if err != nil || len(wrap) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if err := s.store.SetAccountKeyWrap(r.Context(), v.ID, wrap); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	v.AccountKeyWrap = wrap
	writeJSON(w, http.StatusOK, vaultView(user, v))
}

func (s *Server) deleteVault(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	v, err := s.store.Vault(r.Context(), r.PathValue("id"))
	if err != nil || v.OwnerID != user {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.store.DeleteVault(r.Context(), v.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type itemBody struct {
	ID             string `json:"id"`
	Ciphertext     string `json:"ciphertext"`
	CryptoVersion  int    `json:"cryptoVersion"`
	Expected       int64  `json:"expectedRevision"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func validItemID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

func readItemBody(r *http.Request) (itemBody, []byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		return itemBody{}, nil, err
	}
	if err := rejectPlaintext(raw); err != nil {
		return itemBody{}, raw, err
	}
	var body itemBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return itemBody{}, raw, err
	}
	return body, raw, nil
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	vaultID := r.PathValue("id")
	if _, ok := s.canAccess(r, user, vaultID, "edit"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	body, _, err := readItemBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plaintext_rejected")
		return
	}
	ct, err := base64.StdEncoding.DecodeString(body.Ciphertext)
	if err != nil || len(ct) == 0 || body.CryptoVersion != 1 || (body.ID != "" && !validItemID(body.ID)) {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	item, err := s.store.PutItem(r.Context(), storage.Item{
		ID: body.ID, VaultID: vaultID, Ciphertext: ct, CryptoVersion: body.CryptoVersion, Idempotency: body.IdempotencyKey,
	}, 0)
	if err != nil {
		writeErr(w, http.StatusConflict, "conflict")
		return
	}
	writeJSON(w, http.StatusCreated, itemJSON(item))
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	vaultID := r.PathValue("id")
	if _, ok := s.canAccess(r, user, vaultID, "view"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	items, err := s.store.ListItems(r.Context(), vaultID, cursor, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, itemJSON(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	user, item, ok := s.itemFor(w, r, "view")
	if !ok {
		return
	}
	_ = user
	writeJSON(w, http.StatusOK, itemJSON(item))
}

func (s *Server) putItem(w http.ResponseWriter, r *http.Request) {
	user, current, ok := s.itemFor(w, r, "edit")
	if !ok {
		return
	}
	_ = user
	body, _, err := readItemBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plaintext_rejected")
		return
	}
	ct, err := base64.StdEncoding.DecodeString(body.Ciphertext)
	if err != nil || len(ct) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	item, err := s.store.PutItem(r.Context(), storage.Item{
		ID: current.ID, VaultID: current.VaultID, Ciphertext: ct, CryptoVersion: 1,
	}, body.Expected)
	var conflict *storage.ConflictError
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "revision_conflict", "currentRevision": conflict.Current})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, itemJSON(item))
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	_, current, ok := s.itemFor(w, r, "edit")
	if !ok {
		return
	}
	expected, _ := strconv.ParseInt(r.URL.Query().Get("expectedRevision"), 10, 64)
	item, err := s.store.Tombstone(r.Context(), current.ID, expected)
	var conflict *storage.ConflictError
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "revision_conflict", "currentRevision": conflict.Current})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, itemJSON(item))
}

func (s *Server) itemHistory(w http.ResponseWriter, r *http.Request) {
	_, current, ok := s.itemFor(w, r, "view")
	if !ok {
		return
	}
	hist, err := s.store.ItemHistory(r.Context(), current.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": hist})
}

func (s *Server) itemFor(w http.ResponseWriter, r *http.Request, need string) (string, storage.Item, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return "", storage.Item{}, false
	}
	item, err := s.store.Item(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return "", storage.Item{}, false
	}
	if _, ok := s.canAccess(r, user, item.VaultID, need); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return "", storage.Item{}, false
	}
	return user, item, true
}

func itemJSON(item storage.Item) map[string]any {
	return map[string]any{
		"id": item.ID, "vaultId": item.VaultID,
		"ciphertext":    base64.StdEncoding.EncodeToString(item.Ciphertext),
		"cryptoVersion": item.CryptoVersion, "revision": item.Revision, "deleted": item.Deleted,
	}
}

func (s *Server) listOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	orgs, err := s.store.ListOrgs(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizations": orgs})
}

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	o := storage.Org{ID: mustID(), Name: body.Name, OwnerID: user}
	if err := s.store.CreateOrg(r.Context(), o, "owner"); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (s *Server) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	role, okRole := s.store.OrgRole(r.Context(), r.PathValue("id"), user)
	if !okRole || (role != "owner" && role != "admin") {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	members, err := s.store.ListOrgMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		devs := make([]map[string]string, 0, len(m.Devices))
		for _, d := range m.Devices {
			devs = append(devs, map[string]string{
				"id": d.ID, "boxPub": base64.StdEncoding.EncodeToString(d.BoxPub),
			})
		}
		out = append(out, map[string]any{
			"id": m.UserID, "displayName": m.DisplayName, "role": m.Role, "devices": devs,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	role, okRole := s.store.OrgRole(r.Context(), r.PathValue("id"), user)
	if !okRole || (role != "owner" && role != "admin") {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.Role == "" {
		body.Role = "member"
	}
	token, err := randomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	sum := sha256.Sum256([]byte(token))
	if err := s.store.PutInvite(r.Context(), storage.Invite{
		TokenHash: encodeHex(sum[:]), OrgID: r.PathValue("id"), Role: body.Role,
		Expires: time.Now().Add(48 * time.Hour),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "invite", Target: r.PathValue("id")})
	writeJSON(w, http.StatusCreated, map[string]string{"token": token, "role": body.Role})
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	sum := sha256.Sum256([]byte(r.PathValue("token")))
	if err := s.store.AcceptInvite(r.Context(), encodeHex(sum[:]), user); err != nil {
		writeErr(w, http.StatusBadRequest, "invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if _, ok := s.canAccess(r, user, r.PathValue("id"), "manage"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		PrincipalID string `json:"principalId"`
		Permission  string `json:"permission"`
	}
	if err := decodeJSON(r, &body); err != nil || body.PrincipalID == "" {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.Permission != "view" && body.Permission != "edit" && body.Permission != "manage" {
		writeErr(w, http.StatusBadRequest, "permission")
		return
	}
	if err := s.store.PutMembership(r.Context(), storage.Membership{
		VaultID: r.PathValue("id"), PrincipalID: body.PrincipalID, Permission: body.Permission,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "vault_share", Target: r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]string{"status": "granted"})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	vaultID := r.PathValue("id")
	if _, ok := s.canAccess(r, user, vaultID, "manage"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	principal := r.PathValue("principalId")
	if err := s.store.DeleteMembership(r.Context(), vaultID, principal); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	devs, _ := s.store.ListDevices(r.Context(), principal)
	for _, d := range devs {
		_ = s.store.DeleteEnvelopesForDevice(r.Context(), vaultID, d.ID)
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "vault_revoke", Target: principal})
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) listEnvelopes(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if _, ok := s.canAccess(r, user, r.PathValue("id"), "view"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	envs, err := s.store.ListEnvelopes(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"envelopes": envs})
}

func (s *Server) putEnvelope(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if _, ok := s.canAccess(r, user, r.PathValue("id"), "manage"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		RecipientDeviceID string `json:"recipientDeviceId"`
		Ciphertext        string `json:"ciphertext"`
		Signature         string `json:"signature"`
		Generation        int64  `json:"generation"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	ct, err := base64.StdEncoding.DecodeString(body.Ciphertext)
	if err != nil || len(ct) == 0 || body.RecipientDeviceID == "" {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	sig, _ := base64.StdEncoding.DecodeString(body.Signature)
	if err := s.store.PutEnvelope(r.Context(), storage.Envelope{
		VaultID: r.PathValue("id"), Generation: body.Generation, RecipientDeviceID: body.RecipientDeviceID,
		Ciphertext: ct, Signature: sig,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "stored"})
}

func (s *Server) rotateVault(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if _, ok := s.canAccess(r, user, r.PathValue("id"), "manage"); !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		RemoveDeviceID string `json:"removeDeviceId"`
		Generation     int64  `json:"generation"`
		Envelopes      []struct {
			RecipientDeviceID string `json:"recipientDeviceId"`
			Ciphertext        string `json:"ciphertext"`
			Signature         string `json:"signature"`
		} `json:"envelopes"`
	}
	if err := decodeJSON(r, &body); err != nil || len(body.Envelopes) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.RemoveDeviceID != "" {
		_ = s.store.DeleteEnvelopesForDevice(r.Context(), r.PathValue("id"), body.RemoveDeviceID)
	}
	for _, e := range body.Envelopes {
		ct, err := base64.StdEncoding.DecodeString(e.Ciphertext)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_body")
			return
		}
		sig, _ := base64.StdEncoding.DecodeString(e.Signature)
		if err := s.store.PutEnvelope(r.Context(), storage.Envelope{
			VaultID: r.PathValue("id"), Generation: body.Generation, RecipientDeviceID: e.RecipientDeviceID,
			Ciphertext: ct, Signature: sig,
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, "store")
			return
		}
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "vault_rotation", Target: r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]string{"status": "rotated"})
}

func (s *Server) startUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": mustID(), "ownerId": user})
}

func (s *Server) putChunk(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 {
		writeErr(w, http.StatusBadRequest, "chunk")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil || len(body) == 0 {
		writeErr(w, http.StatusBadRequest, "chunk")
		return
	}
	sum := sha256.Sum256(body)
	if err := s.store.PutChunk(r.Context(), storage.Chunk{
		AttachmentID: r.PathValue("id"), OwnerID: user, N: n, Sum: sum[:], Body: body,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"n": n, "sha256": base64.StdEncoding.EncodeToString(sum[:])})
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Chunks []struct {
			N      int    `json:"n"`
			SHA256 string `json:"sha256"`
		} `json:"chunks"`
	}
	if err := decodeJSON(r, &body); err != nil || len(body.Chunks) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	for _, c := range body.Chunks {
		got, err := s.store.Chunk(r.Context(), r.PathValue("id"), c.N)
		if err != nil || got.OwnerID != user {
			writeErr(w, http.StatusBadRequest, "chunk")
			return
		}
		want, err := base64.StdEncoding.DecodeString(c.SHA256)
		if err != nil || !bytesEqual(want, got.Sum) {
			writeErr(w, http.StatusBadRequest, "checksum")
			return
		}
	}
	if err := s.store.CompleteAttachment(r.Context(), r.PathValue("id"), user); err != nil {
		writeErr(w, http.StatusBadRequest, "chunk")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "complete"})
}

func (s *Server) getChunk(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	n, _ := strconv.Atoi(r.PathValue("n"))
	c, err := s.store.Chunk(r.Context(), r.PathValue("id"), n)
	if err != nil || c.OwnerID != user || !c.Complete {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(c.Body)
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	ch, err := s.store.Changes(r.Context(), user, cursor, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": ch})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		writeErr(w, http.StatusInternalServerError, "stream")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	ch, err := s.store.Changes(r.Context(), user, cursor, 1)
	if err != nil {
		return
	}
	seq := cursor
	if len(ch) > 0 {
		seq = ch[0].Seq
	}
	_, _ = w.Write([]byte("data: {\"cursor\":" + strconv.FormatInt(seq, 10) + "}\n\n"))
	flusher.Flush()
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := s.store.ListAudit(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	for _, row := range rows {
		if containsSecret(row.Metadata) || containsSecret(row.Target) {
			writeErr(w, http.StatusInternalServerError, "audit")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": rows})
}

func containsSecret(s string) bool {
	lower := strings.ToLower(s)
	for _, word := range []string{"password", "bearer ", "masterkey", "pin="} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func encodeHex(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
