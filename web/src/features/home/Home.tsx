import { useQuery } from "@tanstack/react-query";
import { fetchMeta } from "@/api/client";
import { t, useLanguage } from "@/i18n";

export function Home() {
  useLanguage();
  const meta = useQuery({ queryKey: ["meta"], queryFn: fetchMeta });
  return (
    <div className="mx-auto max-w-3xl px-6 py-8">
      <h1 className="text-2xl font-semibold">{t.home.title}</h1>
      <p className="mt-2 text-base text-ink-muted">{t.home.intro}</p>
      <h2 className="mt-8 text-lg font-semibold">{t.home.next}</h2>
      <ol className="mt-2 list-decimal space-y-1 pl-5 text-base text-ink-muted">
        {t.home.steps.map((step) => (
          <li key={step}>{step}</li>
        ))}
      </ol>
      <h2 className="mt-8 text-lg font-semibold">{t.home.status}</h2>
      <p className="mt-2 text-sm text-ink-muted" data-testid="api-status">
        {meta.isSuccess ? t.home.apiBuild(meta.data.build.version) : meta.isError ? t.home.apiUnreachable : " "}
      </p>
    </div>
  );
}
