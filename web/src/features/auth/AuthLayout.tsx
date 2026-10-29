import type { ReactNode } from "react";
import { Mark } from "@/components/Logo";
import { useLanguage } from "@/i18n";

/** The frame of the pages before anybody is signed in: the mark and a card. */
export function AuthLayout({ title, intro, children }: { title: string; intro?: string; children: ReactNode }) {
  useLanguage();
  return (
    <main className="flex min-h-full items-start justify-center bg-backdrop px-4 py-16">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center">
          <Mark size={40} />
        </div>
        <div className="rounded-xl border border-border bg-surface p-6 shadow-1">
          <h1 className="text-xl font-semibold text-ink">{title}</h1>
          {intro && <p className="mt-1 text-sm text-ink-muted">{intro}</p>}
          <div className="mt-5">{children}</div>
        </div>
      </div>
    </main>
  );
}
