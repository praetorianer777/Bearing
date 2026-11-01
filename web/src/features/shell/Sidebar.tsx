import { Link } from "@tanstack/react-router";
import type { ComponentType, SVGProps } from "react";
import { APP_NAME, SIDEBAR_RAIL_WIDTH, SIDEBAR_WIDTH } from "@/config";
import { useMe } from "@/api/auth";
import { FoldIcon, HomeIcon, OrgIcon, PeopleIcon, PlanningIcon, ProcessIcon, SettingsIcon } from "@/components/icons";
import { Mark } from "@/components/Logo";
import { t, useLanguage } from "@/i18n";

type Place = {
  to: "/" | "/organisation" | "/people" | "/processes" | "/planning" | "/settings/accounts";
  label: () => string;
  icon: ComponentType<SVGProps<SVGSVGElement>>;
};

const places: Place[] = [
  { to: "/", label: () => t.nav.overview, icon: HomeIcon },
  { to: "/organisation", label: () => t.nav.organisation, icon: OrgIcon },
  { to: "/people", label: () => t.nav.people, icon: PeopleIcon },
  { to: "/processes", label: () => t.nav.processes, icon: ProcessIcon },
  { to: "/planning", label: () => t.nav.planning, icon: PlanningIcon },
];

const settings: Place = { to: "/settings/accounts", label: () => t.nav.settings, icon: SettingsIcon };

export function Sidebar({ folded, onToggle }: { folded: boolean; onToggle: () => void }) {
  useLanguage();
  const administrator = useMe().data?.role === "administrator";
  const shown = administrator ? [...places, settings] : places;
  return (
    <aside
      className="flex h-full shrink-0 flex-col border-r border-border bg-surface transition-[width] duration-150"
      style={{ width: folded ? SIDEBAR_RAIL_WIDTH : SIDEBAR_WIDTH }}
    >
      <div className="flex h-12 items-center gap-2 px-4">
        <Mark size={22} />
        {!folded && <span className="text-base font-semibold text-ink">{APP_NAME}</span>}
      </div>
      <nav aria-label={t.nav.main} className="flex flex-1 flex-col gap-0.5 px-2 py-2">
        {shown.map(({ to, label, icon: Icon }) => (
          <Link
            key={to}
            to={to}
            activeOptions={{ exact: to === "/" }}
            className="flex h-8 items-center gap-2.5 rounded-md px-2 text-sm text-ink-muted hover:bg-surface-raised hover:text-ink"
            activeProps={{ className: "bg-accent-subtle text-ink font-medium", "aria-current": "page" }}
            title={folded ? label() : undefined}
          >
            <Icon className="shrink-0" />
            {!folded && <span className="truncate">{label()}</span>}
          </Link>
        ))}
      </nav>
      <div className="px-2 py-2">
        <button
          type="button"
          onClick={onToggle}
          aria-label={folded ? t.nav.expand : t.nav.collapse}
          aria-expanded={!folded}
          className="flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-sm text-ink-subtle hover:bg-surface-raised hover:text-ink"
        >
          <FoldIcon className="shrink-0" />
          {!folded && <span>{t.nav.collapse}</span>}
        </button>
      </div>
    </aside>
  );
}
