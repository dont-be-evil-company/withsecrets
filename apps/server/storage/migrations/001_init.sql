CREATE TABLE schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE server_instance (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE external_identities (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id),
    provider TEXT NOT NULL,
    issuer TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    verified_email TEXT NOT NULL DEFAULT '',
    UNIQUE (issuer, provider_subject)
);

CREATE TABLE login_transactions (
    id TEXT PRIMARY KEY,
    state TEXT NOT NULL,
    state_hash TEXT NOT NULL,
    pkce_challenge TEXT NOT NULL,
    callback TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    provider TEXT NOT NULL DEFAULT '',
    nonce TEXT NOT NULL DEFAULT '',
    provider_code_verifier TEXT NOT NULL DEFAULT ''
);

CREATE TABLE login_grants (
    code_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id),
    pkce_challenge TEXT NOT NULL,
    state_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);

CREATE TABLE login_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id),
    device_id TEXT NOT NULL DEFAULT '',
    family_id TEXT NOT NULL,
    access_hash TEXT NOT NULL UNIQUE,
    refresh_hash TEXT NOT NULL UNIQUE,
    access_expires_at TIMESTAMPTZ NOT NULL,
    refresh_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    family_revoked BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE trusted_devices (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id),
    sign_pub BYTEA NOT NULL,
    box_pub BYTEA NOT NULL,
    fingerprint TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL
);

CREATE TABLE accounts (
    user_id TEXT PRIMARY KEY REFERENCES users (id),
    recovery_policy TEXT NOT NULL,
    generation_id TEXT NOT NULL DEFAULT '',
    sequence BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE account_unlock_slots (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts (user_id),
    generation_id TEXT NOT NULL DEFAULT '',
    slot_type TEXT NOT NULL,
    public_label TEXT NOT NULL DEFAULT '',
    alg_version INT NOT NULL,
    public_params BYTEA NOT NULL DEFAULT ''::bytea,
    encrypted_key_wrap BYTEA NOT NULL,
    state TEXT NOT NULL,
    revision BIGINT NOT NULL
);

CREATE TABLE account_key_generations (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts (user_id),
    sequence BIGINT NOT NULL,
    state TEXT NOT NULL,
    manifest BYTEA NOT NULL,
    signature BYTEA NOT NULL,
    signer TEXT NOT NULL
);

CREATE TABLE organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE organization_members (
    org_id TEXT NOT NULL REFERENCES organizations (id),
    user_id TEXT NOT NULL REFERENCES users (id),
    role TEXT NOT NULL,
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE organization_invites (
    token_hash TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations (id),
    role TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ
);

CREATE TABLE vaults (
    id TEXT PRIMARY KEY,
    name_hint TEXT NOT NULL DEFAULT '',
    owner_user_id TEXT NOT NULL REFERENCES users (id),
    org_id TEXT REFERENCES organizations (id),
    revision BIGINT NOT NULL
);

CREATE TABLE vault_memberships (
    vault_id TEXT NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
    principal_id TEXT NOT NULL,
    permission TEXT NOT NULL,
    PRIMARY KEY (vault_id, principal_id)
);

CREATE TABLE vault_items (
    id TEXT PRIMARY KEY,
    vault_id TEXT NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
    ciphertext BYTEA NOT NULL,
    crypto_version INT NOT NULL,
    revision BIGINT NOT NULL,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    idempotency_key TEXT
);

CREATE UNIQUE INDEX vault_items_idempotency
    ON vault_items (vault_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

CREATE TABLE item_versions (
    item_id TEXT NOT NULL REFERENCES vault_items (id) ON DELETE CASCADE,
    revision BIGINT NOT NULL,
    ciphertext BYTEA NOT NULL,
    PRIMARY KEY (item_id, revision)
);

CREATE TABLE vault_key_envelopes (
    vault_id TEXT NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
    key_generation BIGINT NOT NULL,
    recipient_device_id TEXT NOT NULL,
    ciphertext BYTEA NOT NULL,
    signature BYTEA NOT NULL DEFAULT ''::bytea
);

CREATE TABLE attachment_chunks (
    attachment_id TEXT NOT NULL,
    chunk_no INT NOT NULL,
    owner_user_id TEXT NOT NULL REFERENCES users (id),
    sha256 BYTEA NOT NULL,
    body BYTEA NOT NULL,
    complete BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (attachment_id, chunk_no)
);

CREATE TABLE sync_events (
    seq BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    vault_id TEXT NOT NULL,
    object_kind TEXT NOT NULL,
    object_id TEXT NOT NULL,
    object_revision BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    target_ref TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
