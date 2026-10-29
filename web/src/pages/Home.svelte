<script lang="ts">
  import Link from '../lib/Link.svelte'
  import { router } from '../lib/router.svelte'
  import LegalLinks from '../lib/LegalLinks.svelte'
</script>

<svelte:head>
  <title>Tstamps — your week, in a few drags</title>
</svelte:head>

<a class="skip" href="#main">Skip to content</a>

<wa-page>
  <header slot="header" class="site-head">
    <div class="wrap head-inner wa-split">
      <a class="brand" href="/">
        <wa-icon name="stopwatch"></wa-icon>
        Tstamps
      </a>
      <nav class="header-nav wa-cluster wa-gap-m" aria-label="Homepage">
        <a href="#loop">The loop</a>
        <a href="#reports">Reports</a>
        <a href="#cli">CLI &amp; agents</a>
        <a href="#self-host">Self-host</a>
        <a href="/docs/">Docs</a>
        <Link href="/login" class="nav-signin btn ghost">Sign in</Link>
      </nav>
      <wa-button data-toggle-nav appearance="plain" class="wa-mobile-only" aria-label="Open menu">
        <wa-icon name="bars"></wa-icon>
      </wa-button>
    </div>
  </header>

  <nav slot="navigation" class="wa-stack wa-gap-s" aria-label="Mobile navigation">
    <a href="#loop" data-drawer="close">The loop</a>
    <a href="#reports" data-drawer="close">Reports</a>
    <a href="#cli" data-drawer="close">CLI &amp; agents</a>
    <a href="#self-host" data-drawer="close">Self-host</a>
    <a href="/docs/" data-drawer="close">Docs</a>
    <Link href="/login" class="btn primary" data-drawer="close">Sign in</Link>
  </nav>

  <main id="main" class="home">
    <section class="hero wrap" aria-labelledby="hero-title">
      <div class="hero-copy wa-stack wa-gap-m">
        <p class="eyebrow wa-cluster wa-gap-xs">
          <span class="dot" aria-hidden="true"></span>Self-hosted or hosted team time tracking
        </p>
        <h1 id="hero-title">Your week, <br />in a <span class="mark">few drags.</span></h1>
        <p class="lede">
          Tstamps is a team time tracker built around a timeline. Drag on a lane to log an entry, drag its
          edges to retime it, and let the report do the remembering. Run it yourself for one company, or
          several as separate workspaces: one Go binary, one SQLite database. Hosted by us is
          coming.
        </p>
        <div class="actions wa-cluster wa-gap-s">
          {#if router.hosted}
            <Link href="/signup" class="btn primary">Create a workspace <wa-icon name="arrow-right"></wa-icon></Link>
            <Link href="/login" class="btn ghost">Sign in</Link>
          {:else}
            <Link href="/login" class="btn primary">Sign in <wa-icon name="arrow-right"></wa-icon></Link>
            <a class="btn ghost" href="#loop">See the timeline <wa-icon name="arrow-down"></wa-icon></a>
          {/if}
        </div>
        <ul class="trust wa-cluster wa-gap-m" aria-label="At a glance">
          <li>AGPL-3.0</li>
          <li>No per-seat pricing</li>
          <li>One VM</li>
        </ul>
      </div>

      <figure class="hero-shot">
        <img
          src="/screenshots/timeline.webp"
          width="2200"
          height="1402"
          alt="The Tstamps timeline: one lane per project with entries for CLI, Timeline, MCP and API, a running entry and a now line."
          fetchpriority="high"
          decoding="async"
        />
        <wa-tag class="hero-tag" size="small" variant="brand">drag to log</wa-tag>
        <figcaption>
          <span>Drag a block. Drag its edges. Press <kbd>space</kbd> to start and stop.</span>
          <span class="shot-note">Real UI · sample data</span>
        </figcaption>
      </figure>
    </section>

    <section id="loop" class="section" aria-labelledby="loop-title">
      <div class="wrap">
        <div class="section-head wa-stack wa-gap-xs">
          <p class="label">01 — The loop</p>
          <h2 id="loop-title">Three moves, no timesheet.</h2>
        </div>
        <ol class="loop wa-grid wa-gap-xl" style="--min-column-size: 18rem;">
          <li>
            <span class="num">01</span>
            <h3>Drag, don't fill in.</h3>
            <p>
              Drag on a lane to log; drag an edge to retime; press space to start and stop. Overlaps are
              shown, not rejected.
            </p>
            <div class="mini-lane" aria-hidden="true">
              <span class="block green"></span>
              <span class="block rust"></span>
              <span class="block draft"></span>
            </div>
          </li>
          <li>
            <span class="num">02</span>
            <h3>Read the week.</h3>
            <p>
              Projects as rows, days as columns, biggest first. Open a project for one line per person,
              then take the CSV.
            </p>
            <div class="mini-table" aria-hidden="true">
              <span></span><span class="on"></span><span></span><span class="on"></span>
              <span class="on"></span><span></span><span class="on"></span><span class="on"></span>
              <span></span><span class="on"></span><span class="on"></span><span></span>
            </div>
          </li>
          <li>
            <span class="num">03</span>
            <h3>Log from anywhere.</h3>
            <p>
              A one-line CLI and an MCP server share the same API, so a terminal or an agent can start,
              stop and correct entries.
            </p>
            <div class="mini-code" aria-hidden="true"><span>$</span> tstamps start Timeline</div>
          </li>
        </ol>
      </div>
    </section>

    <section id="reports" class="section" aria-labelledby="reports-title">
      <div class="wrap split wa-split wa-gap-xl">
        <div class="split-copy wa-stack wa-gap-m">
          <div class="section-head wa-stack wa-gap-xs">
            <p class="label">02 — Reports</p>
            <h2 id="reports-title">The report is the meeting.</h2>
          </div>
          <p>
            Leads read every hour booked on their team's projects, by person and by day, week or month.
            There is no form to chase and no separate analytics tool to reconcile. Export the same rows as
            CSV.
          </p>
          <ul class="ticks wa-stack wa-gap-xs">
            <li><wa-icon name="check"></wa-icon>The entry's timezone decides its day</li>
            <li><wa-icon name="check"></wa-icon>Archived projects keep their hours</li>
            <li><wa-icon name="check"></wa-icon>Read-only, scoped to the teams you lead</li>
          </ul>
        </div>
        <figure class="shot">
          <img
            src="/screenshots/reports.webp"
            width="2048"
            height="1138"
            alt="The Tstamps report: projects as rows, days as columns, totals on the right and a grand total at the bottom."
            loading="lazy"
            decoding="async"
          />
          <figcaption>This week · all teams · 61h 4m</figcaption>
        </figure>
      </div>
    </section>

    <section id="cli" class="section" aria-labelledby="cli-title">
      <div class="wrap split reverse wa-split wa-gap-xl">
        <div class="split-copy wa-stack wa-gap-m">
          <div class="section-head wa-stack wa-gap-xs">
            <p class="label">03 — CLI &amp; agents</p>
            <h2 id="cli-title">Works where you do.</h2>
          </div>
          <p>
            <code>tstamps start</code> is one line in a terminal. <code>tstamps mcp</code> lets an agent
            start, stop and correct entries with the permissions of the token you gave it. Both talk to
            the same HTTP API as the timeline.
          </p>
          <p>
            The <code>tstamps</code> agent skill ships the MCP setup and the rules for logging honestly:
            it proposes entries and asks before writing them.
          </p>
          <div class="actions wa-cluster wa-gap-s">
            <a class="text-link" href="https://github.com/havspect/tstamps-skills">
              Install the agent skill <wa-icon name="arrow-up-right-from-square"></wa-icon>
            </a>
            <a class="text-link" href="https://github.com/havspect/tstamps#cli">
              Read the CLI reference <wa-icon name="arrow-up-right-from-square"></wa-icon>
            </a>
          </div>
        </div>
        <div class="terminal" role="group" aria-label="Example terminal session">
          <div class="term-bar wa-cluster" aria-hidden="true">
            <span></span><span></span><span></span>
            <span class="term-title">tstamps</span>
          </div>
          <pre><code><span class="prompt">$</span> tstamps start Timeline --note "OAuth callback"
<span class="out">started 09:41 on Timeline</span>
<span class="prompt">$</span> tstamps stop
<span class="out">stopped 11:12 (1h 31m) on Timeline</span>
<span class="prompt">$</span> tstamps report --from 2026-10-19 --to 2026-10-26 --by week
<span class="out">BUCKET    USER   TEAM      PROJECT   HOURS</span>
<span class="out">2026-W43  Ada    Platform  Timeline  10h 15m</span>
<span class="out">total                            61h 4m</span></code></pre>
        </div>
      </div>
    </section>

    <section id="self-host" class="night" aria-labelledby="host-title">
      <div class="wrap split wa-split wa-gap-xl">
        <div class="wa-stack wa-gap-m">
          <div class="section-head wa-stack wa-gap-xs">
            <p class="label">Self-hosted, or hosted by us</p>
            <h2 id="host-title">Yours to run.</h2>
          </div>
          <p>
            One <code>tstamps-server</code> process on one VM, behind Caddy on the same host. Run it for a
            single company, or host several: each workspace has its own users and
            projects, scoped inside the same SQLite database. Backups use <code>VACUUM INTO</code> and are safe while the server is running.
          </p>
          <ul class="paths wa-stack wa-gap-s">
            <li>
              <strong>One company.</strong> One workspace, one SQLite database, one admin you create from the
              CLI.
            </li>
            <li>
              <strong>Several companies.</strong> One workspace per company, each with its own users and
              logins, all in one SQLite database. Operators invite companies, or let people create their own workspace.
            </li>
            <li>
              <strong>Hosted by us.</strong> Coming soon — we run the same binary and the same shared
              database.
              <a href="https://github.com/havspect/tstamps">
                Follow along on GitHub <wa-icon name="arrow-up-right-from-square"></wa-icon>
              </a>
            </li>
          </ul>
          <ul class="facts wa-cluster wa-gap-xl">
            <li><strong>1</strong>binary</li>
            <li><strong>1</strong>SQLite database</li>
            <li><strong>0</strong>containers</li>
          </ul>
        </div>
        <div class="install wa-stack wa-gap-s">
          <p class="install-label">Get the CLI</p>
          <pre><code>curl -fsSL \
  https://github.com/havspect/tstamps/releases/latest/download/tstamps_linux_amd64.tar.gz \
  | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
tstamps login --url https://time.example.com/w/example.com</code></pre>
          <p class="install-note">
            Linux, macOS and Windows builds. The server runs under systemd behind Caddy.
          </p>
        </div>
      </div>
    </section>

    <section class="section" aria-labelledby="small-title">
      <div class="wrap">
        <div class="section-head narrow wa-stack wa-gap-xs">
          <p class="label">Scope</p>
          <h2 id="small-title">Deliberately small.</h2>
          <p>
            Tstamps is not trying to run your whole company. If you need the list on the right, choose
            something else — and that is the point.
          </p>
        </div>
        <div class="scope wa-grid wa-gap-xl" style="--min-column-size: 18rem;">
          <div>
            <h3>It does</h3>
            <ul class="ticks wa-stack wa-gap-xs">
              <li><wa-icon name="check"></wa-icon>Timeline logging, one running entry</li>
              <li><wa-icon name="check"></wa-icon>Team-lead reports and CSV</li>
              <li><wa-icon name="check"></wa-icon>CLI, MCP and a shipped agent skill</li>
              <li><wa-icon name="check"></wa-icon>Entry timezones that decide the day</li>
              <li><wa-icon name="check"></wa-icon>Soft delete and admin restore</li>
              <li><wa-icon name="check"></wa-icon>One company per workspace</li>
            </ul>
          </div>
          <div>
            <h3>It leaves out</h3>
            <ul class="ticks no wa-stack wa-gap-xs">
              <li><wa-icon name="minus"></wa-icon>Approvals, leave and holidays</li>
              <li><wa-icon name="minus"></wa-icon>Invoicing and rates</li>
              <li><wa-icon name="minus"></wa-icon>SSO and an audit log</li>
              <li><wa-icon name="minus"></wa-icon>A mobile app</li>
              <li><wa-icon name="minus"></wa-icon>Realtime co-editing</li>
            </ul>
          </div>
        </div>
        <p class="scope-note">
          What it does and does not do is in the
          <a href="https://github.com/havspect/tstamps/blob/main/docs/SPEC.md">specification</a>.
        </p>
      </div>
    </section>

    <section class="close" aria-labelledby="close-title">
      <div class="wrap close-inner wa-split wa-gap-l">
        <div>
          <h2 id="close-title">Open your workspace.</h2>
          <p>If your server is already running, the timeline is one sign-in away.</p>
        </div>
        <div class="actions wa-cluster wa-gap-s">
          {#if router.hosted}
            <Link href="/signup" class="btn primary">Create a workspace <wa-icon name="arrow-right"></wa-icon></Link>
            <Link href="/login" class="btn ghost">Sign in</Link>
          {:else}
            <Link href="/login" class="btn primary">Sign in <wa-icon name="arrow-right"></wa-icon></Link>
            <a
              class="btn ghost"
              href="https://github.com/havspect/tstamps/blob/main/docs/SPEC.md">Read the specification</a
            >
          {/if}
        </div>
      </div>
    </section>
  </main>

  <footer slot="footer" class="foot">
    <div class="wrap foot-inner wa-split wa-gap-m">
      <span class="foot-brand"><wa-icon name="stopwatch"></wa-icon>Tstamps</span>
      <nav class="wa-cluster wa-gap-m" aria-label="Project links">
        <a href="/docs/">Docs</a>
        <a href="https://github.com/havspect/tstamps">GitHub</a>
        <a href="https://github.com/havspect/tstamps/blob/main/docs/SPEC.md">Specification</a>
        <a href="https://github.com/havspect/tstamps/blob/main/GLOSSARY.md">Glossary</a>
        <a href="https://github.com/havspect/tstamps/tree/main/docs/adr">Decisions</a>
        <a href="https://github.com/havspect/tstamps/blob/main/LICENSE">AGPL-3.0</a>
        <LegalLinks />
      </nav>
      <span class="foot-note">Built with Go and Svelte.</span>
    </div>
  </footer>
</wa-page>

<style>
  main {
    padding: 0;
  }

  wa-page {
    --menu-width: auto;
  }

  :global(wa-page[view='mobile'] .header-nav) {
    display: none;
  }

  :global(wa-page[view='desktop']::part(navigation)) {
    display: none;
  }

  .wrap {
    width: min(100% - 2.5rem, 72rem);
    margin-inline: auto;
  }

  main.home {
    max-width: none;
    padding: 0;
  }

  .skip {
    position: absolute;
    left: var(--wa-space-m);
    top: -4rem;
    z-index: 20;
    padding: var(--wa-space-xs) var(--wa-space-m);
    background: var(--wa-color-surface-raised);
    border: 1px solid var(--wa-color-surface-border);
    border-radius: var(--wa-border-radius-m);
    color: var(--wa-color-text-normal);
  }

  .skip:focus {
    top: var(--wa-space-xs);
    text-decoration: none;
  }

  .site-head {
    padding: 0;
    border-bottom: 1px solid var(--wa-color-surface-border);
    background: var(--wa-color-surface-raised);
  }

  .head-inner {
    min-height: 3.5rem;
  }

  .brand,
  .foot-brand {
    display: inline-flex;
    align-items: center;
    gap: var(--wa-space-xs);
    color: var(--wa-color-text-normal);
    font-weight: 700;
    letter-spacing: -0.02em;
  }

  .brand:hover {
    text-decoration: none;
  }

  .brand wa-icon,
  .foot-brand wa-icon {
    color: var(--wa-color-brand);
    font-size: var(--wa-font-size-l);
  }

  .header-nav {
    font-size: var(--wa-font-size-s);
  }

  .header-nav a {
    color: var(--wa-color-text-quiet);
  }

  .header-nav a:hover {
    color: var(--wa-color-text-normal);
    text-decoration: none;
  }

  nav[slot='navigation'] {
    padding: var(--wa-space-m);
  }

  nav[slot='navigation'] a {
    color: var(--wa-color-text-normal);
    font-size: var(--wa-font-size-m);
    padding: var(--wa-space-xs) 0;
  }

  nav[slot='navigation'] a:hover {
    color: var(--wa-color-brand);
    text-decoration: none;
  }

  nav[slot='navigation'] :global(.btn) {
    margin-top: var(--wa-space-s);
  }

  :global(.nav-signin) {
    display: inline-flex;
    align-items: center;
    min-height: 2rem !important;
    padding: 0 var(--wa-space-s) !important;
    border: 1px solid var(--wa-color-surface-border);
    border-radius: var(--wa-border-radius-m);
    background: var(--wa-color-surface-raised);
    color: var(--wa-color-text-normal);
    font-size: var(--wa-font-size-s) !important;
    font-weight: 600;
  }

  :global(.nav-signin):hover {
    border-color: var(--wa-color-brand);
    color: var(--wa-color-brand);
    text-decoration: none;
  }

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1.02fr) minmax(0, 0.98fr);
    align-items: center;
    gap: clamp(var(--wa-space-l), 6vw, var(--wa-space-5xl));
    padding-block: clamp(var(--wa-space-2xl), 8vw, var(--wa-space-5xl)) clamp(var(--wa-space-xl), 6vw, var(--wa-space-4xl));
  }

  .eyebrow,
  .label {
    margin: 0;
    color: var(--wa-color-text-quiet);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.11em;
    line-height: 1.4;
    text-transform: uppercase;
  }

  .dot {
    position: relative;
    width: 0.4375rem;
    height: 0.4375rem;
    border-radius: var(--wa-border-radius-circle);
    background: var(--wa-color-brand);
  }

  h1 {
    margin: 0;
    font-size: clamp(2.75rem, 5.4vw, 4.4rem);
    font-weight: 650;
    letter-spacing: -0.045em;
    line-height: 0.96;
    text-wrap: balance;
  }

  h1 .mark {
    display: inline-block;
    padding: 0.02em 0.12em 0.07em;
    border-radius: var(--wa-border-radius-xs, 0.1em);
    background: var(--wa-color-brand);
    color: var(--wa-color-brand-on-loud, #fff);
    line-height: 0.95;
  }

  .lede {
    max-width: 46ch;
    margin: 0;
    font-size: clamp(1.0625rem, 1.4vw, 1.1875rem);
    line-height: 1.6;
    color: var(--wa-color-text-normal);
  }

  :global(.btn) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--wa-space-xs);
    min-height: 3rem;
    padding: 0 var(--wa-space-l);
    border: 1px solid transparent;
    border-radius: var(--wa-border-radius-m);
    font-size: var(--wa-font-size-m);
    font-weight: 600;
    transition:
      background var(--wa-transition-fast) ease,
      border-color var(--wa-transition-fast) ease,
      color var(--wa-transition-fast) ease,
      transform var(--wa-transition-fast) ease;
  }

  :global(.btn):hover {
    text-decoration: none;
    transform: translateY(-1px);
  }

  :global(.btn.primary) {
    background: var(--wa-color-brand);
    color: var(--wa-color-brand-on-loud, #fff);
  }

  :global(.btn.primary):hover {
    background: var(--wa-color-brand-40, var(--accent-strong));
  }

  :global(.btn.ghost) {
    border-color: var(--wa-color-surface-border);
    background: var(--wa-color-surface-raised);
    color: var(--wa-color-text-normal);
  }

  :global(.btn.ghost):hover {
    border-color: var(--wa-color-brand);
    color: var(--wa-color-brand);
  }

  .trust {
    margin: 0;
    padding: 0;
    color: var(--wa-color-text-quiet);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.08em;
    list-style: none;
    text-transform: uppercase;
  }

  .trust li + li::before {
    content: '·';
    margin-right: var(--wa-space-s);
    color: var(--wa-color-surface-border);
  }

  .hero-shot,
  .shot {
    position: relative;
    margin: 0;
    padding: var(--wa-space-2xs);
    border: 1px solid var(--wa-color-surface-border);
    border-radius: var(--wa-border-radius-l);
    background: var(--wa-color-surface-raised);
    box-shadow: var(--wa-shadow-xl);
  }

  .hero-shot img,
  .shot img {
    display: block;
    width: 100%;
    height: auto;
    border-radius: var(--wa-border-radius-m);
  }

  .hero-tag {
    position: absolute;
    left: 28%;
    top: 31%;
    transform: rotate(-2deg);
    box-shadow: var(--wa-shadow-s);
  }

  figcaption {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--wa-space-2xs) var(--wa-space-m);
    margin-top: var(--wa-space-xs);
    color: var(--wa-color-text-quiet);
    font-size: var(--wa-font-size-xs);
  }

  .shot-note {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  kbd {
    padding: 0 var(--wa-space-2xs);
    border: 1px solid var(--wa-color-surface-border);
    border-bottom-width: 2px;
    border-radius: var(--wa-border-radius-xs, 3px);
    background: var(--wa-color-surface-default);
    font: inherit;
    font-size: 0.7rem;
  }

  .section {
    padding-block: clamp(var(--wa-space-xl), 6vw, var(--wa-space-4xl));
    border-top: 1px solid var(--wa-color-surface-border);
  }

  .section-head {
    max-width: 40rem;
    margin-bottom: clamp(var(--wa-space-l), 4vw, var(--wa-space-2xl));
  }

  .section-head.narrow {
    max-width: 36rem;
  }

  .section-head p:last-child {
    margin-top: var(--wa-space-xs);
  }

  h2 {
    margin: 0;
    font-size: clamp(1.9rem, 3.4vw, 2.75rem);
    font-weight: 650;
    letter-spacing: -0.03em;
    line-height: 1.05;
  }

  .loop {
    margin: 0;
    padding: var(--wa-space-l) 0 0;
    border-top: 1px solid var(--wa-color-surface-border);
    list-style: none;
  }

  .num {
    display: block;
    margin-bottom: var(--wa-space-s);
    color: var(--wa-color-brand);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.1em;
  }

  .loop h3 {
    margin: 0 0 var(--wa-space-xs);
    font-size: var(--wa-font-size-l);
    letter-spacing: -0.01em;
  }

  .loop p {
    max-width: 33ch;
    margin: 0;
    font-size: var(--wa-font-size-m);
    color: var(--wa-color-text-quiet);
  }

  .mini-lane {
    display: flex;
    align-items: center;
    gap: 4px;
    height: 2rem;
    margin-top: var(--wa-space-m);
    padding: 0 var(--wa-space-2xs);
    border-left: 2px solid var(--wa-color-surface-border);
    background-image: repeating-linear-gradient(
      to right,
      var(--wa-color-surface-border) 0 1px,
      transparent 1px 25%
    );
    opacity: 0.9;
  }

  .mini-lane .block {
    height: 0.9rem;
    border-radius: var(--wa-border-radius-xs, 3px);
  }

  .mini-lane .green {
    width: 38%;
    background: var(--wa-color-brand);
  }

  .mini-lane .rust {
    width: 22%;
    background: #a4602c;
  }

  .mini-lane .draft {
    width: 18%;
    border: 1px dashed var(--wa-color-brand);
    background: color-mix(in srgb, var(--wa-color-brand) 18%, transparent);
  }

  .mini-table {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 4px;
    margin-top: var(--wa-space-m);
  }

  .mini-table span {
    height: 0.5rem;
    border-radius: 2px;
    background: var(--wa-color-surface-border);
  }

  .mini-table .on {
    background: color-mix(in srgb, var(--wa-color-brand) 55%, var(--wa-color-surface-border));
  }

  .mini-code {
    margin-top: var(--wa-space-m);
    padding: var(--wa-space-2xs) var(--wa-space-xs);
    border: 1px solid var(--wa-color-surface-border);
    border-radius: var(--wa-border-radius-s);
    background: var(--wa-color-surface-raised);
    color: var(--wa-color-text-normal);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mini-code span {
    margin-right: var(--wa-space-2xs);
    color: var(--wa-color-brand);
  }

  .split > :first-child {
    flex: 1 1 24rem;
  }

  .split > :last-child {
    flex: 1 1 26rem;
  }

  .split-copy p {
    margin: 0;
  }

  .split-copy code {
    padding: 0.05rem var(--wa-space-2xs);
    border: 1px solid var(--wa-color-surface-border);
    border-radius: var(--wa-border-radius-xs, 3px);
    background: var(--wa-color-surface-raised);
    color: var(--wa-color-text-normal);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.85em;
  }

  .shot figcaption {
    justify-content: flex-start;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .ticks {
    margin: var(--wa-space-s) 0 0;
    padding: 0;
    list-style: none;
    font-size: var(--wa-font-size-m);
  }

  .ticks li {
    display: flex;
    align-items: baseline;
    gap: var(--wa-space-xs);
  }

  .ticks wa-icon {
    color: var(--wa-color-brand);
    font-size: var(--wa-font-size-s);
    flex-shrink: 0;
  }

  .ticks.no wa-icon {
    color: var(--wa-color-text-quiet);
  }

  .text-link {
    display: inline-flex;
    align-items: center;
    gap: var(--wa-space-2xs);
    font-weight: 600;
    text-decoration: underline;
    text-decoration-color: color-mix(in srgb, var(--wa-color-brand) 35%, transparent);
    text-underline-offset: 0.25em;
  }

  .text-link wa-icon {
    font-size: var(--wa-font-size-s);
  }

  .terminal {
    overflow: hidden;
    border: 1px solid #2c4038;
    border-radius: var(--wa-border-radius-l);
    background: #17231f;
    box-shadow: var(--wa-shadow-xl);
    color: #e8ede8;
  }

  .term-bar {
    gap: var(--wa-space-2xs);
    padding: var(--wa-space-xs) var(--wa-space-s);
    border-bottom: 1px solid #2c4038;
  }

  .term-bar span {
    width: 0.4rem;
    height: 0.4rem;
    border-radius: var(--wa-border-radius-circle);
    background: #3a4a43;
  }

  .term-bar span:first-child {
    background: #a4602c;
  }

  .term-bar span:nth-child(2) {
    background: #8e6d3d;
  }

  .term-bar span:nth-child(3) {
    background: #4f7a4f;
  }

  .term-bar .term-title {
    width: auto;
    height: auto;
    margin-left: var(--wa-space-2xs);
    border-radius: 0;
    background: none;
    color: #93a99e;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.08em;
  }

  .terminal pre {
    margin: 0;
    padding: var(--wa-space-m) var(--wa-space-s);
    overflow-x: auto;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.78rem;
    line-height: 1.75;
  }

  .prompt {
    color: #a9c9b8;
    font-weight: 700;
  }

  .out {
    color: #8fa69b;
  }

  .night {
    padding-block: clamp(var(--wa-space-2xl), 7vw, var(--wa-space-4xl));
    background: #1c352d;
    color: #f2f2ec;
  }

  .night .label {
    color: #a9c9b8;
  }

  .night h2 {
    color: #fff;
  }

  .night p {
    color: #c9d4cc;
  }

  .night code {
    color: #fff;
  }

  .paths {
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--wa-font-size-m);
  }

  .paths li {
    padding-left: var(--wa-space-s);
    border-left: 2px solid rgb(255 255 255 / 18%);
    color: #c9d4cc;
  }

  .paths strong {
    color: #fff;
  }

  .paths a {
    display: inline-flex;
    align-items: center;
    gap: var(--wa-space-2xs);
    color: #a9c9b8;
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .paths a:hover {
    color: #fff;
  }

  .facts {
    margin: 0;
    padding: var(--wa-space-m) 0 0;
    border-top: 1px solid rgb(255 255 255 / 15%);
    list-style: none;
  }

  .facts li {
    color: #c9d4cc;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .facts strong {
    display: block;
    color: #fff;
    font-family: system-ui, sans-serif;
    font-size: 1.8rem;
    font-weight: 650;
    letter-spacing: -0.04em;
    line-height: 1.1;
    text-transform: none;
  }

  .install {
    padding: var(--wa-space-m);
    border: 1px solid rgb(255 255 255 / 14%);
    border-radius: var(--wa-border-radius-l);
    background: #13251f;
  }

  .install-label {
    margin: 0 !important;
    color: #a9c9b8 !important;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs) !important;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .install pre {
    margin: 0;
    overflow-x: auto;
    color: #e8ede8;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.72rem;
    line-height: 1.8;
    white-space: pre;
  }

  .install-note {
    margin: 0 !important;
    color: #93a99e !important;
    font-size: var(--wa-font-size-xs) !important;
  }

  .scope {
    margin-top: var(--wa-space-xl);
  }

  .scope h3 {
    margin: 0 0 var(--wa-space-s);
    padding-bottom: var(--wa-space-xs);
    border-bottom: 1px solid var(--wa-color-surface-border);
    color: var(--wa-color-text-quiet);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    font-weight: 500;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .scope-note {
    margin-top: var(--wa-space-l);
    font-size: var(--wa-font-size-s);
    color: var(--wa-color-text-quiet);
  }

  .scope-note a {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .close {
    padding-block: clamp(var(--wa-space-xl), 6vw, var(--wa-space-3xl));
    border-top: 1px solid var(--wa-color-surface-border);
  }

  .close h2 {
    margin: 0;
    font-size: clamp(1.6rem, 2.6vw, 2.1rem);
  }

  .close p {
    margin: var(--wa-space-2xs) 0 0;
    color: var(--wa-color-text-quiet);
  }

  .foot {
    padding-block: var(--wa-space-m) var(--wa-space-l);
    border-top: 1px solid var(--wa-color-surface-border);
    background: var(--wa-color-surface-raised);
  }

  .foot-inner {
    color: var(--wa-color-text-quiet);
    font-size: var(--wa-font-size-s);
  }

  .foot nav a {
    color: var(--wa-color-text-quiet);
  }

  .foot nav a:hover {
    color: var(--wa-color-text-normal);
  }

  .foot-note {
    flex-basis: 100%;
    color: var(--wa-color-text-quiet);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: var(--wa-font-size-xs);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  @media (prefers-reduced-motion: no-preference) {
    .hero-copy > * {
      animation: rise 0.55s ease both;
    }

    .hero-copy > :nth-child(2) {
      animation-delay: 0.05s;
    }

    .hero-copy > :nth-child(3) {
      animation-delay: 0.1s;
    }

    .hero-copy > :nth-child(4) {
      animation-delay: 0.15s;
    }

    .hero-shot {
      animation: rise 0.7s 0.12s ease both;
    }

    .dot::after {
      content: '';
      position: absolute;
      inset: 0;
      border-radius: var(--wa-border-radius-circle);
      background: var(--wa-color-brand);
      animation: pulse 2.6s ease-out infinite;
    }
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(12px);
    }
  }

  @keyframes pulse {
    0% {
      opacity: 0.5;
      transform: scale(1);
    }
    70%,
    100% {
      opacity: 0;
      transform: scale(2.6);
    }
  }

  @media (max-width: 60rem) {
    .hero {
      grid-template-columns: minmax(0, 1fr);
    }

    .hero-shot {
      order: 2;
    }

    .split.reverse > :last-child {
      order: 2;
    }

    .mini-lane,
    .mini-table,
    .mini-code {
      max-width: 22rem;
    }
  }

  @media (max-width: 40rem) {
    .wrap {
      width: calc(100% - 2rem);
    }

    .head-inner {
      min-height: 3.5rem;
    }

    h1 {
      font-size: clamp(2.5rem, 12vw, 3.5rem);
    }

    .hero-tag {
      display: none;
    }

    .hero-shot figcaption {
      display: block;
    }

    .shot-note {
      display: block;
      margin-top: var(--wa-space-2xs);
    }

    .foot nav {
      margin-left: 0;
    }

    .foot nav a,
    .text-link {
      display: inline-flex;
      align-items: center;
      min-height: 44px;
    }
  }
</style>
