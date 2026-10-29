import { Outlet } from "@tanstack/react-router";
import { useState } from "react";
import { SearchIcon } from "@/components/icons";
import { t, useLanguage } from "@/i18n";
import { AccountMenu } from "./AccountMenu";
import { Sidebar } from "./Sidebar";
import { ThemeStyle } from "./ThemeStyle";
import { readFolded, writeFolded } from "./sidebar";

/** The frame every page sits in: the sidebar, the strip along the top, and the page. */
export function AppShell() {
  useLanguage();
  const [folded, setFolded] = useState(() => readFolded());
  const toggle = () =>
    setFolded((was) => {
      writeFolded(!was);
      return !was;
    });

  return (
    <div className="flex h-full bg-canvas text-ink">
      <ThemeStyle />
      <Sidebar folded={folded} onToggle={toggle} />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border px-4">
          <label className="flex h-8 max-w-md flex-1 items-center gap-2 rounded-md border border-border bg-surface px-2 text-sm text-ink-subtle">
            <SearchIcon />
            <input type="search" disabled placeholder={t.nav.search} aria-label={t.nav.search} className="min-w-0 flex-1 bg-transparent outline-none" />
          </label>
          <div className="ml-auto">
            <AccountMenu />
          </div>
        </header>
        {/* Focusable so a keyboard can scroll the page, which scrolls here rather than in the window. */}
        <main tabIndex={0} className="min-h-0 flex-1 overflow-auto bg-backdrop outline-none">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
