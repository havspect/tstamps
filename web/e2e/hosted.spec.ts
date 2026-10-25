import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const state = JSON.parse(
  readFileSync(fileURLToPath(new URL('./.state.json', import.meta.url)), 'utf8'),
) as { hostedPort: number; hostedPassword: string; workspaceSlug: string }
const baseURL = `http://127.0.0.1:${state.hostedPort}`

test.use({ baseURL })

test('choose a workspace, log in and keep the workspace in every URL', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: /Your week/ })).toBeVisible()
  await page.getByRole('link', { name: 'Sign in' }).first().click()

  await expect(page).toHaveURL('/login')
  await expect(page.getByRole('heading', { name: 'Open your workspace' })).toBeVisible()
  await page.getByLabel('Workspace').fill(state.workspaceSlug.toUpperCase())
  await page.getByRole('button', { name: 'Continue' }).click()

  await expect(page).toHaveURL(`/w/${state.workspaceSlug}/login`)
  await page.getByLabel('Email').fill('admin@example.com')
  await page.getByLabel('Password').fill(state.hostedPassword)
  await page.getByRole('button', { name: 'Sign in' }).click()

  await expect(page).toHaveURL(`/w/${state.workspaceSlug}/timeline`)
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible()
  await page.goto(`/w/${state.workspaceSlug}/reports`)
  await expect(page.getByRole('heading', { name: 'Reports' })).toBeVisible()
})

test('an unknown workspace does not create one or show a login form', async ({ page }) => {
  await page.goto('/w/not-a-workspace/login')
  await expect(page.getByRole('heading', { name: 'Workspace not found' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Sign in' })).toHaveCount(0)
})

test('a typed workspace that does not exist shows an error and stays on the prompt', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Workspace').fill('not-a-workspace')
  await page.getByRole('button', { name: 'Continue' }).click()
  await expect(page.getByRole('alert')).toContainText('Workspace not-a-workspace does not exist')
  await expect(page).toHaveURL('/login')
})

test('a hosted homepage offers signup and says when it is disabled', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('link', { name: 'Create a workspace' }).first().click()

  await expect(page).toHaveURL('/signup')
  await expect(page.getByRole('heading', { name: 'Create a workspace' })).toBeVisible()
  await page.getByLabel('Email').fill('ada@example.com')
  await page.getByRole('button', { name: 'Send link' }).click()

  // The e2e server runs without email settings, so signup is the off switch.
  await expect(page.getByRole('alert')).toContainText('not enabled')
})
