const { test, expect } = require('playwright/test');
const { login } = require('./helpers');

test.describe('webhook management', () => {
  test('create and delete a webhook endpoint', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Webhooks' }).click();
    await expect(page.getByRole('heading', { name: 'Webhooks' })).toBeVisible();

    await page.getByPlaceholder('Ops webhook').fill('e2e-webhook');
    await page.getByPlaceholder('https://hooks.example.com/ingest').fill('http://127.0.0.1:19099/hook');
    await page.getByPlaceholder('forward:created, proxy:deleted, cron:updated').fill('forward:created');
    await page.getByRole('button', { name: 'Save Webhook' }).click();

    const row = page.locator('tr', { hasText: 'e2e-webhook' });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText('127.0.0.1:19099');

    await row.getByRole('button', { name: 'Delete webhook' }).click();
    await expect(row).toBeHidden({ timeout: 15_000 });
  });
});
