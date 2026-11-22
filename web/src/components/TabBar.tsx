import { For } from "solid-js";
import { route } from "../router";

const tabs = [
  { href: "/", label: "À voir", icon: '<rect x="3" y="7" width="18" height="11" rx="2"/><path d="M9 7v11" stroke-dasharray="2 2"/>' },
  { href: "/journal", label: "Journal", icon: '<path d="M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3z"/><path d="M5 17a3 3 0 0 1 3-3h11"/>' },
  { href: "/chercher", label: "Chercher", icon: '<circle cx="11" cy="11" r="6"/><path d="m20 20-4.5-4.5"/>' },
  { href: "/moi", label: "Moi", icon: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>' },
];

// Pages below a tab (a production, a person) keep the tab they were opened from highlighted.
let lastTab = "/";

export function TabBar() {
  const active = () => {
    const path = route().path;
    const tab = tabs.find((t) => t.href === path);
    if (tab) lastTab = tab.href;
    return lastTab;
  };
  return (
    <nav class="tabbar">
      <For each={tabs}>
        {(t) => (
          <a href={t.href} aria-current={active() === t.href ? "page" : undefined}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" innerHTML={t.icon} />
            {t.label}
          </a>
        )}
      </For>
    </nav>
  );
}
