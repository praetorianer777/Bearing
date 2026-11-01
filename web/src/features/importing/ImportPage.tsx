import { type ChangeEvent, type FormEvent, useId, useState } from "react";
import { EXPORT_KINDS, type ExportKind, exportURL, type ImportFiles, type OrgImport, useImport, usePreviewImport, useRunImport } from "@/api/imports";
import { Button, Field, FormError } from "@/components/ui";
import { today } from "@/features/org/common";
import { useEditor } from "@/features/org/edit";
import { t, useLanguage } from "@/i18n";

// FileReader rather than file.text(), which the test DOM does not have.
function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

function Export() {
  const [asOf, setAsOf] = useState(today());
  return (
    <section aria-labelledby="export-heading" className="rounded-xl border border-border bg-surface p-5 text-sm">
      <h2 id="export-heading" className="text-lg font-semibold">
        {t.importing.export}
      </h2>
      <p className="mt-1 text-ink-muted">{t.importing.exportIntro}</p>
      <div className="mt-3 max-w-48">
        <Field label={t.importing.asOf} type="date" value={asOf} onChange={(e) => e.target.value && setAsOf(e.target.value)} />
      </div>
      <ul className="mt-3 flex flex-wrap gap-2">
        {EXPORT_KINDS.map((kind) => (
          <li key={kind}>
            <a
              href={exportURL(kind, asOf)}
              download
              className="inline-flex h-9 items-center rounded-md border border-border-strong bg-surface px-3 font-medium text-ink hover:bg-surface-raised"
            >
              {t.importing.file[kind]}
            </a>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Preview({ found }: { found: OrgImport }) {
  const run = useRunImport();
  // Followed only once it runs; until then the preview is all there is.
  const [started, setStarted] = useState(false);
  const current = useImport(started ? found.id : undefined).data ?? found;
  return (
    <section aria-labelledby="preview-heading" className="rounded-xl border border-border bg-surface p-5 text-sm">
      <h2 id="preview-heading" className="text-lg font-semibold">
        {t.importing.preview}
      </h2>
      <p className="mt-1">{t.importing.counts(current.counts.create ?? 0, current.counts.update ?? 0, current.counts.unchanged ?? 0)}</p>
      {current.errors.length > 0 && (
        <div className="mt-3">
          <h3 className="font-medium text-danger">{t.importing.errors(current.errors.length)}</h3>
          <ul aria-label={t.importing.errorList} className="mt-1 space-y-1">
            {current.errors.map((e) => (
              <li key={`${e.file}-${e.row}-${e.message}`}>
                <span className="font-medium">{t.importing.where(t.importing.file[e.file as ExportKind] ?? e.file, e.row)}</span> {e.message}
              </li>
            ))}
          </ul>
        </div>
      )}
      {current.changes.some((c) => c.action !== "unchanged") && (
        <table className="mt-3 w-full text-left">
          <caption className="sr-only">{t.importing.changes}</caption>
          <thead>
            <tr className="text-ink-muted">
              <th scope="col" className="py-1 font-medium">
                {t.importing.fileColumn}
              </th>
              <th scope="col" className="py-1 font-medium">
                {t.importing.row}
              </th>
              <th scope="col" className="py-1 font-medium">
                {t.importing.action}
              </th>
              <th scope="col" className="py-1 font-medium">
                {t.importing.name}
              </th>
            </tr>
          </thead>
          <tbody>
            {current.changes
              .filter((c) => c.action !== "unchanged")
              .map((c) => (
                <tr key={`${c.file}-${c.row}`} className="border-t border-border">
                  <td className="py-1">{t.importing.file[c.file as ExportKind] ?? c.file}</td>
                  <td className="py-1 tabular-nums">{c.row}</td>
                  <td className="py-1">{t.importing.actions[c.action] ?? c.action}</td>
                  <td className="py-1">{c.name}</td>
                </tr>
              ))}
          </tbody>
        </table>
      )}
      <div className="mt-4">
        {current.status === "previewed" && current.errors.length === 0 && (current.counts.create ?? 0) + (current.counts.update ?? 0) > 0 && (
          <Button onClick={() => run.mutate(found.id, { onSuccess: () => setStarted(true) })} disabled={run.isPending}>
            {t.importing.run}
          </Button>
        )}
        <FormError error={run.error} />
        {current.status !== "previewed" && (
          <p role="status" className={current.status === "failed" ? "text-danger" : "text-ink"}>
            {current.status === "done"
              ? t.importing.done
              : current.status === "failed"
                ? current.result?.error
                  ? t.importing.failedAt(
                      t.importing.file[current.result.error.file as ExportKind] ?? current.result.error.file,
                      current.result.error.row,
                      current.result.error.message,
                    )
                  : t.importing.failed
                : t.importing.running}
          </p>
        )}
      </div>
    </section>
  );
}

function Import() {
  const id = useId();
  const preview = usePreviewImport();
  const [files, setFiles] = useState<ImportFiles>({});
  const [from, setFrom] = useState(today());
  const choose = (kind: ExportKind) => async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0];
    const text = file ? await readText(file) : undefined;
    setFiles((was) => ({ ...was, [kind]: text }));
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    preview.mutate({ ...files, from });
  };
  return (
    <>
      <section aria-labelledby="import-heading" className="rounded-xl border border-border bg-surface p-5 text-sm">
        <h2 id="import-heading" className="text-lg font-semibold">
          {t.importing.import}
        </h2>
        <p className="mt-1 text-ink-muted">{t.importing.importIntro}</p>
        <form onSubmit={submit} className="mt-3 flex flex-col gap-3">
          <div className="grid gap-3 sm:grid-cols-2">
            {EXPORT_KINDS.map((kind) => (
              <div key={kind} className="flex flex-col gap-1">
                <label htmlFor={`${id}-${kind}`} className="font-medium">
                  {t.importing.file[kind]}
                </label>
                <input id={`${id}-${kind}`} type="file" accept=".csv,text/csv" onChange={choose(kind)} className="text-ink" />
              </div>
            ))}
          </div>
          <div className="max-w-48">
            <Field label={t.importing.from} type="date" required value={from} onChange={(e) => setFrom(e.target.value)} />
          </div>
          <FormError error={preview.error} />
          <Button type="submit" disabled={preview.isPending || !Object.values(files).some(Boolean)} className="self-start">
            {t.importing.previewSubmit}
          </Button>
        </form>
      </section>
      {preview.data && <Preview key={preview.data.id} found={preview.data} />}
    </>
  );
}

/** The organisation out to CSV files and back in from them, previewed before anything changes; for editors. */
export function ImportPage() {
  useLanguage();
  const editor = useEditor();
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.importing.title}</h1>
        <p className="mt-1 text-sm text-ink-muted">{t.importing.intro}</p>
      </div>
      {editor ? (
        <>
          <Export />
          <Import />
        </>
      ) : (
        <p className="text-sm text-ink-muted">{t.importing.editorsOnly}</p>
      )}
    </div>
  );
}
