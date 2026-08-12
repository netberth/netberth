const { test, expect } = require('playwright/test');
const { login } = require('./helpers');

const NAV = [
  { label: 'Dashboard', heading: 'Dashboard', path: '/' },
  { label: 'Port Forwarding', heading: 'Port Forwarding', path: '/forward' },
  { label: 'Reverse Proxy', heading: 'Reverse Proxy', path: '/proxy' },
  { label: 'DDNS', heading: 'Dynamic DNS', path: '/ddns' },
  { label: 'STUN Tunnel', heading: 'STUN Tunnel', path: '/stun' },
  { label: 'Wake-on-LAN', heading: 'Wake-on-LAN', path: '/wol' },
  { label: 'Cron Jobs', heading: 'Cron Jobs', path: '/cron' },
  { label: 'Certificates', heading: 'Certificates', path: '/acme' },
  { label: 'Storage', heading: 'Storage', path: '/storage' },
  { label: 'Users', heading: 'Users', path: '/users' },
  { label: 'Audit Log', heading: 'Audit Log', path: '/audit' },
  { label: 'Webhooks', heading: 'Webhooks', path: '/webhooks' },
  { label: 'Settings', heading: 'Settings', path: '/settings' },
];

test.describe('navigation', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  for (const item of NAV) {
    test(`opens ${item.label}`, async ({ page }) => {
      await page.getByRole('link', { name: item.label }).click();
      await expect(page).toHaveURL(new RegExp(`${item.path}$`));
      await expect(page.getByRole('heading', { name: item.heading })).toBeVisible();
    });
  }

  test('SPA refresh on a deep route keeps the session', async ({ page }) => {
    await page.goto('/forward');
    await expect(page.getByRole('heading', { name: 'Port Forwarding' })).toBeVisible();
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Port Forwarding' })).toBeVisible();
  });
});
