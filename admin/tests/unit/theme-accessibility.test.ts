import { accessibleThemeToken } from '@root/config/accessibleTheme';

const relativeLuminance = (hex: string) => {
  const channels = hex
    .slice(1)
    .match(/.{2}/g)
    ?.map((value) => Number.parseInt(value, 16) / 255)
    .map((value) =>
      value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4,
    );

  if (channels?.length !== 3) {
    throw new Error(`不支持的颜色值：${hex}`);
  }

  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
};

const contrastRatio = (foreground: string, background: string) => {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);

  return (lighter + 0.05) / (darker + 0.05);
};

describe('管理端无障碍主题', () => {
  it('为正文、占位符、主按钮和成功状态保持 WCAG AA 对比度', () => {
    expect(
      contrastRatio(accessibleThemeToken.colorTextSecondary, '#ffffff'),
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio(accessibleThemeToken.colorTextDescription, '#ffffff'),
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio(accessibleThemeToken.colorTextPlaceholder, '#f5f5f5'),
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio('#ffffff', accessibleThemeToken.colorPrimary),
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio(
        accessibleThemeToken.colorSuccess,
        accessibleThemeToken.colorSuccessBg,
      ),
    ).toBeGreaterThanOrEqual(4.5);
  });
});
