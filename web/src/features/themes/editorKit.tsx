import { type ReactNode, type TextareaHTMLAttributes, useId } from "react";
import type { ThemeAsset } from "@/api/themes";
import { t } from "@/i18n";
import type { ThemeSpec } from "@/lib/theme";

/** A change to the draft, made on a copy of it. */
export type Patch = (change: (draft: ThemeSpec) => void) => void;

export const isFont = (a: ThemeAsset) => a.contentType.startsWith("font/");
export const isImage = (a: ThemeAsset) => a.contentType.startsWith("image/");

export const controlClass = "h-9 rounded-md border border-border-strong bg-surface px-2.5 text-sm text-ink outline-none focus:border-accent";

/** One part of a tab, under its heading. */
export function Section({ title, children }: { title?: string; children: ReactNode }) {
  const id = useId();
  return (
    <section aria-labelledby={title ? id : undefined} className="rounded-xl border border-border bg-surface p-4">
      {title && (
        <h2 id={id} className="mb-3 text-sm font-semibold text-ink">
          {title}
        </h2>
      )}
      {children}
    </section>
  );
}

/** A few options side by side, as radio buttons a keyboard moves through with the arrows. */
export function Choice<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  const name = useId();
  return (
    <fieldset className="flex flex-wrap items-center gap-2">
      <legend className="sr-only">{label}</legend>
      <span className="inline-flex rounded-md border border-border-strong p-0.5">
        {options.map((option) => (
          <label
            key={option.value}
            className="cursor-pointer rounded px-2.5 py-1 text-sm text-ink-muted has-checked:bg-accent-subtle has-checked:font-medium has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-focus"
          >
            <input type="radio" name={name} className="sr-only" checked={value === option.value} onChange={() => onChange(option.value)} />
            {option.label}
          </label>
        ))}
      </span>
    </fieldset>
  );
}

/** A labelled choice of one of the theme's files, or none. */
export function AssetSelect({
  label,
  assets,
  value,
  onChange,
}: {
  label: string;
  assets: ThemeAsset[];
  value: string | undefined;
  onChange: (id: string | undefined) => void;
}) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {label}
      </label>
      <select id={id} value={value ?? ""} onChange={(event) => onChange(event.target.value || undefined)} className={`${controlClass} w-56`}>
        <option value="">{assets.length === 0 ? t.themes.editor.noFiles : t.themes.editor.chooseFile}</option>
        {assets.map((a) => (
          <option key={a.id} value={a.id}>
            {a.name}
          </option>
        ))}
      </select>
    </div>
  );
}

/** A labelled whole number between 0 and max; empty is none. */
export function NumberField({
  label,
  value,
  max,
  placeholder,
  onChange,
}: {
  label: string;
  value: number | undefined;
  max: number;
  placeholder: string;
  onChange: (value: number | undefined) => void;
}) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {label}
      </label>
      <input
        id={id}
        type="number"
        min={0}
        max={max}
        value={value ?? ""}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value === "" ? undefined : Math.max(0, Math.min(max, Number(event.target.value))))}
        className={`${controlClass} w-28`}
      />
    </div>
  );
}

/** A labelled box of several lines, with a hint under it. */
export function TextArea({ label, hint, className = "", ...area }: { label: string; hint?: string } & TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {label}
      </label>
      <textarea
        id={id}
        aria-describedby={hint ? `${id}-hint` : undefined}
        className={`rounded-md border border-border-strong bg-surface px-2.5 py-1.5 font-mono text-sm text-ink outline-none focus:border-accent ${className}`}
        {...area}
      />
      {hint && (
        <p id={`${id}-hint`} className="text-xs text-ink-subtle">
          {hint}
        </p>
      )}
    </div>
  );
}

/** A small button to take a change back. */
export function ResetButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} aria-label={label} className="h-9 rounded-md px-2 text-sm text-ink-muted hover:bg-surface-raised hover:text-ink">
      {t.themes.editor.reset}
    </button>
  );
}
