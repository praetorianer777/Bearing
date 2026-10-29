import { type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, useId } from "react";
import { ApiError } from "@/api/client";
import { t } from "@/i18n";

/** A labelled input with its error under it, tied to it for screen readers. */
export function Field({ label, error, ...input }: { label: string; error?: string } & InputHTMLAttributes<HTMLInputElement>) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {label}
      </label>
      <input
        id={id}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-error` : undefined}
        className="h-9 rounded-md border border-border-strong bg-surface px-2.5 text-base text-ink outline-none focus:border-accent aria-invalid:border-danger"
        {...input}
      />
      {error && (
        <p id={`${id}-error`} className="text-xs text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

type ButtonProps = { variant?: "primary" | "secondary" | "danger"; children: ReactNode } & ButtonHTMLAttributes<HTMLButtonElement>;

const variants = {
  primary: "bg-primary text-on-primary hover:bg-primary-hover",
  secondary: "border border-border-strong bg-surface text-ink hover:bg-surface-raised",
  danger: "border border-border-strong bg-surface text-danger hover:bg-surface-raised",
};

export function Button({ variant = "primary", type = "button", className = "", ...props }: ButtonProps) {
  return (
    <button
      // biome-ignore lint/a11y/useButtonType: the type is a prop with a safe default
      type={type}
      className={`inline-flex h-9 items-center justify-center rounded-md px-3 text-sm font-medium disabled:opacity-50 ${variants[variant]} ${className}`}
      {...props}
    />
  );
}

/** What a failed call says for a field, from the API's own words. */
export function fieldError(error: unknown, field: string): string | undefined {
  return error instanceof ApiError ? error.fields[field] : undefined;
}

/** A failed call's message for the whole form, unless it is about fields the form shows on their own. */
export function FormError({ error }: { error: unknown }) {
  if (!error) return null;
  const message = error instanceof ApiError ? (Object.keys(error.fields).length > 0 ? null : error.message) : t.forms.unexpected;
  if (!message) return null;
  return (
    <p role="alert" className="rounded-md bg-danger-subtle px-3 py-2 text-sm text-danger">
      {message}
    </p>
  );
}
