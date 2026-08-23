import defaultSettings from '@root/config/defaultSettings';
import routes from '@root/config/routes';
import { render, screen } from '@testing-library/react';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import Footer from '@/components/Footer';

describe('admin production shell accessibility', () => {
  beforeAll(() => {
    vi.stubGlobal('__APP_VERSION__', 'test');
    vi.stubGlobal('__UMI_VERSION__', 'test');
    vi.stubGlobal('__UTOO_VERSION__', 'test');
  });

  it('uses one non-fixed top navigation and only local branding assets', () => {
    expect(defaultSettings.layout).toBe('top');
    expect(defaultSettings.fixedHeader).toBe(false);
    expect(defaultSettings.logo).toBe('/logo.svg');
  });

  it('exposes the footer as a contentinfo landmark', () => {
    render(<Footer />);

    expect(screen.getByRole('contentinfo')).toBeInTheDocument();
  });

  it('renders account settings as a direct navigation item', () => {
    const accountSettings = routes.find(
      (route) => route.path === '/account/settings',
    );

    expect(accountSettings).toMatchObject({
      name: '账号',
      component: './account/settings',
    });
    expect(accountSettings).not.toHaveProperty('routes');
  });

  it('sets the document language and preserves browser zoom', async () => {
    document.documentElement.removeAttribute('lang');
    let viewport = document.querySelector<HTMLMetaElement>(
      'meta[name="viewport"]',
    );
    if (!viewport) {
      viewport = document.createElement('meta');
      viewport.name = 'viewport';
      document.head.append(viewport);
    }
    viewport.content = 'width=device-width, user-scalable=no';

    await import('@/global');

    expect(document.documentElement.lang).toBe('zh-CN');
    expect(viewport.content).toBe('width=device-width, initial-scale=1');
  });
});
