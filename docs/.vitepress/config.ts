import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Tstamps',
  description: 'Team time tracking on a timeline. One binary, one SQLite database, yours to run.',
  base: '/docs/',
  srcDir: 'guide',
  outDir: '../web/dist/docs',
  publicDir: 'guide/public',
  cleanUrls: true,
  appearance: false,
  lastUpdated: false,
  head: [['link', { rel: 'icon', href: '/docs/favicon.svg', type: 'image/svg+xml' }]],
  ignoreDeadLinks: [/^\/api\//],
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/' },
      { text: 'API reference', link: '../api/v1/docs' },
      { text: 'GitHub', link: 'https://github.com/havspect/tstamps' },
    ],
    sidebar: [
      {
        text: 'Getting started',
        items: [
          { text: 'Overview', link: '/' },
          { text: 'Install the CLI', link: '/install-cli' },
        ],
      },
      {
        text: 'Using Tstamps',
        items: [
          { text: 'Timeline', link: '/timeline' },
          { text: 'Reports', link: '/reports' },
          { text: 'CLI reference', link: '/cli' },
        ],
      },
      {
        text: 'Agents',
        items: [
          { text: 'MCP', link: '/mcp' },
          { text: 'Agent skill', link: '/agents/' },
          { text: 'Claude', link: '/agents/claude' },
          { text: 'Codex', link: '/agents/codex' },
          { text: 'Langdock', link: '/agents/langdock' },
          { text: 'OpenCode', link: '/agents/opencode' },
        ],
      },
      {
        text: 'Running a workspace',
        items: [
          { text: 'Admin console', link: '/admin' },
          { text: 'Server on Linux', link: '/server' },
          { text: 'Hosted workspaces', link: '/workspaces' },
          { text: 'Backup and upgrade', link: '/backup-upgrade' },
        ],
      },
    ],
    search: { provider: 'local' },
    outline: { level: [2, 3], label: 'On this page' },
    docFooter: { prev: 'Previous', next: 'Next' },
    editLink: {
      pattern: 'https://github.com/havspect/tstamps/edit/main/docs/guide/:path',
      text: 'Edit this page',
    },
    footer: {
      message: 'AGPL-3.0-only. Built with Go and Svelte.',
      copyright: 'One binary, one SQLite database.',
    },
    socialLinks: [{ icon: 'github', link: 'https://github.com/havspect/tstamps' }],
  },
})
