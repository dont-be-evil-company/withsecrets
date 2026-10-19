<script lang="ts">
  type Me = { id: string; instanceId: string };
  type Device = { ID: string; Status: string; Fingerprint: string };

  let me = $state<Me | null>(null);
  let devices = $state<Device[]>([]);
  let signedOut = $state(false);
  let error = $state("");

  async function load() {
    const meRes = await fetch("/api/v1/accounts/me", { credentials: "same-origin" });
    if (meRes.status === 401) {
      signedOut = true;
      return;
    }
    if (!meRes.ok) {
      error = "The account could not be loaded.";
      return;
    }
    me = await meRes.json();
    const devRes = await fetch("/api/v1/devices", { credentials: "same-origin" });
    if (!devRes.ok) {
      error = "Devices could not be loaded.";
      return;
    }
    const body = await devRes.json();
    devices = body.devices ?? [];
  }

  async function logout() {
    await fetch("/api/v1/auth/logout", { method: "POST", credentials: "same-origin" });
    me = null;
    devices = [];
    signedOut = true;
  }

  $effect(() => {
    void load();
  });
</script>

<h1 class="text-2xl font-semibold">Account</h1>
<p class="mt-3">This page shows the signed-in identity. It does not unlock the vault. Unlock stays on the desktop.</p>
{#if error}
  <p class="mt-4 text-error">{error}</p>
{:else if signedOut}
  <p class="mt-4">Sign in to see this account.</p>
  <a class="btn btn-primary mt-4" href="/auth/login">Sign in</a>
{:else if me}
  <dl class="mt-6 grid gap-3 rounded-lg bg-base-100 p-4">
    <div><dt class="text-xs opacity-70">Account</dt><dd class="break-all font-mono">{me.id}</dd></div>
    <div><dt class="text-xs opacity-70">Instance</dt><dd class="break-all font-mono">{me.instanceId}</dd></div>
  </dl>
  <h2 class="mt-8 text-lg font-semibold">Devices</h2>
  {#if devices.length === 0}
    <p class="mt-2 text-sm opacity-70">No devices have enrolled on this server yet.</p>
  {:else}
    <ul class="mt-3 flex flex-col gap-2">
      {#each devices as device}
        <li class="rounded-lg bg-base-100 p-3 text-sm">
          <span class="font-medium">{device.Status}</span>
          <span class="mt-1 block break-all font-mono opacity-80">{device.Fingerprint || device.ID}</span>
        </li>
      {/each}
    </ul>
  {/if}
  <p class="mt-6 text-sm">Open the desktop app to unlock items with a passphrase, recovery key, or security key.</p>
  <button class="btn mt-4" type="button" onclick={logout}>Sign out</button>
{/if}
