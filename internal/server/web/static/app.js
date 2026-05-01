(function () {
  const layout = document.getElementById("layout");
  const root = document.documentElement;
  const viewer = document.getElementById("viewer");
  const currentPath = document.getElementById("current-path");
  const VIEW_PREFIX = "/view/";

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

  // ---- Routing via URL pathname ----
  function relFromURL() {
    const p = window.location.pathname;
    if (!p.startsWith(VIEW_PREFIX)) return "";
    try {
      return decodeURI(p.slice(VIEW_PREFIX.length));
    } catch (_) {
      return p.slice(VIEW_PREFIX.length);
    }
  }
  function urlForRel(rel) {
    return VIEW_PREFIX + rel.split("/").map(encodeURIComponent).join("/");
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
        const target = urlForRel(rel) + window.location.search;
        if (target !== window.location.pathname + window.location.search) {
          history.pushState({ rel }, "", target);
        }
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

  // Intercept clicks on any internal /view/ link (sidebar tree, search hits, markdown links).
  document.body.addEventListener("click", (ev) => {
    if (ev.defaultPrevented) return;
    if (ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
    const a = ev.target.closest("a");
    if (!a) return;
    const href = a.getAttribute("href");
    if (!href) return;
    let u;
    try { u = new URL(a.href, window.location.href); } catch (_) { return; }
    if (u.origin !== window.location.origin) return;
    if (!u.pathname.startsWith(VIEW_PREFIX)) return;
    if (u.pathname === window.location.pathname && u.hash) return; // anchor on same page
    ev.preventDefault();
    let rel;
    try { rel = decodeURI(u.pathname.slice(VIEW_PREFIX.length)); }
    catch (_) { rel = u.pathname.slice(VIEW_PREFIX.length); }
    loadFile(rel).then(() => {
      if (u.hash) {
        const t = document.getElementById(decodeURIComponent(u.hash.slice(1)));
        if (t) t.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    });
  });

  window.addEventListener("popstate", () => loadFile(relFromURL(), false));

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
  document.body.addEventListener("htmx:afterSwap", () => markActive(relFromURL()));

  // ---- SSE hot reload ----
  function connectSSE() {
    const es = new EventSource("/api/events");
    es.addEventListener("reload", () => {
      htmx.trigger(document.body, "fv:reload");
      const cur = relFromURL();
      if (cur) loadFile(cur, false);
    });
    es.onerror = () => { es.close(); setTimeout(connectSSE, 1500); };
  }
  connectSSE();

  // Initial load
  document.addEventListener("DOMContentLoaded", () => {
    const f = relFromURL();
    if (f) loadFile(f, false);
  });
})();
