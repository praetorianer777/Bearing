import { useRef, useState } from "react";
import { type Theme, type ThemeAsset, useAddThemeAsset, useRemoveThemeAsset } from "@/api/themes";
import { ICONS } from "@/components/icons";
import { Button, Field, FormError } from "@/components/ui";
import { KILOBYTE, THEME_ASSET_MAX_BYTES, THEME_CSS_MAX_BYTES } from "@/config";
import { t } from "@/i18n";
import type { ThemeSpec } from "@/lib/theme";
import { AssetSelect, Choice, isImage, type Patch, ResetButton, Section, TextArea } from "./editorKit";

// The editor's tabs for the icons, the backdrop, the theme's files and the extra CSS.

type Effect = "none" | "constellation" | "confetti";

const UPLOAD_ACCEPT = "image/png,image/jpeg,image/webp,image/gif,image/svg+xml,.svg,.woff,.woff2,font/woff,font/woff2";

const glyphs = Object.entries(ICONS)
  .map(([name, Glyph]) => ({ name, Glyph }))
  .sort((a, b) => a.name.localeCompare(b.name));

export function IconsTab({ spec, patch, assets }: { spec: ThemeSpec; patch: Patch; assets: ThemeAsset[] }) {
  const images = assets.filter(isImage);
  const [query, setQuery] = useState("");
  const shown = glyphs.filter((g) => g.name.includes(query.trim().toLowerCase()));
  return (
    <Section>
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="w-56">
          <Field label={t.themes.editor.findIcon} type="search" value={query} onChange={(event) => setQuery(event.target.value)} />
        </div>
        <p className="flex-1 text-sm text-ink-muted">{t.themes.editor.iconsIntro}</p>
      </div>
      <div className="divide-y divide-border">
        {shown.map(({ name, Glyph }) => (
          <IconRow key={name} name={name} Glyph={Glyph} icon={spec.icons[name]} images={images} patch={patch} />
        ))}
      </div>
    </Section>
  );
}

// The path data is kept as typed, so a space at the end of a line survives until the next keystroke; the spec
// takes the lines trimmed.
function IconRow({
  name,
  Glyph,
  icon,
  images,
  patch,
}: {
  name: string;
  Glyph: (typeof glyphs)[number]["Glyph"];
  icon: ThemeSpec["icons"][string] | undefined;
  images: ThemeAsset[];
  patch: Patch;
}) {
  const [typed, setTyped] = useState(icon?.paths?.join("\n") ?? "");
  const paths = icon?.paths ?? [];
  const shown =
    typed
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .join("\n") === paths.join("\n")
      ? typed
      : paths.join("\n");
  return (
    <div className="flex flex-wrap items-start gap-3 py-3" data-theme-icon={name}>
      <span className="flex w-32 items-center gap-2 pt-7 text-sm text-ink">
        <Glyph />
        {name}
      </span>
      <AssetSelect
        label={t.themes.editor.iconPicture(name)}
        assets={images}
        value={icon?.assetId}
        onChange={(id) =>
          patch((draft) => {
            if (!id && paths.length === 0) delete draft.icons[name];
            else draft.icons[name] = { ...(id ? { assetId: id } : {}), ...(paths.length ? { paths } : {}) };
          })
        }
      />
      <div className="w-72">
        <TextArea
          label={t.themes.editor.iconPaths(name)}
          rows={2}
          value={shown}
          placeholder="M2 8h12"
          spellCheck={false}
          onChange={(event) => {
            setTyped(event.target.value);
            const next = event.target.value
              .split("\n")
              .map((line) => line.trim())
              .filter(Boolean);
            patch((draft) => {
              if (next.length === 0 && !icon?.assetId) delete draft.icons[name];
              else draft.icons[name] = { ...(icon?.assetId ? { assetId: icon.assetId } : {}), ...(next.length ? { paths: next } : {}) };
            });
          }}
        />
      </div>
      {icon && (
        <span className="pt-6">
          <ResetButton
            label={`${t.themes.editor.reset}: ${name}`}
            onClick={() => {
              setTyped("");
              patch((draft) => {
                delete draft.icons[name];
              });
            }}
          />
        </span>
      )}
    </div>
  );
}

export function BackdropTab({ spec, patch, assets }: { spec: ThemeSpec; patch: Patch; assets: ThemeAsset[] }) {
  const backdrop = spec.backdrop ?? null;
  const effect: Effect = spec.effect === "constellation" || spec.effect === "confetti" ? spec.effect : "none";
  return (
    <div className="flex flex-col gap-4">
      <Section title={t.themes.editor.backdropPicture}>
        <p className="mb-4 text-sm text-ink-muted">{t.themes.editor.backdropIntro}</p>
        <div className="flex flex-wrap items-end gap-4">
          <AssetSelect
            label={t.themes.editor.picture}
            assets={assets.filter(isImage)}
            value={backdrop?.assetId}
            onChange={(id) =>
              patch((draft) => {
                draft.backdrop = id ? { assetId: id, fit: backdrop?.fit ?? "cover" } : null;
              })
            }
          />
          {backdrop && (
            <Choice<string>
              label={t.themes.editor.fit}
              value={backdrop.fit}
              onChange={(fit) =>
                patch((draft) => {
                  draft.backdrop = { ...backdrop, fit };
                })
              }
              options={[
                { value: "cover", label: t.themes.editor.fitCover },
                { value: "tile", label: t.themes.editor.fitTile },
              ]}
            />
          )}
        </div>
      </Section>
      <Section title={t.themes.editor.effect}>
        <p className="mb-3 text-sm text-ink-muted">{t.themes.editor.effectIntro}</p>
        <Choice<Effect>
          label={t.themes.editor.effect}
          value={effect}
          onChange={(next) =>
            patch((draft) => {
              if (next === "none") delete draft.effect;
              else draft.effect = next;
            })
          }
          options={[
            { value: "none", label: t.themes.editor.effectNone },
            { value: "constellation", label: t.themes.editor.effectConstellation },
            { value: "confetti", label: t.themes.editor.effectConfetti },
          ]}
        />
      </Section>
    </div>
  );
}

export function FilesTab({ theme, spec, onNotice }: { theme?: Theme; spec: ThemeSpec; onNotice: (notice: string) => void }) {
  const add = useAddThemeAsset();
  const remove = useRemoveThemeAsset();
  const input = useRef<HTMLInputElement>(null);
  const [tooBig, setTooBig] = useState(false);
  if (!theme) {
    return (
      <Section>
        <p className="text-sm text-ink-muted">{t.themes.editor.saveFirst}</p>
      </Section>
    );
  }
  const used = new Set<string>(
    [
      spec.fonts.sans?.assetId,
      spec.fonts.mono?.assetId,
      spec.backdrop?.assetId,
      ...Object.values(spec.cursors).map((c) => c.assetId),
      ...Object.values(spec.icons).map((i) => i.assetId),
    ].filter((id): id is string => Boolean(id)),
  );
  const choose = (file: File | undefined) => {
    if (input.current) input.current.value = "";
    if (!file) return;
    setTooBig(file.size > THEME_ASSET_MAX_BYTES);
    if (file.size > THEME_ASSET_MAX_BYTES) return;
    add.mutate({ id: theme.id, file }, { onSuccess: (made) => onNotice(t.themes.editor.added(made.name)) });
  };
  return (
    <Section>
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="secondary" onClick={() => input.current?.click()} disabled={add.isPending}>
          {t.themes.editor.addFile}
        </Button>
        <input
          ref={input}
          type="file"
          accept={UPLOAD_ACCEPT}
          className="sr-only"
          tabIndex={-1}
          aria-label={t.themes.editor.addFile}
          onChange={(event) => choose(event.target.files?.[0])}
        />
        <p className="text-sm text-ink-subtle">{t.themes.editor.filesHint}</p>
      </div>
      <div className="mt-3 flex flex-col gap-2">
        {tooBig && (
          <p role="alert" className="rounded-md bg-danger-subtle px-3 py-2 text-sm text-danger">
            {t.themes.editor.tooBig}
          </p>
        )}
        <FormError error={add.error ?? remove.error} />
      </div>
      {theme.assets.length === 0 ? (
        <p className="mt-3 text-sm text-ink-muted">{t.themes.editor.noFilesYet}</p>
      ) : (
        <ul aria-label={t.themes.editor.tabFiles} className="mt-3 divide-y divide-border">
          {theme.assets.map((a) => (
            <li key={a.id} className="flex items-center gap-3 py-2 text-sm">
              <span className="min-w-0 flex-1 truncate text-ink">{a.name}</span>
              <span className="text-ink-subtle">{a.contentType}</span>
              <span className="text-ink-subtle tabular-nums">{t.themes.editor.kilobytes(Math.max(1, Math.round(a.size / KILOBYTE)))}</span>
              {used.has(a.id) && <span className="text-xs text-ink-muted">{t.themes.editor.inUse}</span>}
              <Button
                variant="danger"
                disabled={used.has(a.id) || remove.isPending}
                onClick={() => remove.mutate({ id: theme.id, assetId: a.id }, { onSuccess: () => onNotice(t.themes.editor.removed(a.name)) })}
                aria-label={t.themes.editor.removeFile(a.name)}
              >
                {t.themes.editor.remove}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}

export function AdvancedTab({ spec, patch }: { spec: ThemeSpec; patch: Patch }) {
  const size = new TextEncoder().encode(spec.css).length;
  return (
    <Section>
      <TextArea
        label={t.themes.editor.extraCSS}
        rows={12}
        value={spec.css}
        spellCheck={false}
        hint={t.themes.editor.cssHint(Math.round(size / KILOBYTE), THEME_CSS_MAX_BYTES / KILOBYTE)}
        onChange={(event) =>
          patch((draft) => {
            draft.css = event.target.value;
          })
        }
      />
    </Section>
  );
}
