import { MonitorIcon, MoonIcon, SunIcon } from "@/components/icons";
import { t } from "@/i18n";
import { applyAppearance, type Mode, nextMode, useAppearance } from "@/lib/appearance";

const glyphs: Record<Mode, typeof SunIcon> = { system: MonitorIcon, light: SunIcon, dark: MoonIcon };

/** Steps through as the system, light and dark, beside the account menu's choice of the same. */
export function ThemeButton({ folded }: { folded: boolean }) {
  const { mode } = useAppearance();
  const Glyph = glyphs[mode];
  const label = t.account.modes[mode];
  return (
    <button
      type="button"
      onClick={() => applyAppearance({ mode: nextMode(mode) })}
      aria-label={t.account.modeButton(label)}
      title={folded ? t.account.modeButton(label) : undefined}
      data-mode={mode}
      className="flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-sm text-ink-subtle hover:bg-surface-raised hover:text-ink"
    >
      <Glyph className="shrink-0" />
      {!folded && <span className="truncate">{label}</span>}
    </button>
  );
}
