import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const state = JSON.parse(
  readFileSync(fileURLToPath(new URL('./.state.json', import.meta.url)), 'utf8'),
) as { port: number; password: string }
const baseURL = `http://127.0.0.1:${state.port}`

test.use({ baseURL })

test('homepage introduces the product and links to sign in', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Your week')
  await page.getByRole('link', { name: 'Sign in' }).first().click()
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
})

test('login, log an entry, read the report and export CSV', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Email').fill('admin@example.com')
  await page.getByLabel('Password').fill(state.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()

  // Seed the workspace over the API; the admin screens are not what this covers.
  const origin = { Origin: baseURL }
  const team = await page.request.post('/api/v1/teams', { data: { name: 'Platform' }, headers: origin })
  expect(team.ok()).toBeTruthy()
  const project = await page.request.post('/api/v1/projects', {
    data: { name: 'Smoke Project', team_id: (await team.json()).id },
    headers: origin,
  })
  expect(project.ok()).toBeTruthy()

  // The timeline fetched its projects before the seed; reload to see them.
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()

  // Put the project on a lane, then create an entry from the keyboard path.
  await page.getByRole('button', { name: 'Add project lane' }).click()
  await page.getByRole('dialog', { name: 'Add project' }).getByRole('button', { name: /Smoke Project/ }).click()
  await page.getByRole('heading', { name: 'Timeline' }).click()
  await page.keyboard.press('n')

  const panel = page.getByRole('complementary', { name: 'New entry' })
  await expect(panel).toBeVisible()
  await panel.getByLabel('Note').fill('smoke note')

  // Stay in the past and inside today, so the block is on the current day.
  const now = Date.now()
  const local = (ms: number) => {
    const d = new Date(ms)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  await panel.getByLabel('Start').fill(local(now - 90 * 60_000))
  await panel.getByLabel('End').fill(local(now - 30 * 60_000))
  await panel.getByRole('button', { name: 'Save' }).click()

  const block = page.locator('[aria-label*="smoke note"]')
  await block.scrollIntoViewIfNeeded()
  await expect(block).toBeVisible()

  await page.goto('/reports')
  await expect(page.getByRole('rowheader', { name: /Smoke Project/ })).toBeVisible()

  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download CSV' }).click()
  const csv = readFileSync(await (await download).path(), 'utf8')
  expect(csv).toContain('Smoke Project')
  expect(csv).toContain('smoke note')
})
