// Docs page helpers: the "On this page" list, the sidebar filter and the
// search on the docs home. Everything works without them.
(() => {
  const toc = document.getElementById("toc");
  const headings = document.querySelectorAll(".prose h2[id]");
  if (toc && headings.length > 1) {
    const list = toc.querySelector("ol");
    for (const h of headings) {
      const a = Object.assign(document.createElement("a"), { href: `#${h.id}`, textContent: h.textContent });
      const li = document.createElement("li");
      li.append(a);
      list.append(li);
    }
    toc.hidden = false;
  }

  const filter = document.getElementById("filter");
  filter?.addEventListener("input", () => {
    const q = filter.value.trim().toLowerCase();
    for (const a of document.querySelectorAll(".sidebar a[data-text]")) a.hidden = q !== "" && !a.dataset.text.includes(q);
  });

  const search = document.getElementById("search");
  const results = document.getElementById("results");
  search?.addEventListener("input", () => {
    const q = search.value.trim().toLowerCase();
    const hits = q ? [...document.querySelectorAll(".cards a[data-text]")].filter((a) => a.dataset.text.includes(q)) : [];
    results.replaceChildren(...hits.map((a) => {
      const li = document.createElement("li");
      const link = Object.assign(document.createElement("a"), { href: a.href });
      link.append(Object.assign(document.createElement("strong"), { textContent: a.textContent }),
        Object.assign(document.createElement("span"), { textContent: `${a.dataset.section} · ${a.dataset.summary}` }));
      li.append(link);
      return li;
    }));
    if (q && !hits.length) results.append(Object.assign(document.createElement("li"), { className: "none", textContent: "No page matches." }));
    results.hidden = !q;
  });
})();
