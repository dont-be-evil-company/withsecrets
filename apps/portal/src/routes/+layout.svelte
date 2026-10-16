<script lang="ts">
  import "../app.css";
  let { children } = $props();
</script>

<div class="min-h-screen bg-base-200 text-base-content">
  <header class="border-b border-base-300 bg-base-100">
    <nav class="mx-auto flex max-w-3xl items-center gap-4 px-6 py-4">
      <a class="font-semibold" href="/">withsecrets</a>
      <a class="link" href="/auth/login">Sign in</a>
      <a class="link" href="/account">Account</a>
      <a class="link" href="/server">Server</a>
    </nav>
  </header>
  <main class="mx-auto max-w-3xl p-8">{@render children()}</main>
</div>
