<script lang="ts">
  import { onMount } from "svelte";

  let tx = $state("");

  onMount(() => {
    tx = new URLSearchParams(window.location.search).get("tx") ?? "";
  });

  function href(provider: string) {
    const q = new URLSearchParams({ provider });
    if (tx) q.set("tx", tx);
    return "/auth/desktop/continue?" + q.toString();
  }
</script>

<h1 class="text-2xl font-semibold">Sign in</h1>
<p class="mt-3">Choose an identity provider. You will still need a separate vault unlock on the desktop.</p>
<div class="mt-6 flex flex-col gap-3">
  <a class="btn" href={href("github")}>GitHub</a>
  <a class="btn" href={href("google")}>Google account</a>
  <a class="btn" href={href("microsoft")}>Microsoft</a>
</div>
<p class="mt-6 text-sm opacity-80">Signing in does not unlock the vault. If no provider is configured, the server shows a setup error instead of these links.</p>
