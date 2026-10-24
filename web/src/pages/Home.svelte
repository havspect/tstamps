<script lang="ts">
  import Link from '../lib/Link.svelte'
</script>

<svelte:head>
  <title>Tstamps — your week, in a few drags</title>
</svelte:head>

<a class="skip" href="#main">Skip to content</a>

<header class="site-head">
  <div class="wrap head-inner">
    <a class="brand" href="/">
      <span class="logo" aria-hidden="true"></span>
      Tstamps
    </a>
    <nav aria-label="Homepage">
      <a href="#loop">The loop</a>
      <a href="#reports">Reports</a>
      <a href="#cli">CLI &amp; agents</a>
      <a href="#self-host">Self-host</a>
      <a href="/docs/">Docs</a>
    </nav>
    <Link href="/login" class="nav-signin">Sign in</Link>
  </div>
</header>

<main id="main" class="home">
  <section class="hero wrap" aria-labelledby="hero-title">
    <div class="hero-copy">
      <p class="eyebrow"><span class="dot" aria-hidden="true"></span>Self-hosted team time tracking</p>
      <h1 id="hero-title">Your week, <br />in a <span class="mark">few drags.</span></h1>
      <p class="lede">
        Tstamps is a team time tracker built around a timeline. Drag on a lane to log an entry, drag its
        edges to retime it, and let the report do the remembering. One Go binary, one SQLite file, yours
        to run.
      </p>
      <div class="actions">
        <Link href="/login" class="btn primary">Sign in <span aria-hidden="true">→</span></Link>
        <a class="btn ghost" href="#loop">See the timeline <span aria-hidden="true">↓</span></a>
      </div>
      <ul class="trust" aria-label="At a glance">
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
      <span class="tag" aria-hidden="true">drag to log</span>
      <figcaption>
        <span>Drag a block. Drag its edges. Press <kbd>space</kbd> to start and stop.</span>
        <span class="shot-note">Real UI · sample data</span>
      </figcaption>
    </figure>
  </section>

  <section id="loop" class="section" aria-labelledby="loop-title">
    <div class="wrap">
      <div class="section-head">
        <p class="label">01 — The loop</p>
        <h2 id="loop-title">Three moves, no timesheet.</h2>
      </div>
      <ol class="loop">
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
    <div class="wrap split">
      <div class="split-copy">
        <p class="label">02 — Reports</p>
        <h2 id="reports-title">The report is the meeting.</h2>
        <p>
          Leads read every hour booked on their team's projects, by person and by day, week or month.
          There is no form to chase and no separate analytics tool to reconcile. Export the same rows as
          CSV.
        </p>
        <ul class="ticks">
          <li>The entry's timezone decides its day</li>
          <li>Archived projects keep their hours</li>
          <li>Read-only, scoped to the teams you lead</li>
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
    <div class="wrap split reverse">
      <div class="split-copy">
        <p class="label">03 — CLI &amp; agents</p>
        <h2 id="cli-title">Works where you do.</h2>
        <p>
          <code>tstamps start</code> is one line in a terminal. <code>tstamps mcp</code> lets an agent
          start, stop and correct entries with the permissions of the token you gave it. Both talk to
          the same HTTP API as the timeline.
        </p>
        <a class="text-link" href="https://github.com/havspect/tstamps#cli">
          Read the CLI reference <span aria-hidden="true">↗</span>
        </a>
      </div>
      <div class="terminal" role="group" aria-label="Example terminal session">
        <div class="term-bar" aria-hidden="true">
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
    <div class="wrap split">
      <div>
        <p class="label">Self-hosted, not hosted</p>
        <h2 id="host-title">Yours to run.</h2>
        <p>
          One <code>tstamps-server</code> process on one VM, behind Caddy on the same host. The UI is
          embedded in the binary; the data is one SQLite file. Backups use <code>VACUUM INTO</code> and
          are safe while the server is running.
        </p>
        <ul class="facts">
          <li><strong>1</strong>binary</li>
          <li><strong>1</strong>SQLite file</li>
          <li><strong>0</strong>containers</li>
        </ul>
      </div>
      <div class="install">
        <p class="install-label">Get the CLI</p>
        <pre><code>curl -fsSL \
  https://github.com/havspect/tstamps/releases/latest/download/tstamps_linux_amd64.tar.gz \
  | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
tstamps login --url https://time.example.com</code></pre>
        <p class="install-note">
          Linux, macOS and Windows builds. The server runs under systemd behind Caddy.
        </p>
      </div>
    </div>
  </section>

  <section class="section" aria-labelledby="small-title">
    <div class="wrap">
      <div class="section-head narrow">
        <p class="label">Scope</p>
        <h2 id="small-title">Deliberately small.</h2>
        <p>
          Tstamps is not trying to run your whole company. If you need the list on the right, choose
          something else — and that is the point.
        </p>
      </div>
      <div class="scope">
        <div>
          <h3>It does</h3>
          <ul class="ticks">
            <li>Timeline logging, one running entry</li>
            <li>Team-lead reports and CSV</li>
            <li>CLI and MCP over the same API</li>
            <li>Entry timezones that decide the day</li>
            <li>Soft delete and admin restore</li>
            <li>One workspace, one company</li>
          </ul>
        </div>
        <div>
          <h3>It leaves out</h3>
          <ul class="ticks no">
            <li>Approvals, leave and holidays</li>
            <li>Invoicing and rates</li>
            <li>SSO and an audit log</li>
            <li>A mobile app</li>
            <li>Realtime co-editing</li>
            <li>Several companies per install</li>
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
    <div class="wrap close-inner">
      <div>
        <h2 id="close-title">Open your workspace.</h2>
        <p>If your server is already running, the timeline is one sign-in away.</p>
      </div>
      <div class="actions">
        <Link href="/login" class="btn primary">Sign in <span aria-hidden="true">→</span></Link>
        <a
          class="btn ghost"
          href="https://github.com/havspect/tstamps/blob/main/docs/SPEC.md">Read the specification</a
        >
      </div>
    </div>
  </section>
</main>

<footer class="foot">
  <div class="wrap foot-inner">
    <span class="foot-brand"><span class="logo" aria-hidden="true"></span>Tstamps</span>
    <nav aria-label="Project links">
      <a href="/docs/">Docs</a>
      <a href="https://github.com/havspect/tstamps">GitHub</a>
      <a href="https://github.com/havspect/tstamps/blob/main/docs/SPEC.md">Specification</a>
      <a href="https://github.com/havspect/tstamps/blob/main/GLOSSARY.md">Glossary</a>
      <a href="https://github.com/havspect/tstamps/tree/main/docs/adr">Decisions</a>
      <a href="https://github.com/havspect/tstamps/blob/main/LICENSE">AGPL-3.0</a>
    </nav>
    <span class="foot-note">Built with Go and Svelte.</span>
  </div>
</footer>

<style>
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
    left: var(--space-3);
    top: -4rem;
    z-index: 20;
    padding: var(--space-2) var(--space-3);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
  }

  .skip:focus {
    top: var(--space-2);
    text-decoration: none;
  }

  .site-head {
    padding: 0;
    border-bottom: 1px solid var(--border);
  }

  .head-inner {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-height: 3.5rem;
  }

  .brand,
  .foot-brand {
    display: inline-flex;
    align-items: center;
    gap: 0.55rem;
    color: var(--text);
    font-weight: 700;
    letter-spacing: -0.02em;
  }

  .brand:hover {
    text-decoration: none;
  }

  .logo {
    position: relative;
    display: inline-block;
    width: 1.125rem;
    height: 1.125rem;
    border-radius: 0.25rem;
    background: var(--accent);
  }

  .logo::before {
    content: '';
    position: absolute;
    left: 50%;
    top: 50%;
    width: 2px;
    height: 0.5rem;
    border-radius: 1px;
    background: #fff;
    transform: translate(-50%, -50%);
  }

  .logo::after {
    content: '';
    position: absolute;
    left: 50%;
    bottom: 0.2rem;
    width: 0.5rem;
    height: 2px;
    border-radius: 1px;
    background: #fff;
    transform: translateX(-50%);
  }

  .site-head nav {
    display: flex;
    gap: 1.5rem;
    margin-left: auto;
    font-size: 0.9375rem;
  }

  .site-head nav a {
    color: var(--muted);
  }

  .site-head nav a:hover {
    color: var(--text);
    text-decoration: none;
  }

  /* The three /login anchors are <Link> components: their root <a> carries no
     page scope class, so these two classes are styled globally. Both names exist
     only on this page. */
  :global(.nav-signin) {
    display: inline-flex;
    align-items: center;
    min-height: 2rem;
    padding: 0 0.75rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    font-size: 0.875rem;
    font-weight: 600;
  }

  :global(.nav-signin):hover {
    border-color: var(--accent);
    color: var(--accent);
    text-decoration: none;
  }

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1.02fr) minmax(0, 0.98fr);
    align-items: center;
    gap: clamp(2.5rem, 6vw, 5rem);
    padding-block: clamp(3.5rem, 8vw, 6.5rem) clamp(3rem, 6vw, 5rem);
  }

  .eyebrow,
  .label {
    margin: 0;
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    letter-spacing: 0.11em;
    line-height: 1.4;
    text-transform: uppercase;
  }

  .eyebrow {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    margin-bottom: 1.5rem;
  }

  .dot {
    position: relative;
    width: 0.4375rem;
    height: 0.4375rem;
    border-radius: 50%;
    background: var(--accent);
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
    border-radius: 0.1em;
    background: var(--accent);
    color: #fff;
    line-height: 0.95;
  }

  .lede {
    max-width: 46ch;
    margin: 1.6rem 0 2rem;
    font-size: clamp(1.0625rem, 1.4vw, 1.1875rem);
    line-height: 1.6;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
  }

  :global(.btn) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    min-height: 3rem;
    padding: 0 1.25rem;
    border: 1px solid transparent;
    border-radius: var(--radius);
    font-size: 0.9375rem;
    font-weight: 600;
    transition:
      background 0.15s ease,
      border-color 0.15s ease,
      color 0.15s ease,
      transform 0.15s ease;
  }

  :global(.btn):hover {
    text-decoration: none;
    transform: translateY(-1px);
  }

  :global(.btn.primary) {
    background: var(--accent);
    color: #fff;
  }

  :global(.btn.primary):hover {
    background: var(--accent-strong);
  }

  :global(.btn.ghost) {
    border-color: var(--border);
    background: var(--surface);
    color: var(--text);
  }

  :global(.btn.ghost):hover {
    border-color: var(--accent);
    color: var(--accent);
  }

  .trust {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem 0.8rem;
    margin: 2rem 0 0;
    padding: 0;
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    letter-spacing: 0.08em;
    list-style: none;
    text-transform: uppercase;
  }

  .trust li + li::before {
    content: '·';
    margin-right: 0.8rem;
    color: var(--border);
  }

  .hero-shot,
  .shot {
    position: relative;
    margin: 0;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--surface);
    box-shadow: 0 40px 80px -48px rgb(29 29 26 / 45%);
  }

  .hero-shot img,
  .shot img {
    display: block;
    width: 100%;
    height: auto;
    border-radius: 6px;
  }

  .tag {
    position: absolute;
    left: 28%;
    top: 31%;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface);
    color: var(--text);
    box-shadow: 0 6px 18px -10px rgb(29 29 26 / 55%);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.625rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    transform: rotate(-2deg);
  }

  .tag::before {
    content: '';
    display: inline-block;
    width: 0.3rem;
    height: 0.3rem;
    margin-right: 0.35rem;
    border-radius: 50%;
    background: var(--accent);
  }

  figcaption {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 0.35rem 1rem;
    margin-top: 0.6rem;
    color: var(--muted);
    font-size: 0.75rem;
  }

  .shot-note {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.65rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  kbd {
    padding: 0 0.25rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 3px;
    background: var(--bg);
    font: inherit;
    font-size: 0.7rem;
  }

  .section {
    padding-block: clamp(3rem, 6vw, 5rem);
    border-top: 1px solid var(--border);
  }

  .section-head {
    max-width: 40rem;
    margin-bottom: clamp(2rem, 4vw, 3rem);
  }

  .section-head.narrow {
    max-width: 36rem;
  }

  .section-head p:last-child {
    margin-top: 1rem;
  }

  h2 {
    margin: 0.6rem 0 0;
    font-size: clamp(1.9rem, 3.4vw, 2.75rem);
    font-weight: 650;
    letter-spacing: -0.03em;
    line-height: 1.05;
  }

  .loop {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin: 0;
    padding: 0;
    border-top: 1px solid var(--border);
    list-style: none;
  }

  .loop li {
    padding: 1.75rem 1.75rem 0 0;
  }

  .loop li + li {
    padding-left: 1.75rem;
    border-left: 1px solid var(--border);
  }

  .num {
    display: block;
    margin-bottom: 1.1rem;
    color: var(--accent);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.7rem;
    letter-spacing: 0.1em;
  }

  .loop h3 {
    margin: 0 0 0.5rem;
    font-size: 1.125rem;
    letter-spacing: -0.01em;
  }

  .loop p {
    max-width: 33ch;
    margin: 0;
    font-size: 0.9375rem;
  }

  .mini-lane {
    display: flex;
    align-items: center;
    gap: 4px;
    height: 2rem;
    margin-top: 1.5rem;
    padding: 0 6px;
    border-left: 2px solid var(--border);
    background-image: repeating-linear-gradient(
      to right,
      var(--border) 0 1px,
      transparent 1px 25%
    );
    opacity: 0.9;
  }

  .mini-lane .block {
    height: 0.9rem;
    border-radius: 3px;
  }

  .mini-lane .green {
    width: 38%;
    background: var(--accent);
  }

  .mini-lane .rust {
    width: 22%;
    background: #a4602c;
  }

  .mini-lane .draft {
    width: 18%;
    border: 1px dashed var(--accent);
    background: color-mix(in srgb, var(--accent) 18%, transparent);
  }

  .mini-table {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 4px;
    margin-top: 1.5rem;
  }

  .mini-table span {
    height: 0.5rem;
    border-radius: 2px;
    background: var(--border);
  }

  .mini-table .on {
    background: color-mix(in srgb, var(--accent) 55%, var(--border));
  }

  .mini-code {
    margin-top: 1.5rem;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface);
    color: var(--text);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.72rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mini-code span {
    margin-right: 0.35rem;
    color: var(--accent);
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.18fr);
    align-items: center;
    gap: clamp(2.5rem, 5vw, 4.5rem);
  }

  .split.reverse {
    grid-template-columns: minmax(0, 1.05fr) minmax(0, 0.95fr);
  }

  .split-copy p {
    margin-top: 1rem;
  }

  .split-copy code {
    padding: 0.05rem 0.3rem;
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--surface);
    color: var(--text);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.85em;
  }

  .shot figcaption {
    justify-content: flex-start;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.65rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .ticks {
    display: grid;
    gap: 0.55rem;
    margin: 1.5rem 0 0;
    padding: 0;
    font-size: 0.9375rem;
    list-style: none;
  }

  .ticks li {
    position: relative;
    padding-left: 1.35rem;
  }

  .ticks li::before {
    content: '✓';
    position: absolute;
    left: 0;
    color: var(--accent);
    font-weight: 700;
  }

  .ticks.no li::before {
    content: '×';
    color: var(--muted);
    font-weight: 400;
  }

  .text-link {
    display: inline-block;
    margin-top: 1.5rem;
    font-weight: 600;
    text-decoration: underline;
    text-decoration-color: color-mix(in srgb, var(--accent) 35%, transparent);
    text-underline-offset: 0.25em;
  }

  .terminal {
    overflow: hidden;
    border: 1px solid #2c4038;
    border-radius: 10px;
    background: #17231f;
    box-shadow: 0 24px 60px -40px rgb(29 29 26 / 55%);
    color: #e8ede8;
  }

  .term-bar {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.7rem 0.9rem;
    border-bottom: 1px solid #2c4038;
  }

  .term-bar span {
    width: 0.4rem;
    height: 0.4rem;
    border-radius: 50%;
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
    margin-left: 0.5rem;
    border-radius: 0;
    background: none;
    color: #93a99e;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.7rem;
    letter-spacing: 0.08em;
  }

  .terminal pre {
    margin: 0;
    padding: 1.25rem 1.1rem;
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
    padding-block: clamp(3.5rem, 7vw, 5.5rem);
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

  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem 2.5rem;
    margin: 2rem 0 0;
    padding: 1.5rem 0 0;
    border-top: 1px solid rgb(255 255 255 / 15%);
    list-style: none;
  }

  .facts li {
    color: #c9d4cc;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.68rem;
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
    padding: 1.25rem;
    border: 1px solid rgb(255 255 255 / 14%);
    border-radius: 10px;
    background: #13251f;
  }

  .install-label {
    margin: 0 0 0.75rem !important;
    color: #a9c9b8 !important;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.68rem !important;
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
    margin: 1rem 0 0 !important;
    color: #93a99e !important;
    font-size: 0.78rem !important;
  }

  .scope {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: clamp(2rem, 5vw, 4rem);
    margin-top: 2.5rem;
  }

  .scope h3 {
    margin: 0;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid var(--border);
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.7rem;
    font-weight: 500;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .scope-note {
    margin-top: 2rem;
    font-size: 0.875rem;
  }

  .close {
    padding-block: clamp(3rem, 6vw, 4.5rem);
    border-top: 1px solid var(--border);
  }

  .close-inner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 2rem;
  }

  .close h2 {
    margin: 0;
    font-size: clamp(1.6rem, 2.6vw, 2.1rem);
  }

  .close p {
    margin: 0.5rem 0 0;
  }

  .foot {
    padding-block: 1.5rem 2rem;
    border-top: 1px solid var(--border);
  }

  .foot-inner {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 1rem 2rem;
    color: var(--muted);
    font-size: 0.8125rem;
  }

  .foot-brand {
    font-size: 0.9375rem;
  }

  .foot nav {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    margin-left: auto;
  }

  .foot nav a {
    color: var(--muted);
  }

  .foot nav a:hover {
    color: var(--text);
  }

  .foot-note {
    flex-basis: 100%;
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.65rem;
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
      border-radius: 50%;
      background: var(--accent);
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
    .site-head nav {
      display: none;
    }

    :global(.nav-signin) {
      margin-left: auto;
    }

    .hero {
      grid-template-columns: minmax(0, 1fr);
    }

    .hero-shot {
      order: 2;
    }

    .loop {
      grid-template-columns: minmax(0, 1fr);
    }

    .loop li,
    .loop li + li {
      padding: 1.5rem 0;
      border-left: 0;
    }

    .loop li + li {
      border-top: 1px solid var(--border);
    }

    .loop p {
      max-width: 46ch;
    }

    .mini-lane,
    .mini-table,
    .mini-code {
      max-width: 22rem;
    }

    .split,
    .split.reverse {
      grid-template-columns: minmax(0, 1fr);
    }

    .split.reverse .terminal {
      order: 2;
    }

    .close-inner {
      display: block;
    }

    .close .actions {
      margin-top: 1.5rem;
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

    .lede {
      margin-top: 1.25rem;
    }

    .actions {
      display: grid;
    }

    .tag {
      display: none;
    }

    .hero-shot figcaption {
      display: block;
    }

    .shot-note {
      display: block;
      margin-top: 0.25rem;
    }

    .facts {
      gap: 1rem 1.75rem;
    }

    .scope {
      grid-template-columns: minmax(0, 1fr);
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
