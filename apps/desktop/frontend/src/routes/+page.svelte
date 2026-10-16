<script lang="ts">
  import { onMount } from "svelte";
  type Summary = {
    id: string;
    kind: string;
    title: string;
    username: string;
  };
  type Item = Summary & {
    password: string;
    url: string;
    notes: string;
    totp: string;
    fields?: Record<string, string>;
  };
  type Settings = {
    theme: string;
    idleLockMinutes: number;
    clipboardClearSeconds: number;
    closeToTray: boolean;
  };
  type VaultChoice = {
    id: string;
    name: string;
    kind: string;
    orgId: string;
    owner: boolean;
    waiting: boolean;
    sync: string;
  };
  type TeamMember = {
    id: string;
    displayName: string;
    role: string;
    devices: { id: string; boxPub: string }[];
  };
  type Status = {
    unlocked: boolean;
    bootstrapped: boolean;
    passphrase: boolean;
    recovery: boolean;
    securityKeys: number;
    profiles: { name: string; url: string }[];
    settings: Settings;
    signedIn: boolean;
    account: string;
    sync: string;
    vaults: VaultChoice[];
    activeVault: string;
  };
  type Doctor = {
    os: string;
    fido2Tag: boolean;
    pivTag: boolean;
    checks: { name: string; ok: boolean; detail: string }[];
  };
  type Device = { path: string; product: string; hmac: boolean };

  const kinds = [
    { id: "login", label: "Login" },
    { id: "note", label: "Secure note" },
    { id: "card", label: "Card" },
    { id: "identity", label: "Identity" },
    { id: "token", label: "API token" },
    { id: "wifi", label: "Wi-Fi" },
  ] as const;

  const extras: Record<string, { key: string; label: string; secret?: boolean }[]> = {
    card: [
      { key: "cardholder", label: "Name on card" },
      { key: "number", label: "Number", secret: true },
      { key: "expiry", label: "Expiry" },
      { key: "cvv", label: "CVV", secret: true },
    ],
    identity: [
      { key: "name", label: "Name" },
      { key: "email", label: "Email" },
      { key: "phone", label: "Phone" },
      { key: "address", label: "Address" },
    ],
    token: [
      { key: "token", label: "Token", secret: true },
      { key: "host", label: "Host" },
    ],
    wifi: [
      { key: "ssid", label: "Network" },
      { key: "password", label: "Password", secret: true },
      { key: "security", label: "Security" },
    ],
  };

  const emptySettings: Settings = {
    theme: "dark",
    idleLockMinutes: 5,
    clipboardClearSeconds: 30,
    closeToTray: true,
  };

  let section = $state<"vault" | "hardware" | "settings" | "team">("vault");
  let status = $state<Status>({
    unlocked: false,
    bootstrapped: false,
    passphrase: false,
    recovery: false,
    securityKeys: 0,
    profiles: [],
    settings: emptySettings,
    signedIn: false,
    account: "",
    sync: "",
    vaults: [],
    activeVault: "",
  });
  let orgName = $state("");
  let teamName = $state("");
  let inviteToken = $state("");
  let acceptToken = $state("");
  let members = $state<TeamMember[]>([]);
  let memberOrg = $state("");
  let doctor = $state<Doctor>({ os: "", fido2Tag: false, pivTag: false, checks: [] });
  let devices = $state<Device[]>([]);
  let items = $state<Summary[]>([]);
  let query = $state("");
  let kindFilter = $state("login");
  let selected = $state<Item | null>(null);
  let editing = $state(false);
  let creating = $state(false);
  let revealed = $state<Record<string, boolean>>({});
  let totpCode = $state("");
  let totpSeconds = $state(0);
  let message = $state("");
  let error = $state("");
  let recoveryKey = $state("");
  let passphrase = $state("");
  let recoveryInput = $state("");
  let setupPass = $state("");
  let fidoPin = $state("");
  let fidoPath = $state("");
  let serverURL = $state("");
  let settingsDraft = $state<Settings>({ ...emptySettings });
  let busy = $state(false);
  let loaded = false;
  let signInURL = $state("");

  function blank(kind = kindFilter): Item {
    return {
      id: "",
      kind,
      title: "",
      username: "",
      password: "",
      url: "",
      notes: "",
      totp: "",
      fields: {},
    };
  }

  let draft = $state<Item>(blank());

  async function api<T>(path: string, init: RequestInit | undefined = undefined): Promise<T> {
    const res = await fetch(path, {
      ...init,
      headers: { "content-type": "application/json", ...(init && init.headers ? init.headers : {}) },
    });
    if (!res.ok) {
      throw new Error((await res.text()).trim() || res.statusText);
    }
    return res.json() as Promise<T>;
  }

  function syncLabel(state: string) {
    if (state === "synced") return "Synced";
    if (state === "offline") return "Offline. Changes stay on this device until the server is reachable.";
    if (state === "error") return "The server is reachable, but sync failed. Try again after unlocking.";
    if (state === "conflict") return "Sync conflict. Both copies were kept.";
    if (state === "waiting") return "Waiting for a vault key from the owner.";
    return "Not syncing";
  }

  async function selectVault(id: string) {
    section = "vault";
    selected = null;
    creating = false;
    editing = false;
    await api("/api/local/vaults/active", { method: "POST", body: JSON.stringify({ id }) });
    await refreshStatus();
    if (status.unlocked) await refreshItems();
  }

  async function createTeam() {
    busy = true;
    error = "";
    try {
      await api("/api/local/teams", { method: "POST", body: JSON.stringify({ org: orgName, vault: teamName }) });
      orgName = "";
      teamName = "";
      message = "Team vault created. Invite someone with a token, then grant their device.";
      await refreshStatus();
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not create the team.";
    } finally {
      busy = false;
    }
  }

  async function loadMembers(orgId: string) {
    memberOrg = orgId;
    members = (await api<{ members: TeamMember[] }>(`/api/local/teams/${orgId}/members`)).members ?? [];
  }

  async function inviteMember(orgId: string) {
    busy = true;
    error = "";
    try {
      const out = await api<{ token: string }>("/api/local/teams/invite", { method: "POST", body: JSON.stringify({ orgId }) });
      inviteToken = out.token;
      message = "Give this token to someone who is already signed in on this server. It works once.";
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not create an invite.";
    } finally {
      busy = false;
    }
  }

  async function acceptInvite() {
    busy = true;
    error = "";
    try {
      await api("/api/local/teams/accept", { method: "POST", body: JSON.stringify({ token: acceptToken }) });
      acceptToken = "";
      message = "Invite accepted. The vault appears here after the owner grants this device a key.";
      await refreshStatus();
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not accept the invite.";
    } finally {
      busy = false;
    }
  }

  async function grantDevice(vaultId: string, principalId: string, deviceId: string, boxPub: string) {
    busy = true;
    error = "";
    try {
      await api("/api/local/teams/grant", {
        method: "POST",
        body: JSON.stringify({ vaultId, principalId, deviceId, boxPub }),
      });
      message = "Shared. Their vault unlocks on the next sync.";
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not share the vault.";
    } finally {
      busy = false;
    }
  }

  async function removeMember(vaultId: string, orgId: string, principalId: string) {
    busy = true;
    error = "";
    try {
      await api("/api/local/teams/remove", { method: "POST", body: JSON.stringify({ vaultId, orgId, principalId }) });
      message = "Removed. The vault key was changed for the people who remain.";
      await loadMembers(orgId);
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not remove that member.";
    } finally {
      busy = false;
    }
  }

  function applyTheme(theme: string) {
    document.documentElement.dataset.theme = theme === "light" ? "corporate" : "night";
  }

  async function refreshStatus() {
    const next = await api<Status>("/api/local/status");
    const was = status.unlocked;
    status = next;
    if (!loaded) {
      settingsDraft = { ...next.settings };
      serverURL = next.profiles[0]?.url ?? serverURL;
      loaded = true;
    }
    applyTheme(next.settings.theme);
    if (next.unlocked && message.startsWith("Signed in. The vault stays locked")) message = "";
    if (was && !next.unlocked) {
      items = [];
      selected = null;
      creating = false;
      editing = false;
      totpCode = "";
    }
    return next;
  }

  async function refreshItems() {
    if (!status.unlocked) {
      items = [];
      return;
    }
    const body = await api<{ items: Summary[] }>(`/api/local/items?q=${encodeURIComponent(query)}`);
    items = body.items ?? [];
  }

  async function loadDoctor() {
    doctor = await api<Doctor>("/api/local/doctor");
    if (!doctor.fido2Tag) {
      devices = [];
      return;
    }
    try {
      const body = await api<{ devices: Device[] }>("/api/local/fido2/devices");
      devices = (body.devices ?? []).filter((d) => d.hmac);
    } catch (err) {
      devices = [];
      error = err instanceof Error ? err.message : "Could not list security keys.";
    }
  }

  function visibleItems() {
    return items.filter((item) => item.kind === kindFilter);
  }

  async function openItem(id: string) {
    error = "";
    creating = false;
    editing = false;
    revealed = {};
    totpCode = "";
    selected = await api<Item>(`/api/local/items/${id}`);
    draft = {
      ...selected,
      fields: { ...(selected.fields ?? {}) },
    };
    if (selected.totp) {
      await refreshTotp();
    }
  }

  function startNew() {
    selected = null;
    creating = true;
    editing = true;
    revealed = {};
    totpCode = "";
    draft = blank(kindFilter);
    error = "";
  }

  async function saveItem() {
    busy = true;
    error = "";
    try {
      const payload = { ...draft, kind: draft.kind || kindFilter, fields: draft.fields ?? {} };
      if (creating || !payload.id) {
        await api("/api/local/items", { method: "POST", body: JSON.stringify(payload) });
      } else {
        await api(`/api/local/items/${payload.id}`, { method: "PUT", body: JSON.stringify(payload) });
      }
      creating = false;
      editing = false;
      await refreshItems();
      const match = items.find((item) => item.title === payload.title && item.kind === payload.kind);
      if (match) {
        await openItem(match.id);
      }
      const next = await refreshStatus();
      message =
        next.sync === "synced"
          ? "Item saved and synced."
          : next.sync === "offline"
            ? "Item saved on this device. Sync will retry when the server is reachable."
            : next.sync === "conflict"
              ? "Item saved. Sync kept both copies of a conflicting item."
              : "Item saved on this device.";
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not save the item.";
    } finally {
      busy = false;
    }
  }

  async function deleteItem() {
    if (!selected?.id) return;
    if (!confirm(`Delete ${selected.title || "this item"}?`)) return;
    busy = true;
    error = "";
    try {
      await api(`/api/local/items/${selected.id}`, { method: "DELETE" });
      selected = null;
      draft = blank();
      await refreshItems();
      message = "Item deleted.";
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not delete the item.";
    } finally {
      busy = false;
    }
  }

  async function refreshTotp() {
    if (!selected?.id || !selected.totp) return;
    const body = await api<{ code: string; seconds: number }>("/api/local/totp", {
      method: "POST",
      body: JSON.stringify({ id: selected.id }),
    });
    totpCode = body.code;
    totpSeconds = body.seconds;
  }

  let clearTimer: ReturnType<typeof setTimeout> | undefined;
  async function copyValue(value: string) {
    error = "";
    try {
      await navigator.clipboard.writeText(value);
      message = "Copied.";
      const seconds = status.settings.clipboardClearSeconds;
      if (seconds > 0) {
        clearTimeout(clearTimer);
        clearTimer = setTimeout(() => {
          void navigator.clipboard.writeText("");
          message = "Clipboard cleared.";
        }, seconds * 1000);
      }
    } catch {
      error = "The clipboard is not available in this window.";
    }
  }

  async function generatePassword() {
    const body = await api<{ password: string }>("/api/local/generate", {
      method: "POST",
      body: JSON.stringify({ length: 20 }),
    });
    draft.password = body.password;
    revealed.password = true;
  }

  async function setup(policy: string) {
    error = "";
    recoveryKey = "";
    if (policy === "passphrase" && setupPass.trim().length < 8) {
      error = "Enter a passphrase of at least 8 characters, then choose Passphrase and recovery key.";
      return;
    }
    busy = true;
    try {
      const body = await api<{ recoveryKey: string }>("/api/local/setup", {
        method: "POST",
        body: JSON.stringify({ policy, passphrase: setupPass }),
      });
      recoveryKey = body.recoveryKey;
      message = "Finish both parts in this window. Write the recovery key down, then use the vault.";
      await refreshStatus();
      await refreshItems();
    } catch (err) {
      error = err instanceof Error ? err.message : "Setup failed.";
    } finally {
      busy = false;
    }
  }

  async function unlockPass() {
    await unlock({ passphrase });
  }

  async function unlockRecovery() {
    await unlock({ recoveryKey: recoveryInput });
  }

  async function unlock(body: Record<string, string>) {
    busy = true;
    error = "";
    try {
      await api("/api/local/unlock", { method: "POST", body: JSON.stringify(body) });
      passphrase = "";
      recoveryInput = "";
      await refreshStatus();
      await refreshItems();
      section = "vault";
    } catch (err) {
      error = err instanceof Error ? err.message : "Unlock failed.";
    } finally {
      busy = false;
    }
  }

  async function unlockFido() {
    busy = true;
    error = "";
    try {
      await api("/api/local/fido2/unlock", {
        method: "POST",
        body: JSON.stringify({ pin: fidoPin, path: fidoPath }),
      });
      fidoPin = "";
      await refreshStatus();
      await refreshItems();
      section = "vault";
    } catch (err) {
      error = err instanceof Error ? err.message : "The security key did not unlock the vault.";
    } finally {
      busy = false;
    }
  }

  async function enrollFido() {
    busy = true;
    error = "";
    try {
      await api("/api/local/fido2/enroll", {
        method: "POST",
        body: JSON.stringify({ pin: fidoPin, path: fidoPath, policy: "hardware" }),
      });
      fidoPin = "";
      message = "Security key enrolled in this window.";
      await refreshStatus();
      await loadDoctor();
    } catch (err) {
      error = err instanceof Error ? err.message : "Enrollment failed.";
    } finally {
      busy = false;
    }
  }

  async function lock() {
    await api("/api/local/lock", { method: "POST" });
    await refreshStatus();
  }

  async function signIn() {
    error = "";
    signInURL = "";
    try {
      const body = await api<{ url: string; browser: boolean }>("/api/local/sign-in", {
        method: "POST",
        body: "{}",
      });
      signInURL = body.url;
      message = body.browser
        ? "Finish sign-in in your browser. Signing in does not unlock the vault."
        : "The browser did not open. Use the sign-in link below. Signing in does not unlock the vault.";
      const started = Date.now();
      const timer = setInterval(() => {
        void refreshStatus().then((next) => {
          if (next.signedIn || Date.now() - started > 5 * 60 * 1000) {
            clearInterval(timer);
            if (next.signedIn) {
              message = next.unlocked ? "Signed in." : "Signed in. The vault stays locked until you unlock it in this window.";
              signInURL = "";
            }
          }
        });
      }, 2000);
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not start sign-in.";
    }
  }

  async function signOut() {
    error = "";
    await api("/api/local/sign-out", { method: "POST", body: "{}" });
    message = "Signed out of the server. The vault lock is unchanged.";
    await refreshStatus();
  }

  async function saveSettings() {
    busy = true;
    error = "";
    try {
      const saved = await api<Settings>("/api/local/settings", {
        method: "POST",
        body: JSON.stringify({
          ...settingsDraft,
          idleLockMinutes: Number(settingsDraft.idleLockMinutes),
          clipboardClearSeconds: Number(settingsDraft.clipboardClearSeconds),
        }),
      });
      settingsDraft = saved;
      applyTheme(saved.theme);
      await api("/api/local/server", { method: "POST", body: JSON.stringify({ url: serverURL }) });
      const next = await refreshStatus();
      message = `Settings saved on this device. ${syncLabel(next.sync)}.`;
    } catch (err) {
      error = err instanceof Error ? err.message : "Could not save settings.";
    } finally {
      busy = false;
    }
  }

  function fieldValue(key: string) {
    return draft.fields?.[key] ?? "";
  }

  function setField(key: string, value: string) {
    draft.fields = { ...(draft.fields ?? {}), [key]: value };
  }

  $effect(() => {
    if (totpSeconds <= 0 || !selected?.totp) return;
    const timer = setTimeout(() => {
      totpSeconds -= 1;
      if (totpSeconds <= 0) void refreshTotp();
    }, 1000);
    return () => clearTimeout(timer);
  });

  let activityTimer: ReturnType<typeof setTimeout> | undefined;
  function noteActivity() {
    clearTimeout(activityTimer);
    activityTimer = setTimeout(() => {
      void fetch("/api/local/activity", { method: "POST" });
    }, 800);
  }

  $effect(() => {
    applyTheme(section === "settings" ? settingsDraft.theme : status.settings.theme);
  });

  onMount(() => {
    let stop = false;
    void (async () => {
      try {
        const next = await refreshStatus();
        await loadDoctor();
        if (!stop && next.unlocked) await refreshItems();
      } catch (err) {
        error = err instanceof Error ? err.message : "The local vault API is not available.";
      }
    })();
    const poll = setInterval(() => {
      void refreshStatus()
        .then((next) => {
          if (next.unlocked) return refreshItems();
        })
        .catch(() => undefined);
    }, 10000);
    window.addEventListener("pointerdown", noteActivity);
    window.addEventListener("keydown", noteActivity);
    return () => {
      stop = true;
      clearInterval(poll);
      window.removeEventListener("pointerdown", noteActivity);
      window.removeEventListener("keydown", noteActivity);
    };
  });
</script>

<div class="grid h-full min-h-0 grid-cols-[15rem_minmax(0,1fr)]">
  <aside class="flex min-h-0 flex-col gap-2 overflow-hidden border-r border-base-300 bg-base-100 p-4">
    <div class="mb-2 flex shrink-0 items-center gap-3">
      <img src="/logo.png" alt="" class="h-9 w-9 rounded-lg" />
      <div>
        <p class="font-semibold leading-tight">withsecrets</p>
        <p class="text-xs opacity-70">{status.unlocked ? "Unlocked" : "Locked"}</p>
        <p class="text-xs opacity-70">{syncLabel(status.sync)}</p>
      </div>
    </div>
    <nav class="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
      <button class="btn btn-ghost shrink-0 justify-start" class:btn-active={section === "vault" && status.activeVault === ""} onclick={() => selectVault("")}>Personal</button>
      {#each status.vaults.filter((vault) => vault.kind === "team") as vault}
        <button class="btn btn-ghost shrink-0 justify-start" class:btn-active={section === "vault" && status.activeVault === vault.id} onclick={() => selectVault(vault.id)}>{vault.name}</button>
      {/each}
      <button class="btn btn-ghost shrink-0 justify-start" class:btn-active={section === "team"} onclick={() => (section = "team")}>Sharing</button>
      <button class="btn btn-ghost shrink-0 justify-start" class:btn-active={section === "hardware"} onclick={() => (section = "hardware")}>Hardware keys</button>
      <button class="btn btn-ghost shrink-0 justify-start" class:btn-active={section === "settings"} onclick={() => (section = "settings")}>Settings</button>
    </nav>
    <div class="flex shrink-0 flex-col gap-2">
      {#if status.signedIn}
        <p class="truncate text-xs opacity-70" title={status.account}>Signed in</p>
        <button class="btn btn-ghost btn-sm" onclick={signOut}>Sign out</button>
      {:else}
        <button class="btn btn-primary btn-sm" onclick={signIn}>Sign in</button>
      {/if}
      {#if status.unlocked}
        <button class="btn btn-outline w-full" onclick={lock}>Lock</button>
      {/if}
    </div>
  </aside>

  <section class="flex min-h-0 min-w-0 flex-col overflow-hidden">
    {#if recoveryKey}
      <div class="alert alert-warning m-4 rounded-lg">
        <div>
          <p>Write this recovery key down. It is shown once in this window.</p>
          <p class="mt-2 break-all font-mono text-lg">{recoveryKey}</p>
          <button class="btn btn-sm mt-3" type="button" onclick={() => (recoveryKey = "")}>I wrote it down</button>
        </div>
      </div>
    {/if}
    {#if error}
      <div class="alert alert-error m-4 rounded-lg">{error}</div>
    {/if}
    {#if message}
      <div class="alert alert-info m-4 rounded-lg">{message}</div>
    {/if}

    {#if section === "settings"}
      <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto max-w-xl p-8">
        <h1 class="text-2xl font-semibold">Settings</h1>
        <p class="mt-2 text-sm opacity-70">These choices stay on this computer. Vault items sync while you are signed in and the vault is unlocked.</p>
        <form class="mt-6 flex flex-col gap-4" onsubmit={(e) => { e.preventDefault(); void saveSettings(); }}>
          <label class="form-control">
            <span class="label-text mb-1">Default server</span>
            <input class="input w-full" bind:value={serverURL} placeholder="https://vault.example.com" />
          </label>
          <label class="form-control">
            <span class="label-text mb-1">Theme</span>
            <select class="select w-full" bind:value={settingsDraft.theme}>
              <option value="dark">Dark</option>
              <option value="light">Light</option>
            </select>
          </label>
          <label class="form-control">
            <span class="label-text mb-1">Lock after idle minutes (0 disables)</span>
            <input class="input w-full" type="number" min="0" max="240" bind:value={settingsDraft.idleLockMinutes} />
          </label>
          <label class="form-control">
            <span class="label-text mb-1">Clear clipboard after copy, in seconds (0 keeps it)</span>
            <input class="input w-full" type="number" min="0" max="300" bind:value={settingsDraft.clipboardClearSeconds} />
          </label>
          <label class="flex items-center gap-3">
            <input class="checkbox" type="checkbox" bind:checked={settingsDraft.closeToTray} />
            <span>Close the window to the tray</span>
          </label>
          <p class="text-sm opacity-70">The tray setting is used by the desktop app. This browser preview has no tray.</p>
          <button class="btn btn-primary" type="submit" disabled={busy}>Save settings</button>
        </form>
        <div class="mt-8 rounded-lg bg-base-100 p-4">
          <h2 class="font-semibold">Server sign-in</h2>
          <p class="mt-2 text-sm">Sign in to the server above with GitHub, Google, or Microsoft. That does not unlock the vault on this computer.</p>
          {#if status.signedIn}
            <p class="mt-3 break-all text-sm">Signed in as {status.account}</p>
            <button class="btn btn-sm mt-3" type="button" onclick={signOut}>Sign out</button>
          {:else}
            <button class="btn btn-primary btn-sm mt-3" type="button" onclick={signIn}>Sign in</button>
          {/if}
          {#if signInURL}
            <p class="mt-3 break-all text-sm"><a class="link" href={signInURL}>Open the sign-in page</a></p>
          {/if}
        </div>
      </div>
      </div>
    {:else if section === "team"}
      <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto max-w-2xl p-8">
        <h1 class="text-2xl font-semibold">Sharing</h1>
        {#if !status.signedIn || !status.unlocked}
          <p class="mt-2">Sign in and unlock this window before sharing a team vault.</p>
        {:else}
          <p class="mt-2 text-sm opacity-70">A team vault is separate from your personal vault. Someone on this server pastes an invite token, then you grant their device.</p>
          <form class="mt-6 flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void createTeam(); }}>
            <label class="form-control">
              <span class="label-text mb-1">Organization</span>
              <input class="input w-full" bind:value={orgName} placeholder="Acme" />
            </label>
            <label class="form-control">
              <span class="label-text mb-1">Vault name</span>
              <input class="input w-full" bind:value={teamName} placeholder="Engineering" />
            </label>
            <button class="btn btn-primary" type="submit" disabled={busy}>Create team vault</button>
          </form>
          <form class="mt-8 flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void acceptInvite(); }}>
            <label class="form-control">
              <span class="label-text mb-1">Invite token</span>
              <input class="input w-full" bind:value={acceptToken} />
            </label>
            <button class="btn" type="submit" disabled={busy}>Accept invite</button>
          </form>
          {#if inviteToken}
            <p class="mt-4 break-all rounded-lg bg-base-100 p-3 font-mono text-sm">{inviteToken}</p>
          {/if}
          {#each status.vaults.filter((vault) => vault.kind === "team") as vault}
            <div class="mt-6 rounded-lg bg-base-100 p-4">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <h2 class="font-semibold">{vault.name}</h2>
                  <p class="text-xs opacity-70">{syncLabel(vault.sync)}</p>
                </div>
                {#if vault.owner}
                  <button class="btn btn-sm" type="button" disabled={busy} onclick={() => inviteMember(vault.orgId)}>Invite</button>
                  <button class="btn btn-sm btn-ghost" type="button" disabled={busy} onclick={() => loadMembers(vault.orgId)}>Members</button>
                {/if}
              </div>
              {#if vault.owner && memberOrg === vault.orgId}
                <ul class="mt-4 flex flex-col gap-3">
                  {#each members as member}
                    <li class="rounded-lg border border-base-300 p-3">
                      <p class="text-sm">{member.displayName || member.id}</p>
                      <p class="text-xs opacity-70">{member.role}</p>
                      {#each member.devices as device}
                        <button class="btn btn-xs mt-2" type="button" disabled={busy || member.id === status.account} onclick={() => grantDevice(vault.id, member.id, device.id, device.boxPub)}>Grant edit</button>
                      {/each}
                      {#if member.devices.length === 0}
                        <p class="mt-2 text-xs opacity-70">No device enrolled yet.</p>
                      {/if}
                      {#if member.id !== status.account}
                        <button class="btn btn-xs btn-ghost mt-2" type="button" disabled={busy} onclick={() => removeMember(vault.id, vault.orgId, member.id)}>Remove</button>
                      {/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
      </div>
    {:else if section === "hardware"}
      <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto max-w-2xl p-8">
        <h1 class="text-2xl font-semibold">Hardware keys</h1>
        <p class="mt-2">Finish both parts in this window. A security key enrolled here unlocks this vault. Signing in to a server does not.</p>
        <ul class="mt-4 flex flex-col gap-2">
          {#each doctor.checks as check}
            <li class="rounded-lg bg-base-100 p-3 text-sm">{check.ok ? "Ready" : "Unavailable"} - {check.detail}</li>
          {/each}
        </ul>
        {#if doctor.fido2Tag}
          <form class="mt-6 flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void enrollFido(); }}>
            <label class="form-control">
              <span class="label-text mb-1">Key</span>
              <select class="select w-full" bind:value={fidoPath}>
                <option value="">Choose a plugged-in key</option>
                {#each devices as device}
                  <option value={device.path}>{device.product} ({device.path})</option>
                {/each}
              </select>
            </label>
            <label class="form-control">
              <span class="label-text mb-1">PIN</span>
              <input class="input w-full" type="password" bind:value={fidoPin} autocomplete="off" />
            </label>
            <div class="flex gap-2">
              <button class="btn btn-primary" type="submit" disabled={busy}>Enroll key</button>
              <button class="btn" type="button" disabled={busy || !status.bootstrapped} onclick={unlockFido}>Unlock with key</button>
              <button class="btn btn-ghost" type="button" onclick={loadDoctor}>Refresh keys</button>
            </div>
          </form>
        {/if}
      </div>
      </div>
    {:else if !status.unlocked}
      <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto flex max-w-lg flex-col gap-4 p-8">
        <h1 class="text-2xl font-semibold">Unlock</h1>
        {#if !status.bootstrapped}
          <p>Finish both parts in this window. Create a recovery key here, then unlock this same window. There is no second desktop to open.</p>
          <p class="text-sm opacity-70">Recovery key needs nothing in the passphrase box. Passphrase and recovery key uses that box, and the passphrase must be at least 8 characters.</p>
          <label class="form-control">
            <span class="label-text mb-1">Passphrase for the passphrase policy</span>
            <input class="input w-full" type="password" bind:value={setupPass} autocomplete="new-password" />
          </label>
          <div class="flex gap-2">
            <button class="btn btn-primary" disabled={busy} onclick={() => setup("recommended")}>Recovery key</button>
            <button class="btn" disabled={busy} onclick={() => setup("passphrase")}>Passphrase and recovery key</button>
          </div>
        {:else}
          {#if status.passphrase}
            <form class="flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void unlockPass(); }}>
              <label class="form-control">
                <span class="label-text mb-1">Passphrase</span>
                <input class="input w-full" type="password" bind:value={passphrase} autocomplete="current-password" />
              </label>
              <button class="btn btn-primary" type="submit" disabled={busy}>Unlock</button>
            </form>
          {/if}
          {#if status.recovery}
            <form class="flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void unlockRecovery(); }}>
              <label class="form-control">
                <span class="label-text mb-1">Recovery key</span>
                <input class="input w-full" bind:value={recoveryInput} autocomplete="off" />
              </label>
              <button class="btn" type="submit" disabled={busy}>Unlock with recovery key</button>
            </form>
          {/if}
          {#if status.securityKeys > 0}
            <p>A security key enrolled on this computer is saved here. Unlock with its PIN. Closing the app does not remove it.</p>
          {/if}
          {#if doctor.fido2Tag}
            <form class="flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void unlockFido(); }}>
              <label class="form-control">
                <span class="label-text mb-1">Security key PIN</span>
                <input class="input w-full" type="password" bind:value={fidoPin} autocomplete="off" />
              </label>
              <button class="btn" type="submit" disabled={busy}>Unlock with security key</button>
            </form>
          {/if}
        {/if}
      </div>
      </div>
    {:else}
      <div class="grid min-h-0 flex-1 grid-cols-[20rem_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] overflow-hidden">
        <div class="flex min-h-0 flex-col overflow-hidden border-r border-base-300">
          <div class="flex flex-col gap-2 border-b border-base-300 p-3">
            <input class="input w-full" placeholder="Search" bind:value={query} oninput={() => refreshItems()} />
            <div class="flex flex-wrap gap-1">
              {#each kinds as kind}
                <button class="btn btn-xs" class:btn-primary={kindFilter === kind.id} onclick={() => (kindFilter = kind.id)}>{kind.label}</button>
              {/each}
            </div>
            <button class="btn btn-primary btn-sm" onclick={startNew}>New {kinds.find((k) => k.id === kindFilter)?.label}</button>
          </div>
          <ul class="min-h-0 flex-1 overflow-auto">
            {#each visibleItems() as item}
              <li>
                <button class="w-full px-4 py-3 text-left hover:bg-base-300" class:bg-base-300={selected?.id === item.id} onclick={() => openItem(item.id)}>
                  <span class="block font-medium">{item.title || "Untitled"}</span>
                  <span class="block text-xs opacity-70">{item.username || kinds.find((k) => k.id === item.kind)?.label}</span>
                </button>
              </li>
            {:else}
              <li class="p-4 text-sm opacity-70">No items of this kind.</li>
            {/each}
          </ul>
        </div>
        <div class="min-h-0 overflow-auto p-6">
          {#if editing || creating}
            <form class="mx-auto flex max-w-xl flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void saveItem(); }}>
              <h2 class="text-xl font-semibold">{creating ? "New item" : "Edit item"}</h2>
              <label class="form-control">
                <span class="label-text mb-1">Title</span>
                <input class="input w-full" bind:value={draft.title} required />
              </label>
              {#if draft.kind === "login"}
                <label class="form-control"><span class="label-text mb-1">Username</span><input class="input w-full" bind:value={draft.username} /></label>
                <label class="form-control"><span class="label-text mb-1">Password</span><input class="input w-full" type={revealed.password ? "text" : "password"} bind:value={draft.password} /></label>
                <button class="btn btn-sm self-start" type="button" onclick={generatePassword}>Generate password</button>
                <label class="form-control"><span class="label-text mb-1">Website</span><input class="input w-full" bind:value={draft.url} /></label>
                <label class="form-control"><span class="label-text mb-1">Authenticator seed (base32)</span><input class="input w-full" bind:value={draft.totp} autocomplete="off" /></label>
              {/if}
              {#each extras[draft.kind] ?? [] as field}
                <label class="form-control">
                  <span class="label-text mb-1">{field.label}</span>
                  <input class="input w-full" type={field.secret && !revealed[field.key] ? "password" : "text"} value={fieldValue(field.key)} oninput={(e) => setField(field.key, e.currentTarget.value)} />
                </label>
              {/each}
              <label class="form-control"><span class="label-text mb-1">Notes</span><textarea class="textarea w-full" rows="4" bind:value={draft.notes}></textarea></label>
              <div class="flex gap-2">
                <button class="btn btn-primary" type="submit" disabled={busy}>Save</button>
                <button class="btn" type="button" onclick={() => { editing = false; creating = false; }}>Cancel</button>
              </div>
            </form>
          {:else if selected}
            <div class="mx-auto flex max-w-xl flex-col gap-4">
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-xs uppercase opacity-60">{kinds.find((k) => k.id === selected?.kind)?.label}</p>
                  <h2 class="text-2xl font-semibold">{selected.title}</h2>
                </div>
                <div class="flex gap-2">
                  <button class="btn btn-sm" onclick={() => (editing = true)}>Edit</button>
                  <button class="btn btn-sm btn-error btn-outline" onclick={deleteItem}>Delete</button>
                </div>
              </div>
              {#if selected.kind === "login"}
                {@render secretRow("Username", selected.username, false)}
                {@render secretRow("Password", selected.password, true, "password")}
                {@render secretRow("Website", selected.url, false)}
                {#if selected.totp}
                  <div class="rounded-lg bg-base-100 p-3">
                    <p class="text-xs opacity-70">Authenticator code</p>
                    <div class="mt-1 flex items-center justify-between">
                      <span class="font-mono text-2xl">{totpCode}</span>
                      <span class="text-sm opacity-70">{totpSeconds}s</span>
                    </div>
                    <button class="btn btn-sm mt-2" type="button" onclick={() => copyValue(totpCode)}>Copy code</button>
                  </div>
                {/if}
              {/if}
              {#each extras[selected.kind] ?? [] as field}
                {@render secretRow(field.label, selected.fields?.[field.key] ?? "", !!field.secret, field.key)}
              {/each}
              {#if selected.notes}
                {@render secretRow("Notes", selected.notes, true, "notes")}
              {/if}
            </div>
          {:else}
            <p class="opacity-70">Select an item, or create one.</p>
          {/if}
        </div>
      </div>
    {/if}
  </section>
</div>

{#snippet secretRow(label: string, value: string, secret: boolean, key = label)}
  <div class="rounded-lg bg-base-100 p-3">
    <p class="text-xs opacity-70">{label}</p>
    <div class="mt-1 flex items-center justify-between gap-3">
      <span class="break-all font-mono">{secret && !revealed[key] ? "••••••••" : value || "-"}</span>
      <span class="flex gap-1">
        {#if secret}
          <button class="btn btn-xs" type="button" onclick={() => (revealed[key] = !revealed[key])}>{revealed[key] ? "Hide" : "Reveal"}</button>
        {/if}
        <button class="btn btn-xs" type="button" onclick={() => copyValue(value)} disabled={!value}>Copy</button>
      </span>
    </div>
  </div>
{/snippet}
