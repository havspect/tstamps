<script lang="ts">
  // The server injects this meta tag only when --legal-dir is configured, so
  // an instance without legal pages never links to a 404.
  const configured =
    typeof document !== 'undefined' && document.querySelector('meta[name="tstamps-legal"]') !== null
</script>

{#if configured}
  <a href="/imprint">Impressum</a>
  <a href="/privacy">Datenschutz</a>
{/if}
