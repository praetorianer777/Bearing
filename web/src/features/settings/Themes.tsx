import { Link, useNavigate } from "@tanstack/react-router";
import { type ChangeEvent, useEffect, useId, useRef, useState } from "react";
import { useMe } from "@/api/auth";
import {
  type Theme,
  type ThemePackage,
  themeExportURL,
  useActiveTheme,
  useChooseTheme,
  useDeleteTheme,
  useImportTheme,
  useSetDefaultTheme,
  useThemes,
  useUpdateTheme,
} from "@/api/themes";
import { Button, FormError } from "@/components/ui";
import { Choice } from "@/features/themes/editorKit";
import { t, useLanguage } from "@/i18n";
import { SettingsNav } from "./SettingsNav";

type View = "mine" | "shared";

/** Which themes a view shows: the reader's own, or what others shared. */
export function inView(theme: Theme, view: View, me: string | undefined): boolean {
  return view === "mine" ? theme.ownerId === me : theme.shared && theme.ownerId !== me;
}

/** What the page says about the theme the reader sees. */
export function activeSentence(seen: Theme | null, source: string, everybodys: Theme | undefined): string {
  if (seen && source === "everyone") return t.themes.usingDefault(seen.name);
  if (seen) return t.themes.using(seen.name);
  if (everybodys) return t.themes.usingBuiltInOverDefault(everybodys.name);
  return t.themes.usingBuiltIn;
}

// FileReader rather than file.text(), which the test DOM does not have.
function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

type MenuItem = { label: string; onSelect: () => void; disabled?: boolean; danger?: boolean };

/** A button that opens a list of what may be done to one row; Escape or a click elsewhere closes it. */
function RowMenu({ label, items }: { label: string; items: MenuItem[] }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const menuId = useId();
  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent ? event.key === "Escape" : !root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);
  return (
    <div ref={root} className="relative inline-block text-left">
      <button
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((was) => !was)}
        className="flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-surface-raised hover:text-ink"
      >
        <span aria-hidden="true" className="text-lg leading-none">
          &#x22EF;
        </span>
      </button>
      {open && (
        <div
          id={menuId}
          role="menu"
          aria-label={label}
          className="absolute right-0 z-20 mt-1 w-60 rounded-lg border border-border bg-surface-overlay p-1 shadow-2"
        >
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              onClick={() => {
                setOpen(false);
                item.onSelect();
              }}
              className={`block w-full rounded-md px-2.5 py-1.5 text-left text-sm hover:bg-surface-raised disabled:opacity-50 ${item.danger ? "text-danger" : "text-ink"}`}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// The page owns the mutations rather than each row: a row deleted, or moved to the other view, unmounts when the
// list comes back, and a mutation's own callbacks do not run once its component is gone.
type RowActions = {
  choose: ReturnType<typeof useChooseTheme>;
  setDefault: ReturnType<typeof useSetDefaultTheme>;
  update: ReturnType<typeof useUpdateTheme>;
  remove: ReturnType<typeof useDeleteTheme>;
};

function ThemeRow({
  theme,
  me,
  administers,
  actions: { choose, setDefault, update, remove },
  onNotice,
}: {
  theme: Theme;
  me?: string;
  administers: boolean;
  actions: RowActions;
  onNotice: (notice: string) => void;
}) {
  const navigate = useNavigate();
  const editable = theme.ownerId === me || (administers && theme.shared);
  const items: MenuItem[] = [
    theme.active
      ? { label: t.themes.stopUsing, onSelect: () => choose.mutate(null, { onSuccess: () => onNotice(t.themes.backToDefault) }) }
      : { label: t.themes.use, onSelect: () => choose.mutate(theme.id, { onSuccess: () => onNotice(t.themes.nowUsing(theme.name)) }) },
    ...(administers && theme.shared
      ? [
          theme.default
            ? { label: t.themes.undefault, onSelect: () => setDefault.mutate(null, { onSuccess: () => onNotice(t.themes.defaultCleared) }) }
            : { label: t.themes.makeDefault, onSelect: () => setDefault.mutate(theme.id, { onSuccess: () => onNotice(t.themes.defaultSet(theme.name)) }) },
        ]
      : []),
    { label: t.themes.edit, disabled: !editable, onSelect: () => navigate({ to: "/settings/themes/$themeId", params: { themeId: theme.id } }) },
    {
      label: t.themes.exportFile,
      onSelect: () => {
        window.location.href = themeExportURL(theme.id);
      },
    },
    {
      label: theme.shared ? t.themes.stopSharing : t.themes.share,
      disabled: !editable,
      onSelect: () =>
        update.mutate(
          { id: theme.id, shared: !theme.shared },
          { onSuccess: () => onNotice(theme.shared ? t.themes.privateNow(theme.name) : t.themes.sharedNow(theme.name)) },
        ),
    },
    {
      label: t.themes.delete,
      danger: true,
      disabled: !editable,
      onSelect: () => {
        if (window.confirm(t.themes.confirmDelete(theme.name, theme.inUse)))
          remove.mutate(theme.id, { onSuccess: () => onNotice(t.themes.deleted(theme.name)) });
      },
    },
  ];
  return (
    <tr className="border-b border-border last:border-b-0">
      <td className="py-2.5 pr-3">
        <span className="flex flex-wrap items-center gap-2">
          <span className="flex gap-1" aria-hidden="true">
            {["canvas", "primary", "accent"].map((token) => (
              <span
                key={token}
                className="size-3.5 rounded-full border border-border"
                style={{ backgroundColor: theme.spec.colors.light[token] ?? `var(--color-${token})` }}
              />
            ))}
          </span>
          {editable ? (
            <Link to="/settings/themes/$themeId" params={{ themeId: theme.id }} className="font-medium text-ink hover:underline">
              {theme.name}
            </Link>
          ) : (
            <span className="font-medium text-ink">{theme.name}</span>
          )}
          {theme.shared && <span className="rounded bg-surface-raised px-1.5 text-xs text-ink-muted">{t.themes.tagShared}</span>}
          {theme.default && <span className="rounded bg-surface-raised px-1.5 text-xs text-ink-muted">{t.themes.tagDefault}</span>}
          {theme.active && <span className="rounded bg-accent-subtle px-1.5 text-xs text-ink">{t.themes.tagInUse}</span>}
        </span>
      </td>
      <td className="py-2.5 pr-3 text-ink-muted">{theme.ownerId === me ? t.themes.you : theme.ownerName}</td>
      <td className="py-2.5 pr-3 text-ink-muted tabular-nums">{t.themes.people(theme.inUse)}</td>
      <td className="py-2.5 text-right">
        <RowMenu label={t.themes.actionsFor(theme.name)} items={items} />
      </td>
    </tr>
  );
}

function ImportButton({ onNotice }: { onNotice: (notice: string) => void }) {
  const importTheme = useImportTheme();
  const input = useRef<HTMLInputElement>(null);
  const [unreadable, setUnreadable] = useState(false);
  const choose = async (event: ChangeEvent<HTMLInputElement>) => {
    const field = event.currentTarget;
    const file = field.files?.[0];
    if (!file) return;
    let pkg: ThemePackage;
    try {
      pkg = JSON.parse(await readText(file));
      setUnreadable(false);
    } catch {
      setUnreadable(true);
      field.value = "";
      return;
    }
    importTheme.mutate(pkg, { onSuccess: (made) => onNotice(t.themes.imported(made.name)), onSettled: () => (field.value = "") });
  };
  return (
    <>
      <Button variant="secondary" onClick={() => input.current?.click()} disabled={importTheme.isPending}>
        {t.themes.importTheme}
      </Button>
      <input ref={input} type="file" accept=".json,application/json" className="sr-only" tabIndex={-1} aria-label={t.themes.importFile} onChange={choose} />
      {unreadable ? (
        <p role="alert" className="w-full text-sm text-danger">
          {t.themes.unreadable}
        </p>
      ) : (
        importTheme.error !== null && (
          <div className="w-full">
            <FormError error={importTheme.error} />
          </div>
        )
      )}
    </>
  );
}

/** The reader's own themes and the shared ones, which they use, and what they may do to each. */
export function ThemesPage() {
  useLanguage();
  const me = useMe().data;
  const administers = me?.role === "administrator";
  const themes = useThemes();
  const active = useActiveTheme();
  const choose = useChooseTheme();
  const actions: RowActions = { choose, setDefault: useSetDefaultTheme(), update: useUpdateTheme(), remove: useDeleteTheme() };
  const failure = choose.error ?? actions.setDefault.error ?? actions.update.error ?? actions.remove.error;
  const [view, setView] = useState<View>("mine");
  const [notice, setNotice] = useState("");
  const all = themes.data ?? [];
  const shown = all.filter((each) => inView(each, view, me?.id));
  const seen = active.data?.theme ?? null;
  const source = active.data?.source ?? "";
  const everybodys = all.find((each) => each.default);
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      {administers && <SettingsNav />}
      <div>
        <h1 className="text-2xl font-semibold">{t.themes.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.themes.intro}</p>
        {active.data && <p className="mt-2 text-sm text-ink">{activeSentence(seen, source, everybodys)}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Choice<View>
          label={t.themes.show}
          value={view}
          onChange={setView}
          options={[
            { value: "mine", label: t.themes.viewMine },
            { value: "shared", label: t.themes.viewShared },
          ]}
        />
        <span className="ml-auto flex flex-wrap justify-end gap-2">
          {seen && source === "everyone" && (
            <Button variant="secondary" onClick={() => choose.mutate({ builtIn: true }, { onSuccess: () => setNotice(t.themes.backToBuiltIn) })}>
              {t.themes.useBuiltIn}
            </Button>
          )}
          {!seen && everybodys && (
            <Button variant="secondary" onClick={() => choose.mutate(null, { onSuccess: () => setNotice(t.themes.backToDefault) })}>
              {t.themes.useDefault}
            </Button>
          )}
          <ImportButton onNotice={setNotice} />
          <Link
            to="/settings/themes/new"
            className="inline-flex h-9 items-center justify-center rounded-md bg-primary px-3 text-sm font-medium text-on-primary hover:bg-primary-hover"
          >
            {t.themes.newTheme}
          </Link>
        </span>
      </div>
      {notice && (
        <p role="status" className="text-sm text-ink-muted">
          {notice}
        </p>
      )}
      <FormError error={failure} />
      {themes.isSuccess &&
        (shown.length === 0 ? (
          <p className="text-sm text-ink-muted">{view === "mine" ? t.themes.emptyMine : t.themes.emptyShared}</p>
        ) : (
          <div className="rounded-xl border border-border bg-surface px-5">
            <table className="w-full text-sm">
              <caption className="sr-only">{view === "mine" ? t.themes.viewMine : t.themes.viewShared}</caption>
              <thead>
                <tr className="border-b border-border text-left text-xs text-ink-subtle">
                  <th className="py-2 pr-3 font-medium">{t.themes.columnTheme}</th>
                  <th className="py-2 pr-3 font-medium">{t.themes.columnOwner}</th>
                  <th className="py-2 pr-3 font-medium">{t.themes.columnInUse}</th>
                  <th className="py-2 font-medium">
                    <span className="sr-only">{t.themes.columnActions}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {shown.map((theme) => (
                  <ThemeRow key={theme.id} theme={theme} me={me?.id} administers={administers} actions={actions} onNotice={setNotice} />
                ))}
              </tbody>
            </table>
          </div>
        ))}
    </div>
  );
}
