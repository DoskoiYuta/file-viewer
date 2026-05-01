(function () {
  const layout = document.getElementById("layout");
  const root = document.documentElement;
  const viewer = document.getElementById("viewer");
  const currentPath = document.getElementById("current-path");
  const VIEW_PREFIX = "/view/";

  function setDisabled(id, off) {
    const el = document.getElementById(id);
    if (el) el.disabled = !!off;
  }

  // ---- Theme ----
  const themeKey = "file-viewer.theme";
  function applyTheme(t) {
    root.dataset.theme = t;
    const resolved = t === "auto"
      ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
      : t;
    root.dataset.themeResolved = resolved;
    root.dataset.colorMode = resolved;
    setDisabled("md-css-light", resolved !== "light");
    setDisabled("md-css-dark", resolved !== "dark");
    setDisabled("hl-css-light", resolved !== "light");
    setDisabled("hl-css-dark", resolved !== "dark");
    if (window.__mermaid) {
      window.__mermaid.initialize({ startOnLoad: false, theme: resolved === "dark" ? "dark" : "default" });
      // Force re-render so existing diagrams pick up the new theme.
      viewer.querySelectorAll('pre.mermaid[data-processed="1"]').forEach((el) => {
        el.dataset.processed = "0";
        el.innerHTML = "";
      });
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
      const code = el.dataset.source || el.textContent;
      if (!el.dataset.source) el.dataset.source = code;
      const id = "mmd-" + Date.now() + "-" + i;
      window.__mermaid.render(id, code).then(({ svg }) => {
        el.innerHTML = svg;
        el.dataset.processed = "1";
        addMermaidZoomButton(el);
      }).catch((err) => {
        el.innerHTML = '<div class="hint">mermaid: ' + (err && err.message ? err.message : "render error") + "</div>";
      });
    });
  }

  function addMermaidZoomButton(el) {
    if (el.querySelector(":scope > .mermaid-zoom-btn")) return;
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "mermaid-zoom-btn";
    btn.title = "Zoom";
    btn.setAttribute("aria-label", "Zoom diagram");
    btn.innerHTML = "&#x2922;"; // diagonal arrows
    btn.addEventListener("click", (ev) => {
      ev.stopPropagation();
      ev.preventDefault();
      openMermaidModal(el);
    });
    el.appendChild(btn);
  }

  function openMermaidModal(el) {
    const svg = el.querySelector("svg");
    if (!svg) return;
    const overlay = document.createElement("div");
    overlay.className = "mermaid-overlay";

    const stage = document.createElement("div");
    stage.className = "mermaid-overlay-stage";

    const clone = svg.cloneNode(true);
    clone.removeAttribute("style");
    clone.style.maxWidth = "none";
    clone.style.maxHeight = "none";
    clone.style.width = "auto";
    clone.style.height = "auto";
    clone.style.transformOrigin = "0 0";
    stage.appendChild(clone);

    let scale = 1;
    let tx = 0, ty = 0;
    function apply() {
      clone.style.transform = `translate(${tx}px, ${ty}px) scale(${scale})`;
    }

    function fit() {
      // Fit svg to stage
      const sb = stage.getBoundingClientRect();
      let vb = svg.viewBox && svg.viewBox.baseVal;
      let w = (vb && vb.width) || svg.getBoundingClientRect().width || 1;
      let h = (vb && vb.height) || svg.getBoundingClientRect().height || 1;
      const pad = 24;
      const fx = (sb.width - pad * 2) / w;
      const fy = (sb.height - pad * 2) / h;
      scale = Math.max(0.1, Math.min(fx, fy));
      tx = (sb.width - w * scale) / 2;
      ty = (sb.height - h * scale) / 2;
      // Make svg natural size match viewBox so transform math works
      clone.setAttribute("width", w);
      clone.setAttribute("height", h);
      apply();
    }

    stage.addEventListener("wheel", (ev) => {
      ev.preventDefault();
      const rect = stage.getBoundingClientRect();
      const mx = ev.clientX - rect.left;
      const my = ev.clientY - rect.top;
      const factor = ev.deltaY > 0 ? 0.9 : 1.1;
      const newScale = Math.max(0.1, Math.min(scale * factor, 20));
      // Zoom around cursor
      tx = mx - (mx - tx) * (newScale / scale);
      ty = my - (my - ty) * (newScale / scale);
      scale = newScale;
      apply();
    }, { passive: false });

    let dragging = false, lastX = 0, lastY = 0;
    stage.addEventListener("pointerdown", (ev) => {
      dragging = true; lastX = ev.clientX; lastY = ev.clientY;
      stage.setPointerCapture(ev.pointerId);
    });
    stage.addEventListener("pointermove", (ev) => {
      if (!dragging) return;
      tx += ev.clientX - lastX;
      ty += ev.clientY - lastY;
      lastX = ev.clientX; lastY = ev.clientY;
      apply();
    });
    stage.addEventListener("pointerup", (ev) => {
      dragging = false;
      try { stage.releasePointerCapture(ev.pointerId); } catch (_) {}
    });

    const tools = document.createElement("div");
    tools.className = "mermaid-overlay-tools";
    tools.innerHTML = `
      <button type="button" data-act="zoom-in" title="Zoom in" aria-label="Zoom in">+</button>
      <button type="button" data-act="zoom-out" title="Zoom out" aria-label="Zoom out">−</button>
      <button type="button" data-act="fit" title="Fit" aria-label="Fit">⤢</button>
      <button type="button" data-act="close" title="Close" aria-label="Close">×</button>
    `;
    tools.addEventListener("click", (ev) => {
      const b = ev.target.closest("button");
      if (!b) return;
      ev.stopPropagation();
      const act = b.dataset.act;
      if (act === "close") closeMermaidModal(overlay);
      else if (act === "fit") fit();
      else if (act === "zoom-in" || act === "zoom-out") {
        const sb = stage.getBoundingClientRect();
        const mx = sb.width / 2, my = sb.height / 2;
        const factor = act === "zoom-in" ? 1.2 : 1 / 1.2;
        const ns = Math.max(0.1, Math.min(scale * factor, 20));
        tx = mx - (mx - tx) * (ns / scale);
        ty = my - (my - ty) * (ns / scale);
        scale = ns;
        apply();
      }
    });

    overlay.appendChild(stage);
    overlay.appendChild(tools);
    overlay.addEventListener("click", (ev) => {
      if (ev.target === overlay) closeMermaidModal(overlay);
    });

    document.body.appendChild(overlay);
    document.body.classList.add("modal-open");

    function onKey(ev) {
      if (ev.key === "Escape") closeMermaidModal(overlay);
    }
    overlay._onKey = onKey;
    document.addEventListener("keydown", onKey);

    // Defer fit until layout is computed.
    requestAnimationFrame(fit);
  }

  function closeMermaidModal(overlay) {
    if (overlay._onKey) document.removeEventListener("keydown", overlay._onKey);
    overlay.remove();
    document.body.classList.remove("modal-open");
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
