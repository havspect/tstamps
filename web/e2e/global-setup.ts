import { execFileSync, spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const server = process.env.TSTAMPS_SERVER ?? fileURLToPath(new URL('../../tstamps-server', import.meta.url))
const statePath = fileURLToPath(new URL('./.state.json', import.meta.url))
const port = Number(process.env.TSTAMPS_E2E_PORT ?? 8098)
const hostedPort = Number(process.env.TSTAMPS_E2E_HOSTED_PORT ?? port + 1)
const baseURL = `http://127.0.0.1:${port}`
const hostedURL = `http://127.0.0.1:${hostedPort}`
const workspaceSlug = 'koeber.com'

// Bootstraps a throwaway legacy workspace and a hosted workspace with the real
// server binary, and leaves both running for the specs.
export default async function globalSetup() {
  const dir = mkdtempSync(join(tmpdir(), 'tstamps-e2e-'))
  const db = join(dir, 'tstamps.db')
  const workspaceDir = join(dir, 'workspaces')
  let legacy: ChildProcess | undefined
  let hosted: ChildProcess | undefined

  try {
    const output = execFileSync(
      server,
      ['admin', 'create', '--db', db, '--email', 'admin@example.com', '--name', 'Ada Admin'],
      { encoding: 'utf8' },
    )
    const password = /initial password: (\S+)/.exec(output)?.[1]
    if (!password) throw new Error(`admin create printed no password: ${output}`)

    const hostedOutput = execFileSync(
      server,
      [
        'workspace', 'create', workspaceSlug,
        '--workspace-dir', workspaceDir,
        '--email', 'admin@example.com',
        '--name', 'Ada Hosted',
      ],
      { encoding: 'utf8' },
    )
    const hostedPassword = /initial password: (\S+)/.exec(hostedOutput)?.[1]
    if (!hostedPassword) throw new Error(`workspace create printed no password: ${hostedOutput}`)

    legacy = spawn(
      server,
      ['serve', '--addr', `127.0.0.1:${port}`, '--db', db, '--log-level', 'warn'],
      { stdio: 'ignore' },
    )
    hosted = spawn(
      server,
      ['serve', '--addr', `127.0.0.1:${hostedPort}`, '--workspace-dir', workspaceDir, '--log-level', 'warn'],
      { stdio: 'ignore' },
    )
    await Promise.all([waitForHealth(`${baseURL}/healthz`), waitForHealth(`${hostedURL}/healthz`)])
    writeFileSync(
      statePath,
      JSON.stringify({ port, password, hostedPort, hostedPassword, workspaceSlug }),
    )

    return async () => {
      await Promise.all([stop(legacy), stop(hosted)])
      rmSync(dir, { recursive: true, force: true })
      rmSync(statePath, { force: true })
    }
  } catch (err) {
    await Promise.all([stop(legacy), stop(hosted)])
    rmSync(dir, { recursive: true, force: true })
    throw err
  }
}

async function stop(child: ChildProcess | undefined) {
  if (!child || child.exitCode !== null) return
  child.kill('SIGTERM')
  await new Promise((resolve) => child.once('exit', resolve))
}

async function waitForHealth(url: string) {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    try {
      if ((await fetch(url)).ok) return
    } catch {
      // Not listening yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error(`server did not answer ${url}`)
}
