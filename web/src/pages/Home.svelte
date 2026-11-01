<script lang="ts">
  import Link from '../lib/Link.svelte'
  import { router } from '../lib/router.svelte'
  import { demo } from '../lib/demo'
  import LegalLinks from '../lib/LegalLinks.svelte'
</script>

<svelte:head>
  <title>Tstamps — plan it, drag it, read it</title>
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
        <a href="#plan">Plan</a>
        <a href="#reports">Reports</a>
        <a href="#agents">Agents</a>
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
    <a href="#plan" data-drawer="close">Plan</a>
    <a href="#reports" data-drawer="close">Reports</a>
    <a href="#agents" data-drawer="close">Agents</a>
    <a href="#self-host" data-drawer="close">Self-host</a>
    <a href="/docs/" data-drawer="close">Docs</a>
    <Link href="/login" class="btn primary" data-drawer="close">Sign in</Link>
  </nav>

  <main id="main" class="home">
    <section class="hero wrap" aria-labelledby="hero-title">
      <div class="hero-copy wa-stack wa-gap-m">
        <p class="eyebrow wa-cluster wa-gap-xs">
          <span class="dot" aria-hidden="true"></span>Self-hosted team time tracking
        </p>
        <h1 id="hero-title">Plan it.<br /><span class="mark">Drag it.</span><br />Read it.</h1>
        <a class="agent-line" href="#agents">Or let your agent log it.</a>
        <p class="lede">
          Tstamps is a team time tracker where the plan and the log live in one place. Leads set next
          month's hours per person and project; people drag the week into shape; reports show what was
          planned against what happened. One Go binary, one SQLite database, yours to run.
        </p>
        <div class="actions wa-cluster wa-gap-s">
          {#if demo.accounts.length}
            <Link href="/login" class="btn primary">Try the live demo <wa-icon name="arrow-right"></wa-icon></Link>
            <a class="btn ghost" href="#plan">See the plan <wa-icon name="arrow-down"></wa-icon></a>
          {:else if router.hosted}
            <Link href="/signup" class="btn primary">Create a workspace <wa-icon name="arrow-right"></wa-icon></Link>
            <Link href="/login" class="btn ghost">Sign in</Link>
          {:else}
            <Link href="/login" class="btn primary">Sign in <wa-icon name="arrow-right"></wa-icon></Link>
            <a class="btn ghost" href="#plan">See the plan <wa-icon name="arrow-down"></wa-icon></a>
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
          height="1260"
          alt="The Tstamps timeline: one lane per project with entries for CLI, Timeline, MCP and API, a running entry and a now line."
          fetchpriority="high"
          decoding="async"
        />
        <figcaption>
          <span>Drag a block. Drag its edges. Press <kbd>space</kbd> to start and stop.</span>
          <span class="shot-note">Real UI · sample data</span>
        </figcaption>
      </figure>
    </section>

    <section id="plan" class="section" aria-labelledby="plan-title">
      <div class="wrap split wa-split wa-gap-xl">
        <div class="split-copy wa-stack wa-gap-m">
          <div class="section-head wa-stack wa-gap-xs">
            <p class="label">01 — Plan</p>
            <h2 id="plan-title">The plan is a board.</h2>
          </div>
          <p>
            Leads set how many hours each person will spend on each project, week by week, against the
            hours they can work. A bar is coloured by project; over capacity is marked, not blocked,
            because a crunch is the thing you came to see. Seed a month from last month's hours, or
            repeat a week.
          </p>
          <ul class="ticks wa-stack wa-gap-xs">
            <li><wa-icon name="check"></wa-icon>By person or by project, week by week</li>
            <li><wa-icon name="check"></wa-icon>Capacity per person, 40h by default</li>
            <li><wa-icon name="check"></wa-icon>Past weeks frozen; current and future writable</li>
            <li><wa-icon name="check"></wa-icon>A plan is never counted as logged time</li>
          </ul>
        </div>
        <figure class="shot">
          <img
            src="/screenshots/plan.webp"
            width="2048"
            height="956"
            alt="The Tstamps plan board: one row per person, one column per ISO week, stacked project bars against capacity, with one week over capacity marked."
            loading="lazy"
            decoding="async"
          />
          <figcaption>November · 4 people · one week over capacity</figcaption>
        </figure>
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
            On week and month columns each cell shows the hours logged against the plan. There is no form
            to chase and no separate analytics tool to reconcile. Export the same rows as CSV.
          </p>
          <ul class="ticks wa-stack wa-gap-xs">
            <li><wa-icon name="check"></wa-icon>Logged minus planned on every week</li>
            <li><wa-icon name="check"></wa-icon>The entry's timezone decides its day</li>
            <li><wa-icon name="check"></wa-icon>Archived projects keep their hours</li>
            <li><wa-icon name="check"></wa-icon>Read-only, scoped to the teams you lead</li>
          </ul>
        </div>
        <figure class="shot">
          <img
            src="/screenshots/reports.webp"
            width="2048"
            height="1057"
            alt="The Tstamps report: projects as rows, ISO weeks as columns, each week showing logged hours with the difference from the plan, and a grand total."
            loading="lazy"
            decoding="async"
          />
          <figcaption>October · by week · logged against planned</figcaption>
        </figure>
      </div>

      <div class="wrap wide">
        <p class="wide-note">
          Every role also gets <strong>My hours</strong>: their own chart, table and CSV. The team-wide
          table stays with the lead.
        </p>
        <figure class="wide-shot">
          <img
            src="/screenshots/my-hours.webp"
            width="2048"
            height="519"
            alt="My hours: one bar per ISO week, stacked by project, with a legend for Timeline, MCP, API and CLI."
            loading="lazy"
            decoding="async"
          />
          <figcaption>My hours · October · one bar per week, stacked by project</figcaption>
        </figure>
      </div>
    </section>

    <section id="agents" class="section" aria-labelledby="agents-title">
      <div class="wrap split reverse wa-split wa-gap-xl">
        <div class="split-copy wa-stack wa-gap-m">
          <div class="section-head wa-stack wa-gap-xs">
            <p class="label">03 — Agents</p>
            <h2 id="agents-title">Hand it to your agent.</h2>
          </div>
          <p>
            <code>tstamps mcp</code> runs locally; any MCP client can also connect to
            <code>/w/{'{slug}'}/mcp</code> over OAuth 2.1. Either way the agent acts as the person who
            approved it: scopes narrow what it can do, never widen it, and you can revoke the grant on
            <code>/tokens</code>.
          </p>
          <p>
            The shipped skill proposes entries from your calendar or the agent's session history, and
            writes only after you confirm.
          </p>
          <div class="actions wa-cluster wa-gap-s">
            <a class="text-link" href="https://github.com/havspect/tstamps-skills">
              Install the agent skill <wa-icon name="arrow-up-right-from-square"></wa-icon>
            </a>
            <a class="text-link" href="/docs/mcp">
              Read the MCP guide <wa-icon name="arrow-right"></wa-icon>
            </a>
          </div>
        </div>
        <div class="terminal" role="group" aria-label="Example MCP client configuration">
          <div class="term-bar wa-cluster" aria-hidden="true">
            <span></span><span></span><span></span>
            <span class="term-title">mcp.json</span>
          </div>
          <pre><code>&#123;
  "mcpServers": &#123;
    "tstamps": &#123;
      "url": "https://time.example.com/w/example.com/mcp"
    &#125;
  &#125;
&#125;</code></pre>
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
            single company, or host several: each workspace has its own users and projects, scoped inside
            the same SQLite database. Backups use <code>VACUUM INTO</code> and are safe while the server
            is running. The manual and the API reference ship inside the binary.
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
          <pre><code>base=https://github.com/havspect/tstamps/releases/latest/download
curl -fsSL "$base/tstamps_linux_amd64.tar.gz" | tar -xz
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
              <li><wa-icon name="check"></wa-icon>Plan by person, project and week</li>
              <li><wa-icon name="check"></wa-icon>Vacations that lower plan capacity</li>
              <li><wa-icon name="check"></wa-icon>Reports with plan drift, and My hours</li>
              <li><wa-icon name="check"></wa-icon>CLI, remote MCP and an agent skill</li>
              <li><wa-icon name="check"></wa-icon>Entry timezones that decide the day</li>
              <li><wa-icon name="check"></wa-icon>Audit log of role-based changes</li>
              <li><wa-icon name="check"></wa-icon>Several workspaces, one database</li>
            </ul>
          </div>
          <div>
            <h3>It leaves out</h3>
            <ul class="ticks no wa-stack wa-gap-xs">
              <li><wa-icon name="minus"></wa-icon>Approvals and public holidays</li>
              <li><wa-icon name="minus"></wa-icon>Invoicing and rates</li>
              <li><wa-icon name="minus"></wa-icon>SSO, billing and a mobile app</li>
              <li><wa-icon name="minus"></wa-icon>Reminders and notifications</li>
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
          {#if demo.accounts.length}
            <Link href="/login" class="btn primary">Try the live demo <wa-icon name="arrow-right"></wa-icon></Link>
            <a
              class="btn ghost"
              href="https://github.com/havspect/tstamps/blob/main/docs/SPEC.md">Read the specification</a
            >
          {:else if router.hosted}
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

  .agent-line {
    display: inline-flex;
    align-items: center;
    gap: var(--wa-space-2xs);
    margin: calc(-1 * var(--wa-space-2xs)) 0 0;
    padding-left: var(--wa-space-s);
    border-left: 2px dashed color-mix(in srgb, var(--wa-color-brand) 45%, transparent);
    color: var(--wa-color-text-normal);
    font-size: clamp(1.0625rem, 1.4vw, 1.25rem);
    font-weight: 500;
  }

  .agent-line:hover {
    color: var(--wa-color-brand);
    text-decoration: none;
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
  .shot img,
  .wide-shot img {
    display: block;
    width: 100%;
    height: auto;
    border-radius: var(--wa-border-radius-m);
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

  .split > :first-child {
    flex: 1 1 24rem;
    min-width: 0;
  }

  .split > :last-child {
    flex: 1 1 26rem;
    min-width: 0;
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

  .wide {
    margin-top: clamp(var(--wa-space-l), 4vw, var(--wa-space-2xl));
    padding-top: clamp(var(--wa-space-l), 4vw, var(--wa-space-2xl));
    border-top: 1px solid var(--wa-color-surface-border);
  }

  .wide-note {
    max-width: 56ch;
    margin: 0 0 var(--wa-space-m);
    color: var(--wa-color-text-normal);
  }

  .wide-shot {
    margin: 0;
    padding: var(--wa-space-2xs);
    border: 1px solid var(--wa-color-surface-border);
    border-radius: var(--wa-border-radius-l);
    background: var(--wa-color-surface-raised);
  }

  .wide-shot figcaption {
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
    max-width: 100%;
    padding: var(--wa-space-m) var(--wa-space-s);
    overflow-x: auto;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.78rem;
    line-height: 1.75;
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
    min-width: 0;
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
    max-width: 100%;
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
