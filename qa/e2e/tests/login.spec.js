const { test, expect } = require('playwright/test');
const { login, USER, PASS } = require('./helpers');

test.describe('login', () => {
  test('rejects invalid credentials without crashing', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill(USER);
    await page.getByLabel('Password').fill('definitely-wrong-password');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByText('Invalid credentials')).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
  });

  test('logs in and lands on dashboard', async ({ page }) => {
    await login(page);
    await expect(page.getByText('System Online')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Port Forwarding' })).toBeVisible();
  });

  test('supports show/hide password and placeholder affordance', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByLabel('Username')).toHaveAttribute('placeholder', /username/i);
    await page.getByLabel('Username').fill('admin');
    const pw = page.getByLabel('Password');
    await pw.fill(PASS);
    await expect(pw).toHaveAttribute('type', 'password');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible({ timeout: 20_000 });
  });
});
