# 织光品牌资源

静态品牌资源统一放在本目录，公共 URL 以 `/brand/` 开头。

| 文件                         | 用途                                             |
| ---------------------------- | ------------------------------------------------ |
| `logo.svg`                   | 品牌矢量图形与现代浏览器 favicon，支持系统深浅色 |
| `icons/favicon.ico`          | 16、32、48px 浏览器兼容图标                      |
| `icons/apple-touch-icon.png` | 180px Apple 主屏幕图标                           |
| `icons/icon-192.png`         | 192px 网站图标                                   |
| `icons/icon-512.png`         | 512px 网站图标，留白支持系统蒙版裁切             |

页面内的 Logo 使用 `src/components/brand/brand.tsx` 中的 React SVG 组件，保持主题色与单色版本能力。修改图形时，同步组件与 `logo.svg` 的路径，并从矢量图形重新导出兼容图标。

图标声明在 `src/app/layout.tsx`，网站清单在 `src/app/manifest.ts`。`next.config.ts` 将浏览器默认的 `/favicon.ico` 请求映射到本目录，避免维护重复文件。
