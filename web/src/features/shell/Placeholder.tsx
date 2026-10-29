import { Link } from "@tanstack/react-router";
import { t, useLanguage } from "@/i18n";

/** A page that is planned and not built yet, under its own title. */
export function ComingSoon({ title }: { title: () => string }) {
  useLanguage();
  return (
    <div className="mx-auto max-w-3xl px-6 py-8">
      <h1 className="text-2xl font-semibold">{title()}</h1>
      <p className="mt-2 text-base text-ink-muted">{t.comingSoon.body}</p>
    </div>
  );
}

export function NotFound() {
  useLanguage();
  return (
    <div className="mx-auto max-w-3xl px-6 py-8">
      <h1 className="text-2xl font-semibold">{t.notFound.title}</h1>
      <p className="mt-2 text-base text-ink-muted">{t.notFound.body}</p>
      <Link to="/" className="mt-4 inline-block text-sm text-accent hover:underline">
        {t.notFound.back}
      </Link>
    </div>
  );
}
