import { Outlet } from "@tanstack/react-router";
import { useState } from "react";
import { SearchBox } from "@/features/search/SearchBox";
import { useLanguage } from "@/i18n";
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
          <SearchBox />
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
