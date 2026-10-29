import { type ChangeEvent, useId, useState } from "react";
import { type ThemePackage, themeExportURL, useDeleteTheme, useImportTheme, useSetDefaultTheme, useThemes } from "@/api/themes";
import { Button, FormError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";
import { SettingsNav } from "./SettingsNav";

// FileReader rather than file.text(), which the test DOM does not have.
function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

function ImportForm() {
  const inputId = useId();
  const importTheme = useImportTheme();
  const [unreadable, setUnreadable] = useState(false);
  const [imported, setImported] = useState("");
  const choose = async (event: ChangeEvent<HTMLInputElement>) => {
    const input = event.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    setImported("");
    let pkg: ThemePackage;
    try {
      pkg = JSON.parse(await readText(file));
      setUnreadable(false);
    } catch {
      setUnreadable(true);
      input.value = "";
      return;
    }
    importTheme.mutate(pkg, { onSuccess: (theme) => setImported(theme.name), onSettled: () => (input.value = "") });
  };
  return (
    <section aria-labelledby="import-heading" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="import-heading" className="text-lg font-semibold">
        {t.themes.import}
      </h2>
      <p className="mt-1 text-sm text-ink-muted">{t.themes.importIntro}</p>
      <label htmlFor={inputId} className="mt-3 block text-sm font-medium text-ink">
        {t.themes.file}
      </label>
      <input id={inputId} type="file" accept=".json,application/json" onChange={choose} disabled={importTheme.isPending} className="mt-1 text-sm text-ink" />
      <div className="mt-2">{unreadable ? <p className="text-sm text-danger">{t.themes.unreadable}</p> : <FormError error={importTheme.error} />}</div>
      {imported && (
        <p role="status" className="mt-2 text-sm text-ink">
          {t.themes.imported(imported)}
        </p>
      )}
    </section>
  );
}

export function ThemesPage() {
  useLanguage();
  const themes = useThemes();
  const remove = useDeleteTheme();
  const setDefault = useSetDefaultTheme();
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      <SettingsNav />
      <div>
        <h1 className="text-2xl font-semibold">{t.themes.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.themes.intro}</p>
      </div>
      <section aria-labelledby="installed-heading">
        <h2 id="installed-heading" className="sr-only">
          {t.themes.title}
        </h2>
        {themes.data?.length === 0 && <p className="text-sm text-ink-muted">{t.themes.none}</p>}
        <ul aria-labelledby="installed-heading" className="rounded-xl border border-border bg-surface px-5 empty:hidden">
          {themes.data?.map((theme) => (
            <li key={theme.id} className="flex flex-wrap items-center gap-3 border-b border-border py-3 last:border-b-0">
              <div className="flex gap-1" aria-hidden="true">
                {["canvas", "primary", "accent"].map((token) => (
                  <span
                    key={token}
                    className="size-4 rounded-full border border-border"
                    style={{ backgroundColor: theme.spec.colors.light[token] ?? `var(--color-${token})` }}
                  />
                ))}
              </div>
              <p className="min-w-0 flex-1 truncate text-base font-medium text-ink">
                {theme.name}
                {theme.default && <span className="ml-2 text-xs font-normal text-ink-subtle">{t.themes.isDefault}</span>}
              </p>
              {theme.default ? (
                <Button variant="secondary" onClick={() => setDefault.mutate(null)} aria-label={`${t.themes.unsetDefault}: ${theme.name}`}>
                  {t.themes.unsetDefault}
                </Button>
              ) : (
                <Button variant="secondary" onClick={() => setDefault.mutate(theme.id)} aria-label={`${t.themes.setDefault}: ${theme.name}`}>
                  {t.themes.setDefault}
                </Button>
              )}
              <a
                href={themeExportURL(theme.id)}
                download
                aria-label={`${t.themes.export}: ${theme.name}`}
                className="inline-flex h-9 items-center justify-center rounded-md border border-border-strong bg-surface px-3 text-sm font-medium text-ink hover:bg-surface-raised"
              >
                {t.themes.export}
              </a>
              <Button variant="danger" onClick={() => remove.mutate(theme.id)} disabled={remove.isPending} aria-label={`${t.themes.remove}: ${theme.name}`}>
                {t.themes.remove}
              </Button>
            </li>
          ))}
        </ul>
        <div className="mt-2">
          <FormError error={remove.error ?? setDefault.error} />
        </div>
      </section>
      <ImportForm />
    </div>
  );
}
