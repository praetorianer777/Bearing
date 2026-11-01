import { useNavigate } from "@tanstack/react-router";
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { searchable, useQuickSearch } from "@/api/search";
import { SearchIcon } from "@/components/icons";
import { t, useLanguage } from "@/i18n";
import { hitKey, hitTarget } from "./hits";

/** Whether a key press lands in something that takes typing, where / is a letter rather than the way to search. */
function typingIn(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName);
}

/** The search box along the top: the best few hits while typing, every hit on the search page with Enter. */
export function SearchBox() {
  useLanguage();
  const id = useId();
  const navigate = useNavigate();
  const input = useRef<HTMLInputElement>(null);
  const [text, setText] = useState("");
  const [asked, setAsked] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  // A request after a pause in typing, not one per key.
  useEffect(() => {
    const timer = setTimeout(() => setAsked(text.trim()), 200);
    return () => clearTimeout(timer);
  }, [text]);
  useEffect(() => {
    const focus = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey || typingIn(event.target)) return;
      event.preventDefault();
      input.current?.focus();
    };
    document.addEventListener("keydown", focus);
    return () => document.removeEventListener("keydown", focus);
  }, []);

  const quick = useQuickSearch(asked);
  const hits = searchable(asked) ? (quick.data ?? []) : [];
  const shown = open && searchable(text);
  // The options are the hits and, last, every result on the search page.
  const count = hits.length + 1;

  const done = () => {
    setOpen(false);
    setActive(-1);
    setText("");
    input.current?.blur();
  };
  const choose = (index: number) => {
    const hit = hits[index];
    if (hit) navigate(hitTarget(hit));
    else navigate({ to: "/search", search: { q: text.trim() } });
    done();
  };
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      if (open && text) setOpen(false);
      else done();
      return;
    }
    if (!searchable(text)) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setOpen(true);
      const step = event.key === "ArrowDown" ? 1 : -1;
      // Round from the box through every option and back to the box.
      setActive((was) => ((was + 1 + step + count + 1) % (count + 1)) - 1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      choose(active >= 0 ? active : hits.length);
    }
  };

  const option = (index: number) => `${id}-option-${index}`;
  return (
    <div className="relative max-w-md flex-1">
      <label className="flex h-8 items-center gap-2 rounded-md border border-border bg-surface px-2 text-sm text-ink-subtle focus-within:border-focus">
        <SearchIcon />
        <input
          ref={input}
          type="search"
          role="combobox"
          aria-label={t.nav.search}
          aria-expanded={shown}
          aria-controls={`${id}-list`}
          aria-autocomplete="list"
          aria-activedescendant={shown && active >= 0 ? option(active) : undefined}
          placeholder={t.nav.search}
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            setOpen(true);
            setActive(-1);
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          onKeyDown={onKeyDown}
          className="min-w-0 flex-1 bg-transparent text-ink outline-none placeholder:text-ink-subtle"
        />
        <kbd className="hidden rounded border border-border px-1 text-xs text-ink-subtle sm:inline">/</kbd>
      </label>
      {shown && (
        <div
          id={`${id}-list`}
          role="listbox"
          aria-label={t.search.suggestions}
          className="absolute inset-x-0 top-9 z-30 overflow-hidden rounded-md border border-border bg-surface-overlay py-1 shadow-2"
        >
          {hits.map((hit, index) => (
            <div
              key={hitKey(hit)}
              id={option(index)}
              role="option"
              aria-selected={index === active}
              tabIndex={-1}
              // Picked on press, so focus never leaves the box that names the active option.
              onMouseDown={(event) => {
                event.preventDefault();
                choose(index);
              }}
              className={`flex cursor-pointer items-baseline gap-2 px-3 py-1.5 text-sm ${index === active ? "bg-selection" : "hover:bg-surface-raised"}`}
            >
              <span className="w-16 shrink-0 text-xs text-ink-subtle">{t.search.kind[hit.kind] ?? hit.kind}</span>
              <span className="min-w-0 truncate text-ink">{hit.name}</span>
              {hit.detail && <span className="min-w-0 truncate text-xs text-ink-muted">{hit.detail}</span>}
            </div>
          ))}
          {quick.isSuccess && hits.length === 0 && searchable(asked) && <p className="px-3 py-1.5 text-sm text-ink-muted">{t.search.none}</p>}
          <div
            id={option(hits.length)}
            role="option"
            aria-selected={active === hits.length}
            tabIndex={-1}
            onMouseDown={(event) => {
              event.preventDefault();
              choose(hits.length);
            }}
            className={`cursor-pointer border-t border-border px-3 py-1.5 text-sm text-ink-muted ${active === hits.length ? "bg-selection" : "hover:bg-surface-raised"}`}
          >
            {t.search.everything(text.trim())}
          </div>
        </div>
      )}
    </div>
  );
}
