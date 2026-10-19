package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mistweaverco/withsecrets/apps/server/storage"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

func (s *Server) desktopStart(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.ProviderNames()) == 0 {
		s.setupPage(w, http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	state := q.Get("state")
	challenge := q.Get("code_challenge")
	method := q.Get("code_challenge_method")
	callback := q.Get("callback")
	if state == "" || challenge == "" || method != "S256" {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := validateCallback(callback); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_callback")
		return
	}
	instance, err := s.store.InstanceID(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "instance")
		return
	}
	providerVerifier, err := randomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	tx := storage.Transaction{
		ID:               mustID(),
		State:            state,
		StateHash:        hashToken(state),
		PKCEChallenge:    challenge,
		Callback:         callback,
		InstanceID:       instance,
		Expires:          time.Now().Add(5 * time.Minute),
		Provider:         q.Get("provider"),
		Nonce:            mustID(),
		ProviderVerifier: providerVerifier,
	}
	if err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if tx.Provider == "" {
		http.Redirect(w, r, "/auth/login?tx="+url.QueryEscape(tx.ID), http.StatusFound)
		return
	}
	s.redirectProvider(w, r, tx)
}

func (s *Server) desktopContinue(w http.ResponseWriter, r *http.Request) {
	tx, ok := s.loadTx(w, r, r.URL.Query().Get("tx"))
	if !ok {
		return
	}
	tx.Provider = r.URL.Query().Get("provider")
	s.redirectProvider(w, r, tx)
}

func (s *Server) redirectProvider(w http.ResponseWriter, r *http.Request, tx storage.Transaction) {
	if tx.Provider == "dev" {
		if !s.cfg.DevAuth {
			writeErr(w, http.StatusBadRequest, "provider_disabled")
			return
		}
		s.finishIdentity(w, r, tx, "dev", "https://dev.withsecrets.local", r.URL.Query().Get("subject"), r.URL.Query().Get("email"), "Dev User", true)
		return
	}
	p, ok := s.oauth[tx.Provider]
	if !ok {
		writeErr(w, http.StatusBadRequest, "provider_disabled")
		return
	}
	u, _ := url.Parse(p.AuthURL)
	q := u.Query()
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", strings.TrimRight(s.cfg.PublicBaseURL, "/")+"/auth/callback/"+p.Name)
	q.Set("response_type", "code")
	q.Set("scope", p.Scopes)
	q.Set("state", tx.ID)
	q.Set("nonce", tx.Nonce)
	if tx.ProviderVerifier != "" {
		q.Set("code_challenge", s256(tx.ProviderVerifier))
		q.Set("code_challenge_method", "S256")
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) authCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == "dev" {
		writeErr(w, http.StatusBadRequest, "provider_disabled")
		return
	}
	tx, ok := s.loadTx(w, r, r.URL.Query().Get("state"))
	if !ok {
		return
	}
	if r.URL.Query().Get("error") != "" {
		writeErr(w, http.StatusBadRequest, "provider_denied")
		return
	}
	ident, err := s.exchangeProviderCode(r, provider, r.URL.Query().Get("code"), tx)
	if err != nil {
		log.Printf("oauth exchange %s: %v", provider, err)
		writeErr(w, http.StatusBadRequest, "provider_rejected")
		return
	}
	s.finishIdentity(w, r, tx, provider, ident.Issuer, ident.Subject, ident.Email, ident.Name, ident.EmailVerified)
}

type identityResult struct {
	Issuer        string
	Subject       string
	Email         string
	Name          string
	EmailVerified bool
}

func (s *Server) finishIdentity(w http.ResponseWriter, r *http.Request, tx storage.Transaction, provider, issuer, subject, email, name string, verified bool) {
	if subject == "" {
		subject = "dev-user"
	}
	if provider != "dev" && !verified {
		writeErr(w, http.StatusBadRequest, "email_unverified")
		return
	}
	userID, err := s.store.UpsertIdentity(r.Context(), storage.Identity{
		Provider: provider, Issuer: issuer, Subject: subject, Email: email,
	}, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if err := s.store.ConsumeTransaction(r.Context(), tx.ID); err != nil {
		writeErr(w, http.StatusBadRequest, "transaction_used")
		return
	}
	code, err := randomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	if err := s.store.PutGrant(r.Context(), storage.Grant{
		CodeHash: hashToken(code), UserID: userID, PKCEChallenge: tx.PKCEChallenge,
		StateHash: tx.StateHash, Expires: time.Now().Add(s.cfg.GrantTTL),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: userID, Action: "login", Target: provider})
	access, _, err := s.issueSession(r, userID, "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session")
		return
	}
	s.setPortalCookie(w, access)
	if tx.Callback == "" {
		s.portalPage(w, r)
		return
	}
	u, _ := url.Parse(tx.Callback)
	q := u.Query()
	q.Set("code", code)
	q.Set("state", tx.State)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) loadTx(w http.ResponseWriter, r *http.Request, id string) (storage.Transaction, bool) {
	tx, err := s.store.Transaction(r.Context(), id)
	if err != nil || tx.Consumed || time.Now().After(tx.Expires) {
		writeErr(w, http.StatusBadRequest, "transaction")
		return storage.Transaction{}, false
	}
	return tx, true
}

func (s *Server) exchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"codeVerifier"`
		State        string `json:"state"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	userID, err := s.store.ConsumeGrant(r.Context(), hashToken(body.Code), s256(body.CodeVerifier), hashToken(body.State), time.Now())
	if err != nil {
		_ = s.store.AddAudit(r.Context(), storage.Audit{Action: "login_failed", Target: "exchange"})
		writeErr(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	access, refresh, err := s.issueSession(r, userID, "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": access, "refreshToken": refresh, "tokenType": "Bearer"})
}

func (s *Server) issueSession(r *http.Request, userID, deviceID string) (string, string, error) {
	access, err := randomToken()
	if err != nil {
		return "", "", err
	}
	refresh, err := randomToken()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	err = s.store.PutSession(r.Context(), storage.Session{
		ID: mustID(), UserID: userID, DeviceID: deviceID, FamilyID: mustID(),
		AccessHash: hashToken(access), RefreshHash: hashToken(refresh),
		AccessExp: now.Add(s.cfg.AccessTTL), RefreshExp: now.Add(s.cfg.RefreshTTL),
	})
	return access, refresh, err
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	access, err := randomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	refresh, err := randomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token")
		return
	}
	now := time.Now()
	err = s.store.RotateRefresh(r.Context(), hashToken(body.RefreshToken), storage.Session{
		ID: mustID(), AccessHash: hashToken(access), RefreshHash: hashToken(refresh),
		AccessExp: now.Add(s.cfg.AccessTTL), RefreshExp: now.Add(s.cfg.RefreshTTL),
	})
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_grant")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": access, "refreshToken": refresh, "tokenType": "Bearer"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token := bearerOrCookie(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sess, err := s.store.SessionByAccess(r.Context(), hashToken(token))
	if err != nil || time.Now().After(sess.AccessExp) {
		s.clearPortalCookie(w)
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	_ = s.store.RevokeFamily(r.Context(), sess.FamilyID)
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: sess.UserID, Action: "logout"})
	s.clearPortalCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) setPortalCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "ws_access",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.cfg.PublicBaseURL, "https://"),
		MaxAge:   int(s.cfg.AccessTTL.Seconds()),
	})
}

func (s *Server) clearPortalCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "ws_access",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	instance, _ := s.store.InstanceID(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"id": user, "instanceId": instance})
}

func validateCallback(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return errors.New("callback")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("callback")
	}
	if u.Scheme == "withsecrets" && u.Host == "auth" && u.Path == "/callback" {
		return nil
	}
	if u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost") && u.Path == "/auth/callback" {
		return nil
	}
	return errors.New("callback")
}

func mustID() string {
	t, err := randomToken()
	if err != nil {
		return "id"
	}
	return t
}

func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		RecoveryPolicy string `json:"recoveryPolicy"`
		GenerationID   string `json:"generationId"`
		Sequence       uint64 `json:"sequence"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	switch body.RecoveryPolicy {
	case vaultcrypto.PolicyPassphrase, vaultcrypto.PolicyHardwareOnly, vaultcrypto.PolicyHardwarePlusRecovery:
	default:
		writeErr(w, http.StatusBadRequest, "policy")
		return
	}
	if body.Sequence == 0 {
		body.Sequence = 1
	}
	if body.GenerationID == "" {
		body.GenerationID = mustID()
	}
	if err := s.store.EnsureAccount(r.Context(), storage.Account{
		ID: user, UserID: user, RecoveryPolicy: body.RecoveryPolicy, GenerationID: body.GenerationID,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	_ = s.store.SetAccountPolicy(r.Context(), user, body.RecoveryPolicy, body.GenerationID, body.Sequence)
	writeJSON(w, http.StatusOK, map[string]any{"accountId": user, "generationId": body.GenerationID})
}

func (s *Server) listSlots(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	slots, err := s.store.ListSlots(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

func (s *Server) createSlot(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || rejectPlaintext(body) != nil {
		writeErr(w, http.StatusBadRequest, "plaintext_rejected")
		return
	}
	var in struct {
		ID           string `json:"id"`
		GenerationID string `json:"generationId"`
		Type         string `json:"slotType"`
		Label        string `json:"publicLabel"`
		AlgVersion   int    `json:"algVersion"`
		PublicParams string `json:"publicParams"`
		Wrap         string `json:"encryptedKeyWrap"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Wrap == "" || in.Type == "" {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	wrap, err := base64.StdEncoding.DecodeString(in.Wrap)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if in.ID == "" {
		in.ID = mustID()
	}
	slot := storage.Slot{
		ID: in.ID, AccountID: user, GenerationID: in.GenerationID, Type: in.Type,
		Label: in.Label, AlgVersion: in.AlgVersion, PublicParams: []byte(in.PublicParams),
		Wrap: wrap, State: vaultcrypto.StatePending,
	}
	if err := s.store.PutSlot(r.Context(), slot); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "slot_create", Target: in.Type})
	writeJSON(w, http.StatusCreated, map[string]string{"id": slot.ID, "state": slot.State})
}

func (s *Server) verifySlot(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	slot, err := s.store.Slot(r.Context(), r.PathValue("id"))
	if err != nil || slot.AccountID != user {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		DeviceID string `json:"deviceId"`
		Proof    string `json:"clientProof"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	dev, err := s.store.Device(r.Context(), body.DeviceID)
	if err != nil || dev.UserID != user || dev.Status != "active" {
		writeErr(w, http.StatusForbidden, "device")
		return
	}
	sig, err := base64.StdEncoding.DecodeString(body.Proof)
	if err != nil || !ed25519.Verify(dev.SignPub, slotProof(slot), sig) {
		writeErr(w, http.StatusBadRequest, "bad_proof")
		return
	}
	if err := s.store.SetSlotState(r.Context(), slot.ID, vaultcrypto.StateActive); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": slot.ID, "state": vaultcrypto.StateActive})
}

func slotProof(slot storage.Slot) []byte {
	sum := vaultcrypto.HashEnvelope(slot.Wrap)
	return []byte("withsecrets/v1/slot-verify|" + slot.ID + "|" + slot.Type + "|" + string(sum[:]))
}

func (s *Server) deleteSlot(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	acct, err := s.store.Account(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	rows, err := s.store.ListSlots(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	var cryptoSlots []vaultcrypto.UnlockSlot
	for _, row := range rows {
		cryptoSlots = append(cryptoSlots, vaultcrypto.UnlockSlot{
			Context: vaultcrypto.SlotContext{SlotID: row.ID},
			Type:    row.Type,
			State:   row.State,
		})
	}
	if err := vaultcrypto.CanRemoveSlot(acct.RecoveryPolicy, cryptoSlots, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusConflict, "last_recovery_path")
		return
	}
	if err := s.store.SetSlotState(r.Context(), r.PathValue("id"), vaultcrypto.StateRevoked); err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "slot_revoke", Target: r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) listGenerations(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	gens, err := s.store.Generations(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generations": gens})
}

func (s *Server) rotateKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		GenerationID string `json:"generationId"`
		Sequence     uint64 `json:"sequence"`
		Manifest     string `json:"manifest"`
		Signature    string `json:"signature"`
		DeviceID     string `json:"deviceId"`
		Commit       bool   `json:"commit"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	dev, err := s.store.Device(r.Context(), body.DeviceID)
	if err != nil || dev.UserID != user || dev.Status != "active" {
		writeErr(w, http.StatusForbidden, "device")
		return
	}
	manifest, err := base64.StdEncoding.DecodeString(body.Manifest)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	sig, err := base64.StdEncoding.DecodeString(body.Signature)
	if err != nil || !ed25519.Verify(dev.SignPub, manifest, sig) {
		writeErr(w, http.StatusBadRequest, "bad_proof")
		return
	}
	acct, err := s.store.Account(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if body.Commit {
		if body.Sequence <= acct.Sequence {
			writeErr(w, http.StatusConflict, "rollback")
			return
		}
		if err := s.store.SetAccountPolicy(r.Context(), user, acct.RecoveryPolicy, body.GenerationID, body.Sequence); err != nil {
			writeErr(w, http.StatusConflict, "rollback")
			return
		}
	}
	state := vaultcrypto.StatePending
	if body.Commit {
		state = vaultcrypto.StateActive
	}
	_ = s.store.PutGeneration(r.Context(), storage.KeyGeneration{
		ID: body.GenerationID, AccountID: user, Sequence: body.Sequence, State: state,
		Manifest: manifest, Signature: sig, Signer: body.DeviceID,
	})
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "rotation", Target: body.GenerationID})
	writeJSON(w, http.StatusOK, map[string]any{"state": state, "sequence": body.Sequence})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	devs, err := s.store.ListDevices(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devs})
}

func (s *Server) enrollDevice(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		SignPub     string `json:"signPub"`
		BoxPub      string `json:"boxPub"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	sign, err := base64.StdEncoding.DecodeString(body.SignPub)
	if err != nil || len(sign) != ed25519.PublicKeySize {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	box, err := base64.StdEncoding.DecodeString(body.BoxPub)
	if err != nil || len(box) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	existing, _ := s.store.ListDevices(r.Context(), user)
	status := "pending"
	if len(existing) == 0 {
		status = "active"
	}
	d := storage.Device{ID: mustID(), UserID: user, SignPub: sign, BoxPub: box, Fingerprint: body.Fingerprint, Status: status}
	if err := s.store.PutDevice(r.Context(), d); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": d.ID, "status": status, "fingerprint": d.Fingerprint})
}

func (s *Server) approveDevice(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	dev, err := s.store.Device(r.Context(), r.PathValue("id"))
	if err != nil || dev.UserID != user {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.store.SetDeviceStatus(r.Context(), dev.ID, "active"); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": dev.ID, "status": "active"})
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	dev, err := s.store.Device(r.Context(), r.PathValue("id"))
	if err != nil || dev.UserID != user {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.store.SetDeviceStatus(r.Context(), dev.ID, "revoked"); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	_ = s.store.AddAudit(r.Context(), storage.Audit{ActorID: user, Action: "device_revoke", Target: dev.ID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) portalPage(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.ProviderNames()) == 0 && (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/auth")) {
		s.setupPage(w, http.StatusServiceUnavailable)
		return
	}
	if s.portal != nil {
		s.portal.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")
	_, _ = w.Write([]byte(builtinPortal))
}

func (s *Server) setupPage(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(setupHTML))
}

const setupHTML = `<!doctype html><html lang="en" data-theme="night"><head><meta charset="utf-8"><title>withsecrets setup</title>
<style>body{font-family:system-ui,sans-serif;background:#0f172a;color:#e2e8f0;margin:0;display:grid;place-items:center;min-height:100vh}main{max-width:36rem;padding:2rem}h1{font-size:1.6rem;margin:0 0 1rem}a{color:#93c5fd}</style></head>
<body><main><h1>Sign-in is locked</h1><p>This withsecrets instance has no identity provider configured. Vault data is not available anonymously.</p>
<p>Set GitHub, Google, or Microsoft OAuth credentials and restart. See <code>docs/operations/self-hosting.md</code>.</p>
<p><a href="/server">Server status</a> stays available and does not show OAuth secrets.</p></main></body></html>`

const builtinPortal = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>withsecrets</title>
<style>body{font-family:system-ui,sans-serif;background:#12141a;color:#e8eaed;margin:0}main{max-width:40rem;margin:4rem auto;padding:0 1rem}a{color:#9ec1ff}</style></head>
<body><main><h1>withsecrets</h1><p>Sign in with your identity provider, then unlock the vault on a device that holds a hardware key, passphrase, or recovery kit. Signing in does not decrypt your vault.</p>
<p id="providers">Loading providers…</p></main>
<script>fetch('/auth/providers').then(r=>r.json()).then(d=>{const el=document.getElementById('providers');const tx=new URLSearchParams(location.search).get('tx');if(!d.providers||!d.providers.length){el.textContent='No providers configured.';return}el.innerHTML=d.providers.map(p=>'<p><a href="/auth/desktop/continue?tx='+encodeURIComponent(tx||'')+'&provider='+encodeURIComponent(p)+'">Continue with '+p+'</a></p>').join('')}).catch(()=>{document.getElementById('providers').textContent='Could not load providers.'})</script></body></html>`
