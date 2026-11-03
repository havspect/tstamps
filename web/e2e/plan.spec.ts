import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { shot } from './shot'

const state = JSON.parse(
  readFileSync(fileURLToPath(new URL('./.state.json', import.meta.url)), 'utf8'),
) as { demoPort: number }
const baseURL = `http://127.0.0.1:${state.demoPort}`

test.use({ baseURL })

// The plan board needs people, projects, plans and a vacation to look like the
// product; only the demo seed has all four, and only the demo seed stays right
// when the board changes.
test('the plan board shows people, bars and a vacation week', async ({ page }) => {
  await page.goto('/w/demo/login')
  await page.getByLabel('Email').fill('ada@example.com')
  await page.getByLabel('Password').fill('demo')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()

  await page.goto('/w/demo/plan')
  await expect(page.getByRole('heading', { name: 'Plan' })).toBeVisible()

  // The board opens on next month, where the demo seed puts the plans.
  await expect(page.getByRole('rowheader', { name: /Ada Lovelace/ })).toBeVisible()
  await expect(page.getByRole('rowheader', { name: /Mia Chen/ })).toBeVisible()
  await expect(page.locator('.segment').first()).toBeVisible()
  await expect(page.locator('.vacation').first()).toBeVisible()

  await shot(page, 'plan')
})
