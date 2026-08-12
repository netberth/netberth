const { test, expect } = require('playwright/test');
const { login } = require('./helpers');

test.describe('user management', () => {
  test('create and delete a viewer user', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Users' }).click();
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible();

    await page.getByPlaceholder('username').fill('e2e-viewer');
    await page.getByPlaceholder('user@example.com').fill('e2e@example.com');
    await page.getByPlaceholder('min 8 chars').fill('E2ePass123!');
    await page.getByRole('button', { name: 'Add User' }).click();

    const row = page.locator('tr', { hasText: 'e2e-viewer' });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText('e2e@example.com');

    page.once('dialog', (d) => d.accept());
    await row.getByRole('button', { name: 'Delete user' }).click();
    await expect(row).toBeHidden({ timeout: 15_000 });
  });
});
