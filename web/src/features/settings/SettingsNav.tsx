import { Link } from "@tanstack/react-router";
import { t } from "@/i18n";

/** The settings' pages, as tabs along the top of each. */
export function SettingsNav() {
  return (
    <nav aria-label={t.nav.settings} className="flex gap-1 border-b border-border">
      {(
        [
          ["/settings/accounts", t.accounts.title],
          ["/settings/themes", t.themes.title],
        ] as const
      ).map(([to, label]) => (
        <Link
          key={to}
          to={to}
          className="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-ink-muted hover:text-ink"
          activeProps={{ className: "border-ink! text-ink!", "aria-current": "page" }}
        >
          {label}
        </Link>
      ))}
    </nav>
  );
}
