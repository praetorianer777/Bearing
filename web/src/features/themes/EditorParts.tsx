import { useId, useState } from "react";
import type { ThemeAsset } from "@/api/themes";
import { Field } from "@/components/ui";
import { THEME_COLOUR_PLACEHOLDER, THEME_CURSOR_PX, THEME_MAX_HOTSPOT, THEME_MAX_RADIUS, THEME_SHADOW_PLACEHOLDER } from "@/config";
import { t } from "@/i18n";
import { CURSOR_KINDS, SHADOW_KEYS, TOKEN_GROUPS, TOKEN_NAMES, type ThemeSpec } from "@/lib/theme";
import { AssetSelect, Choice, controlClass, isFont, isImage, NumberField, type Patch, ResetButton, Section } from "./editorKit";

// The editor's tabs for the colours, the type, the shape and the cursors.

type Mode = "light" | "dark";

const tokenLabel = (name: string) => t.themes.tokens[name] ?? name;

/** What the stylesheet gives every token, shown where the theme leaves it alone; read once, not once a field. */
function stockValues(): Record<string, string> {
  const style = getComputedStyle(document.documentElement);
  return Object.fromEntries(TOKEN_NAMES.map((token) => [token, style.getPropertyValue(`--color-${token}`).trim()]));
}

const hex6 = /^#[0-9a-f]{6}$/i;

function ColourField({ token, value, stock, onChange }: { token: string; value: string; stock: string; onChange: (next: string) => void }) {
  const id = useId();
  const label = tokenLabel(token);
  return (
    <div className="flex flex-col gap-1" data-token={token}>
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {label}
      </label>
      <span className="flex items-center gap-2">
        <input
          type="color"
          aria-label={t.themes.editor.pick(label)}
          value={hex6.test(value) ? value : hex6.test(stock) ? stock : THEME_COLOUR_PLACEHOLDER}
          onChange={(event) => onChange(event.target.value)}
          className="h-9 w-10 shrink-0 cursor-pointer rounded-md border border-border-strong bg-surface p-1"
        />
        <input
          id={id}
          value={value}
          placeholder={stock || THEME_COLOUR_PLACEHOLDER}
          onChange={(event) => onChange(event.target.value)}
          spellCheck={false}
          className={`${controlClass} min-w-0 flex-1 font-mono`}
        />
        {value && <ResetButton label={t.themes.editor.resetToken(label)} onClick={() => onChange("")} />}
      </span>
    </div>
  );
}

export function ColoursTab({ spec, patch }: { spec: ThemeSpec; patch: Patch }) {
  const [mode, setMode] = useState<Mode>("light");
  const [stock] = useState(stockValues);
  const set = spec.colors[mode];
  const changed = Object.keys(set).length;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <Choice<Mode>
          label={t.themes.editor.palette}
          value={mode}
          onChange={setMode}
          options={[
            { value: "light", label: t.themes.editor.light },
            { value: "dark", label: t.themes.editor.dark },
          ]}
        />
        <span className="text-sm text-ink-muted">{changed === 0 ? t.themes.editor.nothingChanged : t.themes.editor.changed(changed, TOKEN_NAMES.length)}</span>
      </div>
      {TOKEN_GROUPS.map((group) => (
        <Section key={group.id} title={t.themes.groups[group.id]}>
          <div className="grid gap-3 sm:grid-cols-2">
            {group.tokens.map((token) => (
              <ColourField
                key={`${mode}-${token}`}
                token={token}
                value={set[token] ?? ""}
                stock={stock[token] ?? ""}
                onChange={(next) =>
                  patch((draft) => {
                    if (next.trim() === "") delete draft.colors[mode][token];
                    else draft.colors[mode][token] = next.trim();
                  })
                }
              />
            ))}
          </div>
        </Section>
      ))}
    </div>
  );
}

export function TypeTab({ spec, patch, assets }: { spec: ThemeSpec; patch: Patch; assets: ThemeAsset[] }) {
  const fonts = assets.filter(isFont);
  return (
    <div className="flex flex-col gap-4">
      {(["sans", "mono"] as const).map((role) => {
        const font = spec.fonts[role];
        const family = role === "sans" ? t.themes.editor.textFamily : t.themes.editor.codeFamily;
        return (
          <Section key={role} title={role === "sans" ? t.themes.editor.textFace : t.themes.editor.codeFace}>
            <div className="flex flex-wrap items-end gap-3" data-theme-font={role}>
              <div className="w-64">
                <Field
                  label={family}
                  value={font?.family ?? ""}
                  placeholder={role === "sans" ? "Inter" : "JetBrains Mono"}
                  onChange={(event) =>
                    patch((draft) => {
                      const next = event.target.value;
                      if (next.trim() === "") delete draft.fonts[role];
                      else draft.fonts[role] = { ...draft.fonts[role], family: next };
                    })
                  }
                />
              </div>
              {font && (
                <AssetSelect
                  label={t.themes.editor.fontFile(family)}
                  assets={fonts}
                  value={font.assetId}
                  onChange={(id) =>
                    patch((draft) => {
                      draft.fonts[role] = { family: font.family, ...(id ? { assetId: id } : {}) };
                    })
                  }
                />
              )}
              {font && (
                <ResetButton
                  label={`${t.themes.editor.reset}: ${family}`}
                  onClick={() =>
                    patch((draft) => {
                      delete draft.fonts[role];
                    })
                  }
                />
              )}
            </div>
            <p className="mt-2 text-xs text-ink-subtle">{t.themes.editor.familyHint}</p>
          </Section>
        );
      })}
    </div>
  );
}

export function ShapeTab({ spec, patch }: { spec: ThemeSpec; patch: Patch }) {
  return (
    <div className="flex flex-col gap-4">
      <Section title={t.themes.editor.corners}>
        <div className="flex flex-wrap gap-4">
          {(
            [
              ["radiusControl", t.themes.editor.radiusControl, "6"],
              ["radiusOverlay", t.themes.editor.radiusOverlay, "8"],
            ] as const
          ).map(([key, label, placeholder]) => (
            <NumberField
              key={key}
              label={label}
              value={spec.shape[key]}
              max={THEME_MAX_RADIUS}
              placeholder={placeholder}
              onChange={(v) =>
                patch((draft) => {
                  if (v === undefined) delete draft.shape[key];
                  else draft.shape[key] = v;
                })
              }
            />
          ))}
        </div>
      </Section>
      <Section title={t.themes.editor.shadows}>
        <div className="flex flex-col gap-3">
          {SHADOW_KEYS.map((key) => (
            <Field
              key={key}
              label={t.themes.editor.shadowLabels[key] ?? key}
              value={spec.shadows[key] ?? ""}
              placeholder={THEME_SHADOW_PLACEHOLDER}
              spellCheck={false}
              onChange={(event) =>
                patch((draft) => {
                  if (event.target.value.trim() === "") delete draft.shadows[key];
                  else draft.shadows[key] = event.target.value;
                })
              }
            />
          ))}
        </div>
      </Section>
    </div>
  );
}

export function CursorsTab({ spec, patch, assets }: { spec: ThemeSpec; patch: Patch; assets: ThemeAsset[] }) {
  const images = assets.filter(isImage);
  return (
    <Section>
      <p className="mb-4 text-sm text-ink-muted">{t.themes.editor.cursorsIntro(THEME_CURSOR_PX)}</p>
      <div className="flex flex-col gap-3">
        {CURSOR_KINDS.map((each) => {
          const cursor = spec.cursors[each.kind];
          const label = t.themes.cursors[each.kind] ?? each.kind;
          return (
            <div key={each.kind} className="flex flex-wrap items-end gap-3" data-theme-cursor={each.kind}>
              <AssetSelect
                label={t.themes.editor.cursorPicture(label)}
                assets={images}
                value={cursor?.assetId}
                onChange={(id) =>
                  patch((draft) => {
                    if (!id) delete draft.cursors[each.kind];
                    else draft.cursors[each.kind] = { assetId: id, hotspotX: cursor?.hotspotX ?? 0, hotspotY: cursor?.hotspotY ?? 0 };
                  })
                }
              />
              {cursor &&
                (["hotspotX", "hotspotY"] as const).map((axis) => (
                  <NumberField
                    key={axis}
                    label={axis === "hotspotX" ? t.themes.editor.pointX : t.themes.editor.pointY}
                    value={cursor[axis]}
                    max={THEME_MAX_HOTSPOT}
                    placeholder="0"
                    onChange={(v) =>
                      patch((draft) => {
                        draft.cursors[each.kind] = { ...cursor, [axis]: v ?? 0 };
                      })
                    }
                  />
                ))}
            </div>
          );
        })}
      </div>
    </Section>
  );
}
