import { Link } from "@tanstack/react-router";
import { t, useLanguage } from "@/i18n";

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
