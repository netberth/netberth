const { test, expect } = require('playwright/test');
const { login } = require('./helpers');

test.describe('port forwarding rule lifecycle', () => {
  test('create and delete a TCP rule', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Port Forwarding' }).click();
    await expect(page.getByRole('heading', { name: 'Port Forwarding' })).toBeVisible();

    await page.getByPlaceholder('Rule name').fill('e2e-fwd-rule');
    await page.getByPlaceholder('0.0.0.0').fill('127.0.0.1');
    await page.getByPlaceholder('8080').fill('33199');
    await page.getByPlaceholder('192.168.1.100').fill('127.0.0.1');
    await page.getByPlaceholder('80', { exact: true }).fill('33198');
    await page.getByRole('button', { name: 'Save Rule' }).click();

    const row = page.locator('tr', { hasText: 'e2e-fwd-rule' });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText('33199');

    await row.getByRole('button').first().click();
    await expect(row).toBeHidden({ timeout: 15_000 });
  });
});
