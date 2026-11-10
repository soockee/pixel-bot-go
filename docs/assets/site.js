// Docs page helpers: the "On this page" list, and the sidebar filter. Everything works without them.
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

  const video = document.getElementById("fishing-demo");
  const toggle = document.querySelector(".replay-toggle");
  if (video && toggle) {
    const updateLabel = () => { toggle.textContent = video.paused ? "Play demo" : "Pause demo"; };
    video.addEventListener("play", updateLabel);
    video.addEventListener("pause", updateLabel);
    toggle.addEventListener("click", () => {
      if (video.paused) video.play().catch(updateLabel);
      else video.pause();
    });
    updateLabel();
    toggle.hidden = false;
    video.controls = false;
  }
})();
