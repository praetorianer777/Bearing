import { API_BASE } from "@/config";

// A theme compiles to one unlayered stylesheet, beating the stylesheet's @layer theme, of only what the theme names,
// loading nothing but its own files and inline images: Exciter's compiler, itself Armature's, output for output.

/** The colour tokens a theme may redefine. A test keeps the list equal to the --color-* names in the stylesheet. */
export const TOKEN_NAMES = [
  "canvas",
  "surface",
  "surface-raised",
  "surface-sunken",
  "surface-overlay",
  "surface-glass",
  "backdrop-from",
  "backdrop-to",
  "border",
  "border-strong",
  "ink",
  "ink-muted",
  "ink-subtle",
  "ink-disabled",
  "primary",
  "primary-hover",
  "on-primary",
  "accent",
  "accent-hover",
  "accent-subtle",
  "on-accent",
  "focus",
  "selection",
  "danger",
  "danger-hover",
  "danger-subtle",
  "on-danger",
  "success",
  "success-subtle",
  "warning",
  "warning-subtle",
  "epic",
  "status-todo",
  "status-progress",
  "status-done",
  "status-todo-subtle",
  "status-progress-subtle",
  "status-done-subtle",
  "chart-1",
  "chart-2",
  "chart-3",
  "chart-4",
  "chart-5",
  "chart-6",
];

/** The pointers a theme may replace, with the CSS keyword and what each lands on. */
export const CURSOR_KINDS: Array<{ kind: string; keyword: string; selector: string }> = [
  { kind: "default", keyword: "default", selector: ":root, body" },
  { kind: "pointer", keyword: "pointer", selector: 'a, button, [role="button"], [role="tab"], [role="option"], label, select, summary, .cursor-pointer' },
  {
    kind: "text",
    keyword: "text",
    selector: 'input:not([type="checkbox"]):not([type="radio"]):not([type="file"]), textarea, [contenteditable="true"], .cursor-text',
  },
  { kind: "grab", keyword: "grab", selector: ".cursor-grab" },
  { kind: "grabbing", keyword: "grabbing", selector: ".cursor-grabbing" },
  { kind: "move", keyword: "move", selector: ".cursor-move" },
  { kind: "notAllowed", keyword: "not-allowed", selector: ':disabled, [aria-disabled="true"], .cursor-not-allowed' },
  { kind: "wait", keyword: "wait", selector: ".cursor-wait, .cursor-progress" },
];

const SHADOW_KEYS = new Set(["1", "2", "3"]);
const ICON_STROKE = 1.5;

export interface ThemeSpec {
  colors: { light: Record<string, string>; dark: Record<string, string> };
  fonts: { sans?: { family: string; assetId?: string }; mono?: { family: string; assetId?: string } };
  shape: { radiusControl?: number; radiusOverlay?: number };
  shadows: Record<string, string>;
  cursors: Record<string, { assetId: string; hotspotX: number; hotspotY: number }>;
  icons: Record<string, { assetId?: string; paths?: string[] }>;
  backdrop?: { assetId: string; fit: string } | null;
  css: string;
}

export interface CompilableTheme {
  id: string;
  spec: ThemeSpec;
  assets: { id: string; contentType: string }[];
}

/** Where a theme's file is served from; the id never changes, so the browser keeps it. */
export function themeAssetURL(themeId: string, assetId: string): string {
  return `${API_BASE}/themes/${themeId}/assets/${assetId}`;
}

const known = new Set(TOKEN_NAMES);

function declarations(set: Record<string, string>): string {
  return Object.entries(set)
    .filter(([name]) => known.has(name))
    .map(([name, value]) => `  --color-${name}: ${value};`)
    .join("\n");
}

function url(href: string): string {
  return `url(${JSON.stringify(href)})`;
}

/** Path data drawn on the 16px icon grid, as an inline image a mask can read. */
function pathsAsImage(paths: string[]): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="black" stroke-width="${ICON_STROKE}" stroke-linecap="round" stroke-linejoin="round">${paths.map((d) => `<path d="${d}"/>`).join("")}</svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

/** The stylesheet a theme is. */
export function compileTheme(theme: CompilableTheme): string {
  const { spec } = theme;
  const asset = (id: string) => themeAssetURL(theme.id, id);
  const assetType = (id: string) => theme.assets.find((a) => a.id === id)?.contentType ?? "";
  const out: string[] = [];

  const light = declarations(spec.colors.light);
  if (light) {
    out.push(`:root[data-theme="light"] {\n${light}\n}`);
    out.push(`@media (prefers-color-scheme: light) {\n:root:not([data-theme="dark"]) {\n${light}\n}\n}`);
  }
  const dark = declarations(spec.colors.dark);
  if (dark) {
    out.push(`:root[data-theme="dark"] {\n${dark}\n}`);
    out.push(`@media (prefers-color-scheme: dark) {\n:root:not([data-theme="light"]) {\n${dark}\n}\n}`);
  }

  const root: string[] = [];
  for (const [role, fallback] of [
    ["sans", "ui-sans-serif, system-ui, sans-serif"],
    ["mono", "ui-monospace, monospace"],
  ] as const) {
    const font = spec.fonts[role];
    if (!font) continue;
    if (font.assetId) {
      const format = assetType(font.assetId) === "font/woff2" ? "woff2" : "woff";
      out.push(
        `@font-face {\n  font-family: ${JSON.stringify(font.family)};\n  src: ${url(asset(font.assetId))} format("${format}");\n  font-display: swap;\n}`,
      );
    }
    root.push(`  --font-${role}: ${JSON.stringify(font.family)}, ${fallback};`);
  }
  if (spec.shape.radiusControl !== undefined) root.push(`  --radius-control: ${spec.shape.radiusControl}px;`);
  if (spec.shape.radiusOverlay !== undefined) root.push(`  --radius-overlay: ${spec.shape.radiusOverlay}px;`);
  for (const [key, value] of Object.entries(spec.shadows)) {
    if (SHADOW_KEYS.has(key)) root.push(`  --shadow-${key}: ${value};`);
  }
  if (root.length) out.push(`:root {\n${root.join("\n")}\n}`);

  for (const each of CURSOR_KINDS) {
    const cursor = spec.cursors[each.kind];
    if (!cursor) continue;
    out.push(`${each.selector} {\n  cursor: ${url(asset(cursor.assetId))} ${cursor.hotspotX} ${cursor.hotspotY}, ${each.keyword};\n}`);
  }

  for (const [name, icon] of Object.entries(spec.icons)) {
    const image = icon.assetId ? asset(icon.assetId) : icon.paths?.length ? pathsAsImage(icon.paths) : null;
    if (!image) continue;
    const mask = `${url(image)} center / contain no-repeat`;
    out.push(`svg[data-icon=${JSON.stringify(name)}] > path {\n  display: none;\n}`);
    out.push(`svg[data-icon=${JSON.stringify(name)}] {\n  background-color: currentColor;\n  -webkit-mask: ${mask};\n  mask: ${mask};\n}`);
  }

  if (spec.backdrop) {
    const image = url(asset(spec.backdrop.assetId));
    out.push(
      spec.backdrop.fit === "tile"
        ? `.bg-backdrop {\n  background: ${image} repeat;\n}`
        : `.bg-backdrop {\n  background: ${image} center / cover no-repeat;\n}`,
    );
  }

  if (spec.css.trim()) out.push(spec.css.trim());
  return out.join("\n\n");
}
