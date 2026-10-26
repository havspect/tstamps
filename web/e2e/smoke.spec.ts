import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const state = JSON.parse(
  readFileSync(fileURLToPath(new URL('./.state.json', import.meta.url)), 'utf8'),
) as { port: number; password: string; workspaceSlug: string }
const baseURL = `http://127.0.0.1:${state.port}`
const workspace = `/w/${state.workspaceSlug}`

test.use({ baseURL })

async function signIn(page: Page) {
  await page.goto(`${workspace}/login`)
  await page.getByLabel('Email').fill('admin@example.com')
  await page.getByLabel('Password').fill(state.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()
}

test('homepage introduces the product and links to sign in', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Your week')
  await expect(page).toHaveTitle(/Tstamps/)

  // The static tags ship in index.html: crawlers that never run JS still get them.
  const description = page.locator('meta[name="description"]')
  await expect(description).toHaveCount(1)
  expect(await description.getAttribute('content')).toBeTruthy()
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', '/')
  await expect(page.getByRole('link', { name: 'Docs' }).first()).toBeVisible()

  const robots = await page.request.get('/robots.txt')
  expect(robots.status()).toBe(200)
  expect(await robots.text()).toContain('Allow: /')
  expect(await robots.text()).toContain(`Sitemap: ${baseURL}/sitemap.xml`)

  const sitemap = await page.request.get('/sitemap.xml')
  expect(sitemap.status()).toBe(200)
  expect(await sitemap.text()).toContain(`<loc>${baseURL}/</loc>`)

  expect((await page.request.get('/screenshots/timeline.webp')).status()).toBe(200)

  const home = await page.request.get('/')
  const html = await home.text()
  expect(html).toContain('<noscript>')
  expect(html).toContain('self-hosted or hosted team time tracking')
  expect(home.headers()['x-robots-tag']).toBeUndefined()

  const login = await page.request.get('/login')
  expect(login.headers()['x-robots-tag']).toContain('noindex')
  await page.getByRole('link', { name: 'Sign in' }).first().click()
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
})

test('the manual is public at /docs', async ({ page }) => {
  await page.goto('/docs/')
  await expect(page.getByRole('heading', { level: 1, name: 'Tstamps' })).toBeVisible()
  await page.getByRole('link', { name: 'CLI reference' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'CLI reference' })).toBeVisible()
  await expect(page.locator('label[data-title="Linux"]').first()).toBeVisible()
  await expect(page.locator('label[data-title="Windows"]').first()).toBeVisible()
})

test('login, log an entry, read the report and export CSV', async ({ page }) => {
  await signIn(page)

  // '/' is public for signed-in users too: it renders instead of redirecting.
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Your week')
  await expect(page).toHaveURL('/')

  // The App guard still sends a signed-in user from /login to /timeline.
  await page.getByRole('link', { name: 'Sign in' }).first().click()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()
  // Web Awesome hosts don't forward aria-label, so the stepper names are slotted text.
  await expect(page.getByRole('button', { name: 'Previous day' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Next day' })).toBeVisible()

  // Seed the workspace over the API; the admin screens are not what this covers.
  const origin = { Origin: baseURL }
  const team = await page.request.post(`${workspace}/api/v1/teams`, { data: { name: 'Platform' }, headers: origin })
  expect(team.ok()).toBeTruthy()
  const project = await page.request.post(`${workspace}/api/v1/projects`, {
    data: { name: 'Smoke Project', team_id: (await team.json()).id },
    headers: origin,
  })
  expect(project.ok()).toBeTruthy()

  // The timeline fetched its projects before the seed; reload to see them.
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()

  // Put the project on a lane, then create an entry from the keyboard path.
  await page.getByRole('button', { name: 'Add project lane' }).click()
  await page.locator('wa-dialog').getByRole('button', { name: /Smoke Project/ }).click()
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

  await page.goto(`${workspace}/reports`)
  await expect(page.getByRole('rowheader', { name: /Smoke Project/ })).toBeVisible()
  // The selected period is announced even though wa-button drops aria-pressed.
  await expect(page.getByRole('button', { name: /Day.*selected/ })).toBeVisible()

  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download CSV' }).click()
  const csv = readFileSync(await (await download).path(), 'utf8')
  expect(csv).toContain('Smoke Project')
  expect(csv).toContain('smoke note')
})

test('admin controls bind the custom elements to the forms', async ({ page }) => {
  await signIn(page)
  const stamp = Date.now()
  const email = `admin-check-${stamp}@example.com`

  await page.goto(`${workspace}/admin/users`)
  await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()

  // The nav already has aria-label "Admin", so target the toolbar checkbox by
  // its container instead of by its label.
  const admin = page.locator('.toolbar wa-checkbox')
  await admin.click()
  await expect(admin).toHaveJSProperty('checked', true)

  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Name').fill('Admin Check')
  await page.getByRole('button', { name: 'Create user' }).click()

  const row = page.locator('tr', { hasText: email })
  await expect(row).toBeVisible()
  await expect(row.locator('td').nth(3)).toHaveText('yes')

  const teamName = `Admin Check ${stamp}`
  const created = await page.request.post(`${workspace}/api/v1/teams`, {
    data: { name: teamName },
    headers: { Origin: baseURL },
  })
  expect(created.ok()).toBeTruthy()

  await page.goto(`${workspace}/admin/teams`)
  const teamRow = page.locator('tr', { hasText: teamName })
  await teamRow.getByRole('button', { name: 'Members' }).click()

  const member = page.locator('.member', { hasText: email })
  await expect(member).toBeVisible()
  // Native select: its aria-label is exposed, unlike a label on a custom-element host.
  const roleSelect = member.getByRole('combobox', { name: /Role for/ })
  await expect(roleSelect).toBeVisible()
  await member.locator('wa-checkbox').click()
  await roleSelect.selectOption('lead')

  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === 'PUT' &&
      new URL(response.url()).pathname.endsWith('/members'),
  )
  await page.getByRole('button', { name: 'Save members' }).click()
  expect((await saved).ok()).toBeTruthy()

  const teams = await page.request.get(`${workspace}/api/v1/teams`)
  expect(teams.ok()).toBeTruthy()
  const body = (await teams.json()) as {
    name: string
    members: { email: string; role: string }[] | null
  }[]
  const memberRole = body
    .find((team) => team.name === teamName)
    ?.members?.find((entry) => entry.email === email)?.role
  expect(memberRole).toBe('lead')
})
