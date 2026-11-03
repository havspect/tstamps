import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import type { Page } from '@playwright/test'

const dir = fileURLToPath(new URL('../test-results/shots', import.meta.url))

// A picture the agent looks at after `just e2e`. It asserts nothing: the test
// around it already proves the screen is up.
export async function shot(page: Page, name: string) {
  mkdirSync(dir, { recursive: true })
  // Below-the-fold images are lazy: walk the page once so each one loads.
  await page.evaluate(async () => {
    for (let y = 0; y < document.body.scrollHeight; y += window.innerHeight) {
      window.scrollTo(0, y)
      await new Promise((resolve) => setTimeout(resolve, 50))
    }
    window.scrollTo(0, 0)
  })
  // Chart.js animates the canvas into view; without the wait it can be blank.
  if (await page.locator('canvas').count()) await page.waitForTimeout(1100)
  await page.screenshot({ path: `${dir}/${name}.png`, fullPage: true, animations: 'disabled' })
}
