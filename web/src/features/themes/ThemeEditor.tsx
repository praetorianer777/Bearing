import { Link, useNavigate } from "@tanstack/react-router";
import { type FormEvent, useEffect, useId, useMemo, useState } from "react";
import { type Theme, type ThemeExample, useChooseTheme, useCreateTheme, useTheme, useThemeExamples, useUpdateTheme } from "@/api/themes";
import { Button, Field, fieldError, FormError } from "@/components/ui";
import { THEME_PREVIEW_DEBOUNCE_MS } from "@/config";
import { t, useLanguage } from "@/i18n";
import { compileTheme, emptySpec, type ThemeSpec } from "@/lib/theme";
import { AdvancedTab, BackdropTab, FilesTab, IconsTab } from "./EditorAssets";
import { ColoursTab, CursorsTab, ShapeTab, TypeTab } from "./EditorParts";
import { setThemePreview } from "./ThemeLoader";

const TABS = ["colours", "type", "shape", "cursors", "icons", "backdrop", "files", "advanced"] as const;
type Tab = (typeof TABS)[number];

const tabLabel = (tab: Tab) =>
  ({
    colours: t.themes.editor.tabColours,
    type: t.themes.editor.tabType,
    shape: t.themes.editor.tabShape,
    cursors: t.themes.editor.tabCursors,
    icons: t.themes.editor.tabIcons,
    backdrop: t.themes.editor.tabBackdrop,
    files: t.themes.editor.tabFiles,
    advanced: t.themes.editor.tabAdvanced,
  })[tab];

/** One theme, part by part. Nothing is written until Save; Preview shows the draft on this very page, and leaving
 * the page shows the chosen theme again. */
export function ThemeEditor({ theme }: { theme?: Theme }) {
  const ids = useId();
  const navigate = useNavigate();
  const create = useCreateTheme();
  const update = useUpdateTheme();
  const choose = useChooseTheme();
  const [name, setName] = useState(theme?.name ?? "");
  const [shared, setShared] = useState(theme?.shared ?? false);
  const [spec, setSpec] = useState<ThemeSpec>(() => (theme ? structuredClone(theme.spec) : emptySpec()));
  const [tab, setTab] = useState<Tab>("colours");
  const [preview, setPreview] = useState(false);
  const [startedFrom, setStartedFrom] = useState("blank");
  const [notice, setNotice] = useState("");
  const assets = useMemo(() => theme?.assets ?? [], [theme?.assets]);
  const patch = (change: (draft: ThemeSpec) => void) =>
    setSpec((current) => {
      const draft = structuredClone(current);
      change(draft);
      return draft;
    });

  // The draft is compiled once it settles, so typing a colour does not restyle the page on every keystroke.
  useEffect(() => {
    if (!preview) {
      setThemePreview(null);
      return;
    }
    const timer = window.setTimeout(
      () => setThemePreview(compileTheme({ id: theme?.id ?? "new", spec, assets }), spec.effect ?? null),
      THEME_PREVIEW_DEBOUNCE_MS,
    );
    return () => window.clearTimeout(timer);
  }, [preview, spec, theme?.id, assets]);
  useEffect(() => () => setThemePreview(null), []);

  const save = (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim()) return;
    setNotice("");
    if (theme) update.mutate({ id: theme.id, name: name.trim(), shared, spec }, { onSuccess: (saved) => setNotice(t.themes.saved(saved.name)) });
    else
      create.mutate({ name: name.trim(), shared, spec }, { onSuccess: (made) => navigate({ to: "/settings/themes/$themeId", params: { themeId: made.id } }) });
  };

  const failure = create.error ?? update.error ?? choose.error;
  return (
    <form onSubmit={save} noValidate className="flex flex-col gap-4">
      <FormError error={failure} />
      {notice && (
        <p role="status" className="text-sm text-ink-muted">
          {notice}
        </p>
      )}
      <div className="flex flex-wrap items-end gap-4 rounded-xl border border-border bg-surface p-4">
        <div className="w-64">
          <Field label={t.themes.editor.name} value={name} onChange={(event) => setName(event.target.value)} required error={fieldError(failure, "name")} />
        </div>
        <label className="flex h-9 items-center gap-2 text-sm text-ink">
          <input type="checkbox" checked={shared} onChange={(event) => setShared(event.target.checked)} />
          {t.themes.editor.shared}
        </label>
        <label className="flex h-9 items-center gap-2 text-sm text-ink">
          <input type="checkbox" checked={preview} onChange={(event) => setPreview(event.target.checked)} />
          {t.themes.editor.preview}
        </label>
        <span className="ml-auto flex gap-2">
          {theme && !theme.active && (
            <Button
              variant="secondary"
              disabled={choose.isPending}
              onClick={() => choose.mutate(theme.id, { onSuccess: () => setNotice(t.themes.nowUsing(theme.name)) })}
            >
              {t.themes.use}
            </Button>
          )}
          <Button type="submit" disabled={!name.trim() || create.isPending || update.isPending}>
            {t.themes.editor.save}
          </Button>
        </span>
      </div>

      {!theme && (
        <StartFrom
          chosen={startedFrom}
          onChoose={(example) => {
            setStartedFrom(example?.key ?? "blank");
            setSpec(example ? structuredClone(example.spec) : emptySpec());
            if (example && !name.trim()) setName(example.name);
          }}
        />
      )}

      <div role="tablist" aria-label={t.themes.editor.tabs} className="flex flex-wrap gap-1 border-b border-border">
        {TABS.map((each) => (
          <button
            key={each}
            type="button"
            role="tab"
            id={`${ids}-${each}`}
            aria-selected={tab === each}
            aria-controls={`${ids}-panel`}
            tabIndex={tab === each ? 0 : -1}
            onClick={() => setTab(each)}
            onKeyDown={(event) => {
              const step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
              if (!step) return;
              const next = TABS[(TABS.indexOf(each) + step + TABS.length) % TABS.length] ?? each;
              setTab(next);
              document.getElementById(`${ids}-${next}`)?.focus();
            }}
            className="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-ink-muted hover:text-ink aria-selected:border-ink aria-selected:text-ink"
          >
            {tabLabel(each)}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`${ids}-panel`} aria-labelledby={`${ids}-${tab}`}>
        {tab === "colours" && <ColoursTab spec={spec} patch={patch} />}
        {tab === "type" && <TypeTab spec={spec} patch={patch} assets={assets} />}
        {tab === "shape" && <ShapeTab spec={spec} patch={patch} />}
        {tab === "cursors" && <CursorsTab spec={spec} patch={patch} assets={assets} />}
        {tab === "icons" && <IconsTab spec={spec} patch={patch} assets={assets} />}
        {tab === "backdrop" && <BackdropTab spec={spec} patch={patch} assets={assets} />}
        {tab === "files" && <FilesTab theme={theme} spec={spec} onNotice={setNotice} />}
        {tab === "advanced" && <AdvancedTab spec={spec} patch={patch} />}
      </div>
    </form>
  );
}

// A new theme starts blank or from one Bearing ships; picking one replaces the whole draft, so it comes before
// anything is typed.
function StartFrom({ chosen, onChoose }: { chosen: string; onChoose: (example: ThemeExample | null) => void }) {
  const examples = useThemeExamples().data ?? [];
  const name = useId();
  if (examples.length === 0) return null;
  const options = [
    { key: "blank", name: t.themes.editor.blank, description: t.themes.editor.blankBody, example: null },
    ...examples.map((e) => ({ ...e, example: e })),
  ];
  return (
    <fieldset className="rounded-xl border border-border bg-surface p-4">
      <legend className="px-1 text-sm font-semibold text-ink">{t.themes.editor.startFrom}</legend>
      <div className="grid gap-3 sm:grid-cols-3">
        {options.map((option) => (
          <label
            key={option.key}
            className="flex cursor-pointer flex-col gap-1 rounded-lg border border-border-strong p-3 has-checked:border-accent has-checked:bg-accent-subtle"
          >
            <span className="flex items-center gap-2 text-sm font-medium text-ink">
              <input
                type="radio"
                name={name}
                checked={chosen === option.key}
                onChange={() => onChoose(option.example)}
                aria-labelledby={`${name}-${option.key}`}
                aria-describedby={`${name}-${option.key}-about`}
              />
              <span id={`${name}-${option.key}`}>{option.name}</span>
            </span>
            <span id={`${name}-${option.key}-about`} className="text-xs text-ink-muted">
              {option.description}
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}

/** The editor's page: a new theme, or one of the reader's by its id. */
export function ThemeEditorPage({ themeId }: { themeId?: string }) {
  useLanguage();
  const found = useTheme(themeId);
  const title = themeId ? (found.data?.name ?? t.themes.editTheme) : t.themes.newTheme;
  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-4 px-6 py-8">
      <Link to="/settings/themes" className="text-sm text-accent hover:underline">
        {t.themes.editor.back}
      </Link>
      <h1 className="text-2xl font-semibold">{title}</h1>
      {themeId && found.isError && <FormError error={found.error} />}
      {(!themeId || found.data) && <ThemeEditor key={found.data?.id ?? "new"} theme={found.data} />}
    </div>
  );
}
