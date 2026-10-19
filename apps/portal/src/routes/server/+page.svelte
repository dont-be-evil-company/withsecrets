<script lang="ts">
  type WellKnown = {
    product: string;
    apiVersion: string;
    instanceId: string;
    publicBaseUrl: string;
    authProviders: string[];
    cryptoVersion: number;
    features: string[];
  };

  let info = $state<WellKnown | null>(null);
  let error = $state("");

  $effect(() => {
    void (async () => {
      const res = await fetch("/.well-known/withsecrets");
      if (!res.ok) {
        error = "Server status is not available.";
        return;
      }
      info = await res.json();
    })();
  });
</script>

<h1 class="text-2xl font-semibold">Server status</h1>
<p class="mt-3">This view is read-only. OAuth client secrets stay in the server environment and are not shown or edited here.</p>
{#if error}
  <p class="mt-4 text-error">{error}</p>
{:else if info}
  <dl class="mt-6 grid gap-3 rounded-lg bg-base-100 p-4">
    <div><dt class="text-xs opacity-70">Product</dt><dd>{info.product}</dd></div>
    <div><dt class="text-xs opacity-70">API</dt><dd>{info.apiVersion}</dd></div>
    <div><dt class="text-xs opacity-70">Instance</dt><dd class="break-all font-mono">{info.instanceId}</dd></div>
    <div><dt class="text-xs opacity-70">Public URL</dt><dd class="break-all">{info.publicBaseUrl}</dd></div>
    <div>
      <dt class="text-xs opacity-70">Configured providers</dt>
      <dd>{info.authProviders?.length ? info.authProviders.join(", ") : "None. Sign-in stays closed until one is configured."}</dd>
    </div>
    <div><dt class="text-xs opacity-70">Crypto version</dt><dd>{info.cryptoVersion}</dd></div>
  </dl>
{/if}
