<script>
  // A commit SHA rendered as a pill; click copies the full SHA.
  export let sha;
  export let label = ''; // optional display text (e.g. short hash); default = sha

  let copied = false;
  let timer;

  async function copy() {
    let ok = false;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(sha);
        ok = true;
      }
    } catch {
      ok = false;
    }
    // clipboard API needs a secure context — fall back for plain-HTTP access.
    if (!ok) ok = fallbackCopy(sha);
    if (ok) {
      copied = true;
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1200);
    }
  }

  function fallbackCopy(text) {
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch {
      return false;
    }
  }
</script>

<button
  class="font-mono text-xs px-1.5 py-0.5 rounded border transition-colors
         {copied
           ? 'border-success text-success bg-success/10'
           : 'border-border bg-canvas-inset text-fg hover:border-accent hover:text-accent'}"
  title="Click to copy {sha}"
  on:click={copy}
>{copied ? 'copied!' : label || sha}</button>
