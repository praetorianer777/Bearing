import { useThemes } from "@/api/themes";
import { shownTheme, useAppearance } from "@/lib/appearance";
import { compileTheme } from "@/lib/theme";

/** The stylesheet of the theme this browser shows, after the built-in one so it wins. */
export function ThemeStyle() {
  const { theme: choice } = useAppearance();
  const themes = useThemes();
  const shown = shownTheme(themes.data ?? [], choice);
  if (!shown) return null;
  return <style data-theme-id={shown.id}>{compileTheme(shown)}</style>;
}
