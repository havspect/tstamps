import { execFileSync, spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { createServer, type AddressInfo } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const server = process.env.TSTAMPS_SERVER ?? fileURLToPath(new URL('../../tstamps-server', import.meta.url))
const statePath = fileURLToPath(new URL('./.state.json', import.meta.url))
const workspaceSlug = 'example.com'

// Bootstraps three throwaway databases with the real server binary: one with a
// single workspace (the no-prompt path), one with two (the workspace prompt) and
// the seeded demo, whose plans and vacations no hand-written fixture reproduces.
export default async function globalSetup() {
  const dir = mkdtempSync(join(tmpdir(), 'tstamps-e2e-'))
  const singleDB = join(dir, 'single.db')
  const hostedDB = join(dir, 'hosted.db')
  const demoDB = join(dir, 'demo.db')
  let single: ChildProcess | undefined
  let hosted: ChildProcess | undefined
  let demo: ChildProcess | undefined

  // Free ports let two worktrees run the walk at the same time. A port can be
  // taken between the probe and the listen; the window is small, and a clash
  // fails on /healthz rather than driving another worktree's server.
  const ports: number[] = []
  const port = Number(process.env.TSTAMPS_E2E_PORT ?? 0) || (await freePort(ports))
  const hostedPort = Number(process.env.TSTAMPS_E2E_HOSTED_PORT ?? 0) || (await freePort(ports))
  const demoPort = Number(process.env.TSTAMPS_E2E_DEMO_PORT ?? 0) || (await freePort(ports))
  const baseURL = `http://127.0.0.1:${port}`
  const hostedURL = `http://127.0.0.1:${hostedPort}`
  const demoURL = `http://127.0.0.1:${demoPort}`

  try {
    const output = execFileSync(
      server,
      ['workspace', 'create', workspaceSlug, '--db', singleDB, '--email', 'admin@example.com', '--name', 'Ada Admin'],
      { encoding: 'utf8' },
    )
    const password = /initial password: (\S+)/.exec(output)?.[1]
    if (!password) throw new Error(`workspace create printed no password: ${output}`)

    const hostedOutput = execFileSync(
      server,
      [
        'workspace', 'create', workspaceSlug,
        '--db', hostedDB,
        '--email', 'admin@example.com',
        '--name', 'Ada Hosted',
      ],
      { encoding: 'utf8' },
    )
    const hostedPassword = /initial password: (\S+)/.exec(hostedOutput)?.[1]
    if (!hostedPassword) throw new Error(`workspace create printed no password: ${hostedOutput}`)

    execFileSync(
      server,
      [
        'workspace', 'create', 'other.example',
        '--db', hostedDB,
        '--email', 'other@example.com',
        '--name', 'Other Admin',
      ],
      { encoding: 'utf8' },
    )

    single = spawn(
      server,
      ['serve', '--addr', `127.0.0.1:${port}`, '--db', singleDB, '--log-level', 'warn'],
      { stdio: 'ignore' },
    )
    hosted = spawn(
      server,
      ['serve', '--addr', `127.0.0.1:${hostedPort}`, '--db', hostedDB, '--log-level', 'warn'],
      { stdio: 'ignore' },
    )
    demo = spawn(
      server,
      ['serve', '--demo', '--addr', `127.0.0.1:${demoPort}`, '--db', demoDB, '--log-level', 'warn'],
      { stdio: 'ignore' },
    )
    await Promise.all([
      waitForHealth(`${baseURL}/healthz`),
      waitForHealth(`${hostedURL}/healthz`),
      waitForHealth(`${demoURL}/healthz`),
    ])
    writeFileSync(
      statePath,
      JSON.stringify({ port, password, hostedPort, hostedPassword, demoPort, workspaceSlug }),
    )

    return async () => {
      await Promise.all([stop(single), stop(hosted), stop(demo)])
      rmSync(dir, { recursive: true, force: true })
      rmSync(statePath, { force: true })
    }
  } catch (err) {
    await Promise.all([stop(single), stop(hosted), stop(demo)])
    rmSync(dir, { recursive: true, force: true })
    throw err
  }
}

async function freePort(used: number[]): Promise<number> {
  for (;;) {
    const port = await new Promise<number>((resolve, reject) => {
      const probe = createServer()
      probe.once('error', reject)
      probe.listen(0, '127.0.0.1', () => {
        const { port } = probe.address() as AddressInfo
        probe.close(() => resolve(port))
      })
    })
    if (!used.includes(port)) {
      used.push(port)
      return port
    }
  }
}

async function stop(child: ChildProcess | undefined) {
  if (!child || child.exitCode !== null) return
  child.kill('SIGTERM')
  await new Promise((resolve) => child.once('exit', resolve))
}

async function waitForHealth(url: string) {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const res = await fetch(url)
      if (res.ok) return
    } catch {
      // not listening yet
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error(`${url} did not become healthy`)
}
