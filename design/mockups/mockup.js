// Fills <nav class="tabbar" data-active="..."> so every mockup shares one tab bar.
const tabs = [
  ["a-voir", "a-voir.html", "À voir", '<rect x="3" y="7" width="18" height="11" rx="2"/><path d="M9 7v11" stroke-dasharray="2 2"/>'],
  ["journal", "journal.html", "Journal", '<path d="M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3z"/><path d="M5 17a3 3 0 0 1 3-3h11"/>'],
  ["chercher", "chercher.html", "Chercher", '<circle cx="11" cy="11" r="6"/><path d="m20 20-4.5-4.5"/>'],
  ["moi", "moi.html", "Moi", '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>'],
];

for (const nav of document.querySelectorAll("nav.tabbar")) {
  nav.innerHTML = tabs
    .map(([key, href, label, icon]) => {
      const current = key === nav.dataset.active ? ' aria-current="page"' : "";
      return `<a href="${href}"${current}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${icon}</svg>${label}</a>`;
    })
    .join("");
}
