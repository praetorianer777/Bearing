import { useEffect, useId, useRef, useState } from "react";
import { APP_VERSION } from "@/config";
import { applyLanguage, LANGUAGES, type LanguageChoice, t, useLanguage } from "@/i18n";
import { LANGUAGE_STORAGE_KEY } from "@/config";

function storedChoice(): LanguageChoice {
  try {
    const value = localStorage.getItem(LANGUAGE_STORAGE_KEY);
    return value === "en" || value === "de" ? value : "";
  } catch {
    return "";
  }
}

/** The avatar button and its menu. Until people sign in it holds the language and what this build is. */
export function AccountMenu() {
  useLanguage();
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState<LanguageChoice>(storedChoice);
  const menuId = useId();
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
        ?
      </button>
      {open && (
        <div id={menuId} className="absolute right-0 top-10 z-20 w-60 rounded-lg border border-border bg-surface-overlay p-3 shadow-2">
          <fieldset>
            <legend className="mb-1.5 text-xs font-medium text-ink-subtle">{t.account.language}</legend>
            {(["", ...LANGUAGES] as LanguageChoice[]).map((each) => (
              <label key={each || "browser"} className="flex h-7 items-center gap-2 text-sm text-ink">
                <input type="radio" name="language" checked={choice === each} onChange={() => choose(each)} />
                {each ? t.account.languages[each] : t.account.languageBrowser}
              </label>
            ))}
          </fieldset>
          <p className="mt-3 border-t border-border pt-2 text-xs text-ink-subtle">{t.account.about(APP_VERSION)}</p>
        </div>
      )}
    </div>
  );
}
