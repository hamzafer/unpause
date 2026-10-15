"use client";

import { Moon, Sun } from "lucide-react";
import { useEffect, useSyncExternalStore } from "react";
import { Button } from "@/components/ui/button";

const KEY = "unpause-theme";

function isDark() {
  return document.documentElement.classList.contains("dark");
}

// The theme lives on <html class="dark">; watch it instead of mirroring it.
function subscribe(f: () => void) {
  const mo = new MutationObserver(f);
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => mo.disconnect();
}

/** Flips dark and light and remembers the pick. Until something is picked,
 *  the page follows the system (the inline script in layout.tsx set it). */
export function ThemeToggle() {
  const dark = useSyncExternalStore(subscribe, isDark, () => true);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: light)");
    const onChange = () => {
      let picked: string | null = null;
      try {
        picked = localStorage.getItem(KEY);
      } catch {}
      if (picked) return;
      document.documentElement.classList.toggle("dark", !mq.matches);
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const flip = () => {
    const next = !isDark();
    document.documentElement.classList.toggle("dark", next);
    try {
      localStorage.setItem(KEY, next ? "dark" : "light");
    } catch {}
  };

  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={flip}
      aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}
      title={dark ? "Light theme" : "Dark theme"}
      className="size-8 text-muted-foreground hover:text-foreground"
    >
      {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </Button>
  );
}
