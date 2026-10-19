package storage

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Postgres is the server source of truth. Payloads are ciphertext and metadata.
type Postgres struct {
	pool *pgxpool.Pool
}

// OpenPostgres connects, migrates, and returns a store.
func OpenPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	p := &Postgres{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

// Close releases the pool.
func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) migrate(ctx context.Context) error {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE version = '001'`).Scan(&n)
	if err != nil || n == 0 {
		sqlBytes, err := migrationFS.ReadFile("migrations/001_init.sql")
		if err != nil {
			return err
		}
		if _, err := p.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return err
		}
		if _, err := p.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ('001') ON CONFLICT DO NOTHING`); err != nil {
			return err
		}
	}
	// Local databases created before provider PKCE still need the column.
	_, err = p.pool.Exec(ctx, `ALTER TABLE login_transactions ADD COLUMN IF NOT EXISTS provider_code_verifier TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	sqlBytes, err := migrationFS.ReadFile("migrations/002_account_key_wrap.sql")
	if err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx, string(sqlBytes)); err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ('002') ON CONFLICT DO NOTHING`)
	return err
}

func (p *Postgres) Ready(ctx context.Context) error {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE version = '001'`).Scan(&n)
	if err != nil || n == 0 {
		if err == nil {
			err = errors.New("migrations incomplete")
		}
		return err
	}
	return p.pool.Ping(ctx)
}

func (p *Postgres) InstanceID(ctx context.Context) (string, error) {
	var id string
	err := p.pool.QueryRow(ctx, `SELECT id FROM server_instance LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	id = newID()
	_, err = p.pool.Exec(ctx, `INSERT INTO server_instance (id) VALUES ($1)`, id)
	return id, err
}

func (p *Postgres) CreateTransaction(ctx context.Context, tx Transaction) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO login_transactions
		(id, state, state_hash, pkce_challenge, callback, instance_id, expires_at, provider, nonce, provider_code_verifier)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		tx.ID, tx.State, tx.StateHash, tx.PKCEChallenge, tx.Callback, tx.InstanceID, tx.Expires, tx.Provider, tx.Nonce, tx.ProviderVerifier)
	return err
}

func (p *Postgres) Transaction(ctx context.Context, id string) (Transaction, error) {
	var tx Transaction
	var consumed *time.Time
	err := p.pool.QueryRow(ctx, `SELECT id, state, state_hash, pkce_challenge, callback, instance_id, expires_at, consumed_at, provider, nonce, provider_code_verifier
		FROM login_transactions WHERE id = $1`, id).Scan(
		&tx.ID, &tx.State, &tx.StateHash, &tx.PKCEChallenge, &tx.Callback, &tx.InstanceID, &tx.Expires, &consumed, &tx.Provider, &tx.Nonce, &tx.ProviderVerifier)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, ErrNotFound
	}
	tx.Consumed = consumed != nil
	return tx, err
}

func (p *Postgres) ConsumeTransaction(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE login_transactions SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConsumed
	}
	return nil
}

func (p *Postgres) UpsertIdentity(ctx context.Context, in Identity, display string) (string, error) {
	var userID string
	err := p.pool.QueryRow(ctx, `SELECT user_id FROM external_identities WHERE issuer = $1 AND provider_subject = $2`, in.Issuer, in.Subject).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	userID = newID()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, display_name) VALUES ($1, $2)`, userID, display); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO external_identities (id, user_id, provider, issuer, provider_subject, verified_email)
		VALUES ($1,$2,$3,$4,$5,$6)`, newID(), userID, in.Provider, in.Issuer, in.Subject, in.Email); err != nil {
		return "", err
	}
	return userID, tx.Commit(ctx)
}

func (p *Postgres) PutGrant(ctx context.Context, g Grant) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO login_grants (code_hash, user_id, pkce_challenge, state_hash, expires_at)
		VALUES ($1,$2,$3,$4,$5)`, g.CodeHash, g.UserID, g.PKCEChallenge, g.StateHash, g.Expires)
	return err
}

func (p *Postgres) ConsumeGrant(ctx context.Context, codeHash, pkce, stateHash string, now time.Time) (string, error) {
	var userID, gotPKCE, gotState string
	var exp time.Time
	var used *time.Time
	err := p.pool.QueryRow(ctx, `SELECT user_id, pkce_challenge, state_hash, expires_at, used_at FROM login_grants WHERE code_hash = $1`, codeHash).
		Scan(&userID, &gotPKCE, &gotState, &exp, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if used != nil {
		return "", ErrConsumed
	}
	if now.After(exp) || gotPKCE != pkce || gotState != stateHash {
		if now.After(exp) {
			return "", ErrExpired
		}
		return "", ErrMismatch
	}
	tag, err := p.pool.Exec(ctx, `UPDATE login_grants SET used_at = $2 WHERE code_hash = $1 AND used_at IS NULL`, codeHash, now)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrConsumed
	}
	return userID, nil
}

func (p *Postgres) PutSession(ctx context.Context, s Session) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO login_sessions
		(id, user_id, device_id, family_id, access_hash, refresh_hash, access_expires_at, refresh_expires_at, family_revoked)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		s.ID, s.UserID, s.DeviceID, s.FamilyID, s.AccessHash, s.RefreshHash, s.AccessExp, s.RefreshExp, s.FamilyRevoked)
	return err
}

func (p *Postgres) scanSession(ctx context.Context, where string, arg string) (Session, error) {
	var s Session
	var revoked *time.Time
	err := p.pool.QueryRow(ctx, `SELECT id, user_id, device_id, family_id, access_hash, refresh_hash, access_expires_at, refresh_expires_at, revoked_at, family_revoked
		FROM login_sessions WHERE `+where, arg).Scan(
		&s.ID, &s.UserID, &s.DeviceID, &s.FamilyID, &s.AccessHash, &s.RefreshHash, &s.AccessExp, &s.RefreshExp, &revoked, &s.FamilyRevoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	s.Revoked = revoked != nil
	return s, err
}

func (p *Postgres) SessionByAccess(ctx context.Context, accessHash string) (Session, error) {
	s, err := p.scanSession(ctx, `access_hash = $1`, accessHash)
	if err != nil {
		return Session{}, err
	}
	if s.Revoked || s.FamilyRevoked {
		return Session{}, ErrRevoked
	}
	return s, nil
}

func (p *Postgres) SessionByRefresh(ctx context.Context, refreshHash string) (Session, error) {
	return p.scanSession(ctx, `refresh_hash = $1`, refreshHash)
}

func (p *Postgres) RotateRefresh(ctx context.Context, oldRefreshHash string, next Session) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	old, err := p.SessionByRefresh(ctx, oldRefreshHash)
	if err != nil {
		return err
	}
	if old.Revoked || old.FamilyRevoked {
		_, _ = tx.Exec(ctx, `UPDATE login_sessions SET revoked_at = now(), family_revoked = TRUE WHERE family_id = $1`, old.FamilyID)
		_ = tx.Commit(ctx)
		return ErrRevoked
	}
	if time.Now().After(old.RefreshExp) {
		return ErrExpired
	}
	if _, err := tx.Exec(ctx, `UPDATE login_sessions SET revoked_at = now() WHERE id = $1`, old.ID); err != nil {
		return err
	}
	next.FamilyID = old.FamilyID
	next.UserID = old.UserID
	if _, err := tx.Exec(ctx, `INSERT INTO login_sessions
		(id, user_id, device_id, family_id, access_hash, refresh_hash, access_expires_at, refresh_expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		next.ID, next.UserID, next.DeviceID, next.FamilyID, next.AccessHash, next.RefreshHash, next.AccessExp, next.RefreshExp); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RevokeFamily(ctx context.Context, familyID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE login_sessions SET revoked_at = now(), family_revoked = TRUE WHERE family_id = $1`, familyID)
	return err
}

func (p *Postgres) PutDevice(ctx context.Context, d Device) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO trusted_devices (id, user_id, sign_pub, box_pub, fingerprint, status)
		VALUES ($1,$2,$3,$4,$5,$6)`, d.ID, d.UserID, d.SignPub, d.BoxPub, d.Fingerprint, d.Status)
	return err
}

func (p *Postgres) Device(ctx context.Context, id string) (Device, error) {
	var d Device
	err := p.pool.QueryRow(ctx, `SELECT id, user_id, sign_pub, box_pub, fingerprint, status FROM trusted_devices WHERE id = $1`, id).
		Scan(&d.ID, &d.UserID, &d.SignPub, &d.BoxPub, &d.Fingerprint, &d.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

func (p *Postgres) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, user_id, sign_pub, box_pub, fingerprint, status FROM trusted_devices WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.SignPub, &d.BoxPub, &d.Fingerprint, &d.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *Postgres) SetDeviceStatus(ctx context.Context, id, status string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE trusted_devices SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) EnsureAccount(ctx context.Context, a Account) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO accounts (user_id, recovery_policy, generation_id, sequence)
		VALUES ($1,$2,$3,$4) ON CONFLICT (user_id) DO NOTHING`, a.UserID, a.RecoveryPolicy, a.GenerationID, a.Sequence)
	return err
}

func (p *Postgres) Account(ctx context.Context, userID string) (Account, error) {
	var a Account
	err := p.pool.QueryRow(ctx, `SELECT user_id, user_id, recovery_policy, generation_id, sequence FROM accounts WHERE user_id = $1`, userID).
		Scan(&a.ID, &a.UserID, &a.RecoveryPolicy, &a.GenerationID, &a.Sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

func (p *Postgres) SetAccountPolicy(ctx context.Context, userID, policy, generationID string, sequence uint64) error {
	a, err := p.Account(ctx, userID)
	if err != nil {
		return err
	}
	if sequence < a.Sequence || (sequence == a.Sequence && a.GenerationID != "" && generationID != a.GenerationID) {
		return ErrMismatch
	}
	_, err = p.pool.Exec(ctx, `UPDATE accounts SET recovery_policy = $2, generation_id = $3, sequence = $4 WHERE user_id = $1`,
		userID, policy, generationID, sequence)
	return err
}

func (p *Postgres) PutSlot(ctx context.Context, s Slot) error {
	if s.Revision == 0 {
		s.Revision = 1
	}
	tag, err := p.pool.Exec(ctx, `INSERT INTO account_unlock_slots
		(id, account_id, generation_id, slot_type, public_label, alg_version, public_params, encrypted_key_wrap, state, revision)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			generation_id = EXCLUDED.generation_id,
			slot_type = EXCLUDED.slot_type,
			public_label = EXCLUDED.public_label,
			alg_version = EXCLUDED.alg_version,
			public_params = EXCLUDED.public_params,
			encrypted_key_wrap = EXCLUDED.encrypted_key_wrap,
			state = EXCLUDED.state,
			revision = account_unlock_slots.revision + 1
		WHERE account_unlock_slots.account_id = EXCLUDED.account_id`,
		s.ID, s.AccountID, s.GenerationID, s.Type, s.Label, s.AlgVersion, s.PublicParams, s.Wrap, s.State, s.Revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) ListSlots(ctx context.Context, accountID string) ([]Slot, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, account_id, generation_id, slot_type, public_label, alg_version, public_params, encrypted_key_wrap, state, revision
		FROM account_unlock_slots WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Slot
	for rows.Next() {
		var s Slot
		if err := rows.Scan(&s.ID, &s.AccountID, &s.GenerationID, &s.Type, &s.Label, &s.AlgVersion, &s.PublicParams, &s.Wrap, &s.State, &s.Revision); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) Slot(ctx context.Context, id string) (Slot, error) {
	var s Slot
	err := p.pool.QueryRow(ctx, `SELECT id, account_id, generation_id, slot_type, public_label, alg_version, public_params, encrypted_key_wrap, state, revision
		FROM account_unlock_slots WHERE id = $1`, id).Scan(
		&s.ID, &s.AccountID, &s.GenerationID, &s.Type, &s.Label, &s.AlgVersion, &s.PublicParams, &s.Wrap, &s.State, &s.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Slot{}, ErrNotFound
	}
	return s, err
}

func (p *Postgres) SetSlotState(ctx context.Context, id, state string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE account_unlock_slots SET state = $2, revision = revision + 1 WHERE id = $1`, id, state)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) PutGeneration(ctx context.Context, g KeyGeneration) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO account_key_generations (id, account_id, sequence, state, manifest, signature, signer)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, g.ID, g.AccountID, g.Sequence, g.State, g.Manifest, g.Signature, g.Signer)
	return err
}

func (p *Postgres) Generations(ctx context.Context, accountID string) ([]KeyGeneration, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, account_id, sequence, state, manifest, signature, signer FROM account_key_generations WHERE account_id = $1 ORDER BY sequence`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeyGeneration
	for rows.Next() {
		var g KeyGeneration
		if err := rows.Scan(&g.ID, &g.AccountID, &g.Sequence, &g.State, &g.Manifest, &g.Signature, &g.Signer); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (p *Postgres) CreateVault(ctx context.Context, v Vault) error {
	if v.Revision == 0 {
		v.Revision = 1
	}
	var org any
	if v.OrgID != "" {
		org = v.OrgID
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO vaults (id, name_hint, owner_user_id, org_id, revision) VALUES ($1,$2,$3,$4,$5)`,
		v.ID, v.NameHint, v.OwnerID, org, v.Revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sync_events (user_id, vault_id, object_kind, object_id, object_revision) VALUES ($1,$2,'vault',$3,$4)`,
		v.OwnerID, v.ID, v.ID, v.Revision); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ListVaults(ctx context.Context, userID string) ([]Vault, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, name_hint, owner_user_id, COALESCE(org_id, ''), revision, account_key_wrap FROM vaults
		WHERE owner_user_id = $1 OR id IN (SELECT vault_id FROM vault_memberships WHERE principal_id = $1)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVaults(rows)
}

func scanVaults(rows pgx.Rows) ([]Vault, error) {
	var out []Vault
	for rows.Next() {
		var v Vault
		if err := rows.Scan(&v.ID, &v.NameHint, &v.OwnerID, &v.OrgID, &v.Revision, &v.AccountKeyWrap); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p *Postgres) Vault(ctx context.Context, id string) (Vault, error) {
	var v Vault
	err := p.pool.QueryRow(ctx, `SELECT id, name_hint, owner_user_id, COALESCE(org_id, ''), revision, account_key_wrap FROM vaults WHERE id = $1`, id).
		Scan(&v.ID, &v.NameHint, &v.OwnerID, &v.OrgID, &v.Revision, &v.AccountKeyWrap)
	if errors.Is(err, pgx.ErrNoRows) {
		return Vault{}, ErrNotFound
	}
	return v, err
}

func (p *Postgres) SetAccountKeyWrap(ctx context.Context, id string, wrap []byte) error {
	tag, err := p.pool.Exec(ctx, `UPDATE vaults SET account_key_wrap = $2 WHERE id = $1 AND org_id IS NULL`, id, wrap)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) DeleteVault(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM vaults WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) PutMembership(ctx context.Context, m Membership) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO vault_memberships (vault_id, principal_id, permission) VALUES ($1,$2,$3)
		ON CONFLICT (vault_id, principal_id) DO UPDATE SET permission = EXCLUDED.permission`, m.VaultID, m.PrincipalID, m.Permission)
	return err
}

func (p *Postgres) DeleteMembership(ctx context.Context, vaultID, principalID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM vault_memberships WHERE vault_id = $1 AND principal_id = $2`, vaultID, principalID)
	return err
}

func (p *Postgres) Membership(ctx context.Context, vaultID, principalID string) (Membership, bool) {
	var m Membership
	err := p.pool.QueryRow(ctx, `SELECT vault_id, principal_id, permission FROM vault_memberships WHERE vault_id = $1 AND principal_id = $2`, vaultID, principalID).
		Scan(&m.VaultID, &m.PrincipalID, &m.Permission)
	if err != nil {
		return Membership{}, false
	}
	return m, true
}

func (p *Postgres) PutItem(ctx context.Context, item Item, expected int64) (Item, error) {
	if item.Idempotency != "" {
		var existing Item
		err := p.pool.QueryRow(ctx, `SELECT id, vault_id, ciphertext, crypto_version, revision, deleted FROM vault_items WHERE vault_id = $1 AND idempotency_key = $2`,
			item.VaultID, item.Idempotency).Scan(&existing.ID, &existing.VaultID, &existing.Ciphertext, &existing.CryptoVersion, &existing.Revision, &existing.Deleted)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Item{}, err
		}
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback(ctx)
	if item.ID != "" {
		var rev int64
		err := tx.QueryRow(ctx, `SELECT revision FROM vault_items WHERE id = $1`, item.ID).Scan(&rev)
		if err == nil {
			if rev != expected {
				return Item{}, &ConflictError{Current: rev}
			}
			rev++
			if _, err := tx.Exec(ctx, `UPDATE vault_items SET ciphertext = $2, crypto_version = $3, revision = $4, deleted = FALSE WHERE id = $1`,
				item.ID, item.Ciphertext, item.CryptoVersion, rev); err != nil {
				return Item{}, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO item_versions (item_id, revision, ciphertext) VALUES ($1,$2,$3)`, item.ID, rev, item.Ciphertext); err != nil {
				return Item{}, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO sync_events (vault_id, object_kind, object_id, object_revision) VALUES ($1,'item',$2,$3)`, item.VaultID, item.ID, rev); err != nil {
				return Item{}, err
			}
			item.Revision = rev
			item.Deleted = false
			return item, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Item{}, err
		}
	}
	if expected != 0 {
		return Item{}, &ConflictError{Current: 0}
	}
	if item.ID == "" {
		item.ID = newID()
	}
	item.Revision = 1
	var idem any
	if item.Idempotency != "" {
		idem = item.Idempotency
	}
	if _, err := tx.Exec(ctx, `INSERT INTO vault_items (id, vault_id, ciphertext, crypto_version, revision, deleted, idempotency_key)
		VALUES ($1,$2,$3,$4,1,FALSE,$5)`, item.ID, item.VaultID, item.Ciphertext, item.CryptoVersion, idem); err != nil {
		return Item{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO item_versions (item_id, revision, ciphertext) VALUES ($1,1,$2)`, item.ID, item.Ciphertext); err != nil {
		return Item{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sync_events (vault_id, object_kind, object_id, object_revision) VALUES ($1,'item',$2,1)`, item.VaultID, item.ID); err != nil {
		return Item{}, err
	}
	return item, tx.Commit(ctx)
}

func (p *Postgres) Tombstone(ctx context.Context, id string, expected int64) (Item, error) {
	item, err := p.Item(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if item.Revision != expected {
		return Item{}, &ConflictError{Current: item.Revision}
	}
	item.Revision++
	item.Deleted = true
	_, err = p.pool.Exec(ctx, `UPDATE vault_items SET deleted = TRUE, revision = $2 WHERE id = $1`, id, item.Revision)
	return item, err
}

func (p *Postgres) Item(ctx context.Context, id string) (Item, error) {
	var item Item
	err := p.pool.QueryRow(ctx, `SELECT id, vault_id, ciphertext, crypto_version, revision, deleted, COALESCE(idempotency_key, '') FROM vault_items WHERE id = $1`, id).
		Scan(&item.ID, &item.VaultID, &item.Ciphertext, &item.CryptoVersion, &item.Revision, &item.Deleted, &item.Idempotency)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

func (p *Postgres) ListItems(ctx context.Context, vaultID string, cursor int64, limit int) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT id, vault_id, ciphertext, crypto_version, revision, deleted, COALESCE(idempotency_key, '')
		FROM vault_items WHERE vault_id = $1 AND revision > $2 ORDER BY revision LIMIT $3`, vaultID, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.VaultID, &item.Ciphertext, &item.CryptoVersion, &item.Revision, &item.Deleted, &item.Idempotency); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *Postgres) ItemHistory(ctx context.Context, id string) ([]ItemVersion, error) {
	if _, err := p.Item(ctx, id); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `SELECT item_id, revision, ciphertext FROM item_versions WHERE item_id = $1 ORDER BY revision`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemVersion
	for rows.Next() {
		var v ItemVersion
		if err := rows.Scan(&v.ItemID, &v.Revision, &v.Ciphertext); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p *Postgres) CreateOrg(ctx context.Context, o Org, ownerRole string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name, owner_id) VALUES ($1,$2,$3)`, o.ID, o.Name, o.OwnerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organization_members (org_id, user_id, role) VALUES ($1,$2,$3)`, o.ID, o.OwnerID, ownerRole); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ListOrgs(ctx context.Context, userID string) ([]Org, error) {
	rows, err := p.pool.Query(ctx, `SELECT o.id, o.name, o.owner_id FROM organizations o
		JOIN organization_members m ON m.org_id = o.id WHERE m.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (p *Postgres) OrgRole(ctx context.Context, orgID, userID string) (string, bool) {
	var role string
	err := p.pool.QueryRow(ctx, `SELECT role FROM organization_members WHERE org_id = $1 AND user_id = $2`, orgID, userID).Scan(&role)
	if err != nil {
		return "", false
	}
	return role, true
}

func (p *Postgres) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberInfo, error) {
	rows, err := p.pool.Query(ctx, `SELECT m.user_id, COALESCE(u.display_name, ''), m.role
		FROM organization_members m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgMemberInfo
	for rows.Next() {
		var info OrgMemberInfo
		if err := rows.Scan(&info.UserID, &info.DisplayName, &info.Role); err != nil {
			return nil, err
		}
		info.Devices = []OrgDevice{}
		out = append(out, info)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		devs, err := p.pool.Query(ctx, `SELECT id, box_pub FROM trusted_devices WHERE user_id = $1 AND status = 'active'`, out[i].UserID)
		if err != nil {
			return nil, err
		}
		for devs.Next() {
			var d OrgDevice
			if err := devs.Scan(&d.ID, &d.BoxPub); err != nil {
				devs.Close()
				return nil, err
			}
			out[i].Devices = append(out[i].Devices, d)
		}
		err = devs.Err()
		devs.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *Postgres) PutInvite(ctx context.Context, inv Invite) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO organization_invites (token_hash, org_id, role, expires_at) VALUES ($1,$2,$3,$4)`,
		inv.TokenHash, inv.OrgID, inv.Role, inv.Expires)
	return err
}

func (p *Postgres) Invite(ctx context.Context, tokenHash string) (Invite, error) {
	var inv Invite
	var accepted *time.Time
	err := p.pool.QueryRow(ctx, `SELECT token_hash, org_id, role, expires_at, accepted_at FROM organization_invites WHERE token_hash = $1`, tokenHash).
		Scan(&inv.TokenHash, &inv.OrgID, &inv.Role, &inv.Expires, &accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	inv.Accepted = accepted != nil
	return inv, err
}

func (p *Postgres) AcceptInvite(ctx context.Context, tokenHash, userID string) error {
	inv, err := p.Invite(ctx, tokenHash)
	if err != nil {
		return err
	}
	if inv.Accepted {
		return ErrConsumed
	}
	if time.Now().After(inv.Expires) {
		return ErrExpired
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE organization_invites SET accepted_at = now() WHERE token_hash = $1 AND accepted_at IS NULL`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConsumed
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organization_members (org_id, user_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role`, inv.OrgID, userID, inv.Role); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) PutEnvelope(ctx context.Context, e Envelope) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO vault_key_envelopes (vault_id, key_generation, recipient_device_id, ciphertext, signature)
		VALUES ($1,$2,$3,$4,$5)`, e.VaultID, e.Generation, e.RecipientDeviceID, e.Ciphertext, e.Signature)
	return err
}

func (p *Postgres) ListEnvelopes(ctx context.Context, vaultID string) ([]Envelope, error) {
	rows, err := p.pool.Query(ctx, `SELECT vault_id, key_generation, recipient_device_id, ciphertext, signature FROM vault_key_envelopes WHERE vault_id = $1`, vaultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Envelope
	for rows.Next() {
		var e Envelope
		if err := rows.Scan(&e.VaultID, &e.Generation, &e.RecipientDeviceID, &e.Ciphertext, &e.Signature); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *Postgres) DeleteEnvelopesForDevice(ctx context.Context, vaultID, deviceID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM vault_key_envelopes WHERE vault_id = $1 AND recipient_device_id = $2`, vaultID, deviceID)
	return err
}

func (p *Postgres) PutChunk(ctx context.Context, c Chunk) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO attachment_chunks (attachment_id, chunk_no, owner_user_id, sha256, body, complete)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (attachment_id, chunk_no) DO UPDATE SET sha256 = EXCLUDED.sha256, body = EXCLUDED.body`,
		c.AttachmentID, c.N, c.OwnerID, c.Sum, c.Body, c.Complete)
	return err
}

func (p *Postgres) Chunk(ctx context.Context, attachmentID string, n int) (Chunk, error) {
	var c Chunk
	err := p.pool.QueryRow(ctx, `SELECT attachment_id, chunk_no, owner_user_id, sha256, body, complete FROM attachment_chunks WHERE attachment_id = $1 AND chunk_no = $2`, attachmentID, n).
		Scan(&c.AttachmentID, &c.N, &c.OwnerID, &c.Sum, &c.Body, &c.Complete)
	if errors.Is(err, pgx.ErrNoRows) {
		return Chunk{}, ErrNotFound
	}
	return c, err
}

func (p *Postgres) CompleteAttachment(ctx context.Context, attachmentID, ownerID string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE attachment_chunks SET complete = TRUE WHERE attachment_id = $1 AND owner_user_id = $2`, attachmentID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) Changes(ctx context.Context, userID string, cursor int64, limit int) ([]Change, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT seq, user_id, vault_id, object_kind, object_id, object_revision FROM sync_events
		WHERE seq > $1 AND vault_id IN (
			SELECT id FROM vaults WHERE owner_user_id = $2
			UNION
			SELECT vault_id FROM vault_memberships WHERE principal_id = $2
		) ORDER BY seq LIMIT $3`, cursor, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.Seq, &c.UserID, &c.VaultID, &c.Kind, &c.ObjectID, &c.Revision); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p *Postgres) AddChange(ctx context.Context, c Change) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO sync_events (user_id, vault_id, object_kind, object_id, object_revision) VALUES ($1,$2,$3,$4,$5)`,
		c.UserID, c.VaultID, c.Kind, c.ObjectID, c.Revision)
	return err
}

func (p *Postgres) AddAudit(ctx context.Context, a Audit) error {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO audit_events (id, actor_id, action, target_ref, metadata_json, occurred_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		a.ID, a.ActorID, a.Action, a.Target, a.Metadata, a.At)
	return err
}

func (p *Postgres) ListAudit(ctx context.Context, userID string) ([]Audit, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, actor_id, action, target_ref, metadata_json, occurred_at FROM audit_events
		WHERE actor_id = $1 OR $1 = '' ORDER BY occurred_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Audit
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.ActorID, &a.Action, &a.Target, &a.Metadata, &a.At); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

var _ Store = (*Postgres)(nil)
var _ Store = (*Memory)(nil)
