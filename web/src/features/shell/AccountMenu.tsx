import { Link, useNavigate } from "@tanstack/react-router";
import { useEffect, useId, useRef, useState } from "react";
import { useMe, useSignOut } from "@/api/auth";
import { useThemes } from "@/api/themes";
import { APP_VERSION } from "@/config";
import { applyLanguage, LANGUAGES, type LanguageChoice, t, useLanguage } from "@/i18n";
import { LANGUAGE_STORAGE_KEY } from "@/config";
import { applyAppearance, BUILT_IN_THEME, INSTALLATION_THEME, MODES, useAppearance } from "@/lib/appearance";

function storedChoice(): LanguageChoice {
  try {
    const value = localStorage.getItem(LANGUAGE_STORAGE_KEY);
    return value === "en" || value === "de" ? value : "";
  } catch {
    return "";
  }
}

function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("");
}

/** The avatar button and its menu: who is signed in, the language, signing out, and what this build is. */
export function AccountMenu() {
  useLanguage();
  const me = useMe().data;
  const signOut = useSignOut();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState<LanguageChoice>(storedChoice);
  const menuId = useId();
  const themeId = useId();
  const appearance = useAppearance();
  const themes = useThemes(open);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent ? event.key === "Escape" : !root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);

  const choose = (next: LanguageChoice) => {
    setChoice(next);
    applyLanguage(next);
  };

  return (
    <div ref={root} className="relative">
      <button
        type="button"
        aria-label={t.account.menu}
        aria-haspopup="true"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((was) => !was)}
        className="flex size-8 items-center justify-center rounded-full bg-surface-sunken text-xs font-semibold text-ink-muted hover:text-ink"
      >
        {me ? initials(me.name) : "?"}
      </button>
      {open && (
        <div id={menuId} className="absolute right-0 top-10 z-20 w-60 rounded-lg border border-border bg-surface-overlay p-3 shadow-2">
          {me && (
            <div className="mb-3 border-b border-border pb-3">
              <p className="truncate text-sm font-medium text-ink">{me.name}</p>
              <p className="truncate text-xs text-ink-muted">{me.email}</p>
              <p className="text-xs text-ink-subtle">{t.roles[me.role]}</p>
              <Link to="/settings/profile" onClick={() => setOpen(false)} className="mt-1.5 inline-block text-sm text-accent hover:underline">
                {t.account.profile}
              </Link>
              <Link to="/settings/connected-apps" onClick={() => setOpen(false)} className="mt-1 block text-sm text-accent hover:underline">
                {t.account.connectedApps}
              </Link>
              <Link to="/settings/tokens" onClick={() => setOpen(false)} className="mt-1 block text-sm text-accent hover:underline">
                {t.account.tokens}
              </Link>
            </div>
          )}
          <fieldset>
            <legend className="mb-1.5 text-xs font-medium text-ink-subtle">{t.account.language}</legend>
            {(["", ...LANGUAGES] as LanguageChoice[]).map((each) => (
              <label key={each || "browser"} className="flex h-7 items-center gap-2 text-sm text-ink">
                <input type="radio" name="language" checked={choice === each} onChange={() => choose(each)} />
                {each ? t.account.languages[each] : t.account.languageBrowser}
              </label>
            ))}
          </fieldset>
          <fieldset className="mt-3">
            <legend className="mb-1.5 text-xs font-medium text-ink-subtle">{t.account.mode}</legend>
            {MODES.map((mode) => (
              <label key={mode} className="flex h-7 items-center gap-2 text-sm text-ink">
                <input type="radio" name="mode" checked={appearance.mode === mode} onChange={() => applyAppearance({ mode })} />
                {t.account.modes[mode]}
              </label>
            ))}
          </fieldset>
          <div className="mt-3 flex flex-col gap-1">
            <label htmlFor={themeId} className="text-xs font-medium text-ink-subtle">
              {t.account.theme}
            </label>
            <select
              id={themeId}
              value={appearance.theme}
              onChange={(event) => applyAppearance({ theme: event.target.value })}
              className="h-8 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
            >
              <option value={INSTALLATION_THEME}>{t.account.themeInstallation}</option>
              <option value={BUILT_IN_THEME}>{t.account.themeBuiltIn}</option>
              {themes.data?.map((each) => (
                <option key={each.id} value={each.id}>
                  {each.name}
                </option>
              ))}
            </select>
          </div>
          <button
            type="button"
            onClick={() => signOut.mutate(undefined, { onSuccess: () => navigate({ to: "/sign-in" }) })}
            className="mt-3 w-full rounded-md border border-border px-2 py-1.5 text-left text-sm text-ink hover:bg-surface-raised"
          >
            {t.account.signOut}
          </button>
          <p className="mt-3 border-t border-border pt-2 text-xs text-ink-subtle">{t.account.about(APP_VERSION)}</p>
        </div>
      )}
    </div>
  );
}
