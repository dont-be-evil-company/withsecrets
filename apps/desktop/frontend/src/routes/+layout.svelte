<script lang="ts">
  import "../app.css";
  let { children } = $props();
</script>

<div class="h-screen overflow-hidden bg-base-200 text-base-content">{@render children()}</div>
