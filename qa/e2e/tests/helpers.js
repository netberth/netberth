const { expect } = require('playwright/test');

const PASS = process.env.NB_QA_PASS || '';
const USER = process.env.NB_QA_USER || 'admin';
if (!PASS) throw new Error('NB_QA_PASS is required: set env NB_QA_PASS');

async function login(page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill(USER);
  await page.getByLabel('Password').fill(PASS);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible({ timeout: 20_000 });
}

module.exports = { login, USER, PASS };
