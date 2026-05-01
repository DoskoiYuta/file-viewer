(function () {
  const layout = document.getElementById("layout");
  const root = document.documentElement;
  const viewer = document.getElementById("viewer");
  const currentPath = document.getElementById("current-path");

  // ---- Theme ----
  const themeKey = "file-viewer.theme";
  function applyTheme(t) {
    root.dataset.theme = t;
    const resolved = t === "auto"
      ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
      : t;
    root.dataset.themeResolved = resolved;
    if (window.__mermaid) {
      window.__mermaid.initialize({ startOnLoad: false, theme: resolved === "dark" ? "dark" : "default" });
      renderMermaid();
    }
  }
  applyTheme(localStorage.getItem(themeKey) || "auto");
  document.getElementById("toggle-theme").addEventListener("click", () => {
    const cur = root.dataset.theme || "auto";
    const next = cur === "auto" ? "light" : cur === "light" ? "dark" : "auto";
    localStorage.setItem(themeKey, next);
    applyTheme(next);
  });
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if ((root.dataset.theme || "auto") === "auto") applyTheme("auto");
  });

  // ---- Sidebar toggle ----
  const sbKey = "file-viewer.sidebar";
  function applySidebar(open) {
    layout.classList.toggle("sidebar-open", open);
    localStorage.setItem(sbKey, open ? "1" : "0");
  }
  applySidebar(localStorage.getItem(sbKey) !== "0");
  document.getElementById("toggle-sidebar").addEventListener("click", () => {
    applySidebar(!layout.classList.contains("sidebar-open"));
  });
  document.getElementById("open-sidebar").addEventListener("click", () => applySidebar(true));

  // ---- Routing via ?file= ----
  function currentFileFromURL() {
    const u = new URL(window.location.href);
    return u.searchParams.get("file") || "";
  }

  async function loadFile(rel, push = true) {
    if (!rel) {
      viewer.innerHTML = '<div class="empty">Select a file from the sidebar.</div>';
      currentPath.textContent = "";
      markActive("");
      return;
    }
    currentPath.textContent = rel;
    markActive(rel);
    try {
      const res = await fetch("/api/view?file=" + encodeURIComponent(rel));
      const html = await res.text();
      viewer.innerHTML = html;
      renderMermaid();
      highlightCode();
      if (push) {
        const u = new URL(window.location.href);
        u.searchParams.set("file", rel);
        history.pushState({ file: rel }, "", u);
      }
    } catch (e) {
      viewer.innerHTML = '<div class="empty">Failed to load.</div>';
    }
  }

  function markActive(rel) {
    document.querySelectorAll(".file-link").forEach((el) => {
      el.classList.toggle("active", el.dataset.path === rel);
    });
  }

  document.body.addEventListener("click", (ev) => {
    const a = ev.target.closest("a.file-link");
    if (!a) return;
    ev.preventDefault();
    const rel = a.dataset.path;
    loadFile(rel);
  });

  window.addEventListener("popstate", () => loadFile(currentFileFromURL(), false));

  // ---- Mermaid + highlight ----
  function renderMermaid() {
    if (!window.__mermaid) return;
    const blocks = viewer.querySelectorAll("pre.mermaid");
    blocks.forEach((el, i) => {
      if (el.dataset.processed === "1") return;
      const code = el.textContent;
      const id = "mmd-" + Date.now() + "-" + i;
      window.__mermaid.render(id, code).then(({ svg }) => {
        el.innerHTML = svg;
        el.dataset.processed = "1";
      }).catch((err) => {
        el.innerHTML = '<div class="hint">mermaid: ' + (err && err.message ? err.message : "render error") + "</div>";
      });
    });
  }
  function highlightCode() {
    if (window.hljs) {
      viewer.querySelectorAll("pre code").forEach((el) => {
        if (el.parentElement.classList.contains("mermaid")) return;
        window.hljs.highlightElement(el);
      });
    }
  }

  // After tree HTMX swaps, re-mark active.
  document.body.addEventListener("htmx:afterSwap", () => markActive(currentFileFromURL()));

  // ---- SSE hot reload ----
  function connectSSE() {
    const es = new EventSource("/api/events");
    es.addEventListener("reload", () => {
      htmx.trigger(document.body, "fv:reload");
      const cur = currentFileFromURL();
      if (cur) loadFile(cur, false);
    });
    es.onerror = () => { es.close(); setTimeout(connectSSE, 1500); };
  }
  connectSSE();

  // Initial load
  document.addEventListener("DOMContentLoaded", () => {
    const f = currentFileFromURL();
    if (f) loadFile(f, false);
  });
})();
