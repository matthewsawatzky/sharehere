(() => {
  const boot = window.SHAREHERE_BOOT || { basePath: "" };
  const basePath = boot.basePath === "/" ? "" : (boot.basePath || "");
  const qs = new URLSearchParams(window.location.search);

  const els = {
    sessionInfo: document.getElementById("sessionInfo"),
    breadcrumbs: document.getElementById("breadcrumbs"),
    fileRows: document.getElementById("fileRows"),
    gridView: document.getElementById("gridView"),
    listView: document.getElementById("listView"),
    entrySummary: document.getElementById("entrySummary"),
    previewPane: document.getElementById("previewPane"),
    searchInput: document.getElementById("searchInput"),
    sortSelect: document.getElementById("sortSelect"),
    refreshBtn: document.getElementById("refreshBtn"),
    showHiddenToggle: document.getElementById("showHiddenToggle"),
    listViewBtn: document.getElementById("listViewBtn"),
    gridViewBtn: document.getElementById("gridViewBtn"),
    uploadPanel: document.getElementById("uploadPanel"),
    toggleUploadBtn: document.getElementById("toggleUploadBtn"),
    downloadFolderLink: document.getElementById("downloadFolderLink"),
    dropZone: document.getElementById("dropZone"),
    fileInput: document.getElementById("fileInput"),
    uploadProgress: document.getElementById("uploadProgress"),
    commandModal: document.getElementById("commandModal"),
    commandText: document.getElementById("commandText"),
    copyCmd: document.getElementById("copyCmd"),
    closeCmd: document.getElementById("closeCmd"),
    commandsPane: document.getElementById("commandsPane"),
    remoteUser: document.getElementById("remoteUser"),
    remoteHost: document.getElementById("remoteHost"),
    remotePort: document.getElementById("remotePort"),
    remoteBase: document.getElementById("remoteBase"),
    logoutForm: document.getElementById("logoutForm"),
    logoutCsrf: document.getElementById("logoutCsrf"),
    adminLink: document.getElementById("adminLink")
  };

  const storageKeys = {
    remote: "sharehere_remote",
    uiPrefs: "sharehere_ui_prefs"
  };

  const state = {
    me: null,
    entries: [],
    path: qs.get("path") || "",
    selectedRelPath: "",
    showHidden: false,
    viewMode: "list",
    uploadVisible: false
  };

  function isStateChange(method) {
    return ["POST", "PUT", "PATCH", "DELETE"].includes((method || "GET").toUpperCase());
  }

  async function api(path, opts = {}) {
    const method = (opts.method || "GET").toUpperCase();
    const headers = Object.assign({ Accept: "application/json" }, opts.headers || {});
    if (isStateChange(method) && state.me?.csrfToken) {
      headers["X-CSRF-Token"] = state.me.csrfToken;
    }

    const res = await fetch(basePath + path, Object.assign({}, opts, { method, headers }));
    if (res.status === 401) {
      window.location.href = `${basePath}/login`;
      throw new Error("unauthorized");
    }
    if (!res.ok) {
      const body = await res.text();
      throw new Error(body || `request failed: ${res.status}`);
    }
    if (res.status === 204) {
      return null;
    }

    const contentType = res.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      return res.json();
    }
    return res.text();
  }

  function applyTheme(theme) {
    if (!theme || !theme.css_variables) {
      return;
    }
    Object.entries(theme.css_variables).forEach(([k, v]) => {
      document.documentElement.style.setProperty(k, v);
    });
  }

  function saveRemoteParams() {
    const payload = {
      remoteUser: els.remoteUser.value,
      remoteHost: els.remoteHost.value,
      remotePort: els.remotePort.value,
      remoteBase: els.remoteBase.value
    };
    localStorage.setItem(storageKeys.remote, JSON.stringify(payload));
    renderCommandsForSelection();
  }

  function restoreRemoteParams() {
    const raw = localStorage.getItem(storageKeys.remote);
    if (!raw) {
      return;
    }
    try {
      const data = JSON.parse(raw);
      els.remoteUser.value = data.remoteUser || "";
      els.remoteHost.value = data.remoteHost || "";
      els.remotePort.value = data.remotePort || "22";
      els.remoteBase.value = data.remoteBase || "";
    } catch (_) {
      // ignore corrupt local storage
    }
  }

  function loadUIPreferences() {
    const raw = localStorage.getItem(storageKeys.uiPrefs);
    if (!raw) {
      return;
    }
    try {
      const data = JSON.parse(raw);
      if (data.viewMode === "grid" || data.viewMode === "list") {
        state.viewMode = data.viewMode;
      }
      state.showHidden = data.showHidden === true;
    } catch (_) {
      // ignore corrupt local storage
    }
  }

  function saveUIPreferences() {
    localStorage.setItem(storageKeys.uiPrefs, JSON.stringify({
      viewMode: state.viewMode,
      showHidden: state.showHidden
    }));
  }

  function setViewMode(mode) {
    state.viewMode = mode === "grid" ? "grid" : "list";
    els.listView.classList.toggle("hidden", state.viewMode !== "list");
    els.gridView.classList.toggle("hidden", state.viewMode !== "grid");
    els.listViewBtn.classList.toggle("active", state.viewMode === "list");
    els.gridViewBtn.classList.toggle("active", state.viewMode === "grid");
    saveUIPreferences();
  }

  function setShowHidden(showHidden) {
    state.showHidden = !!showHidden;
    els.showHiddenToggle.checked = state.showHidden;
    saveUIPreferences();
    renderEntries();
  }

  function setUploadVisibility(visible) {
    state.uploadVisible = !!visible;
    els.uploadPanel.classList.toggle("hidden", !state.uploadVisible);
    if (els.toggleUploadBtn && !els.toggleUploadBtn.classList.contains("hidden")) {
      els.toggleUploadBtn.textContent = state.uploadVisible ? "Hide upload" : "Upload files";
    }
  }

  function refreshPathActions() {
    const encoded = encodeURIComponent(state.path || "");
    els.downloadFolderLink.href = `${basePath}/api/zip?path=${encoded}`;
  }

  function formatSize(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    const units = ["KB", "MB", "GB", "TB"];
    let n = bytes / 1024;
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
      n /= 1024;
      i += 1;
    }
    return `${n.toFixed(1)} ${units[i]}`;
  }

  function copyText(value) {
    navigator.clipboard.writeText(value).catch(() => {});
  }

  function entryIcon(entry) {
    if (entry.isDir) {
      return '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path fill="#bf8700" d="M1.75 2.5h4.07l1.1 1.2H14a.75.75 0 0 1 .75.75v7a1.75 1.75 0 0 1-1.75 1.75H3A1.75 1.75 0 0 1 1.25 11.45V3.25a.75.75 0 0 1 .5-.71Z"></path></svg>';
    }
    return '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path fill="#57606a" d="M3.75 1.5h5.19c.2 0 .39.08.53.22l2.81 2.81c.14.14.22.33.22.53v8.69A1.75 1.75 0 0 1 10.75 15h-7A1.75 1.75 0 0 1 2 13.25v-10.0A1.75 1.75 0 0 1 3.75 1.5Zm5 1.56V5.5h2.44L8.75 3.06Z"></path></svg>';
  }

  function breadcrumbButton(label, relPath) {
    const btn = document.createElement("button");
    btn.className = "gh-crumb-btn";
    btn.textContent = label;
    btn.addEventListener("click", () => navigate(relPath));
    return btn;
  }

  function renderBreadcrumbs(items) {
    els.breadcrumbs.innerHTML = "";
    const crumbs = items && items.length ? items : [{ name: "root", path: "" }];
    crumbs.forEach((crumb, idx) => {
      if (idx > 0) {
        const sep = document.createElement("span");
        sep.className = "gh-breadcrumb-sep";
        sep.textContent = "/";
        els.breadcrumbs.appendChild(sep);
      }
      const label = idx === 0 ? "root" : crumb.name;
      els.breadcrumbs.appendChild(breadcrumbButton(label, crumb.path));
    });
  }

  function sortEntries(entries) {
    const mode = els.sortSelect.value;
    const list = [...entries];
    if (mode === "date") {
      list.sort((a, b) => new Date(b.modTime).getTime() - new Date(a.modTime).getTime());
    } else if (mode === "size") {
      list.sort((a, b) => b.size - a.size);
    } else {
      list.sort((a, b) => a.name.localeCompare(b.name));
    }
    list.sort((a, b) => Number(b.isDir) - Number(a.isDir));
    return list;
  }

  function filterBySearch(entries) {
    const q = (els.searchInput.value || "").trim().toLowerCase();
    if (!q) {
      return entries;
    }
    return entries.filter((entry) => entry.name.toLowerCase().includes(q));
  }

  function isHiddenEntry(entry) {
    return entry.name.startsWith(".");
  }

  function currentEntries() {
    let list = sortEntries(state.entries);
    if (!state.showHidden) {
      list = list.filter((entry) => !isHiddenEntry(entry));
    }
    return filterBySearch(list);
  }

  function renderEntrySummary(visibleCount) {
    const total = state.entries.length;
    const hiddenCount = state.entries.filter(isHiddenEntry).length;
    const current = state.path || "/";

    if (total === 0) {
      els.entrySummary.textContent = `Path ${current} is empty.`;
      return;
    }

    let summary = `${visibleCount} of ${total} items in ${current}`;
    if (!state.showHidden && hiddenCount > 0) {
      summary += ` (${hiddenCount} hidden)`;
    }
    els.entrySummary.textContent = summary;
  }

  function renderCommands(relPath, isDir) {
    const user = els.remoteUser.value || "user";
    const host = els.remoteHost.value || "host";
    const port = els.remotePort.value || "22";
    const base = (els.remoteBase.value || "/").replace(/\/+$/, "");
    const safeRel = relPath || ".";
    const local = `"./${safeRel}"`;
    const remote = `"${base}/${safeRel}"`;
    const recursive = isDir ? "-r " : "";

    return [
      `scp ${recursive}-P ${port} ${local} ${user}@${host}:${remote}`,
      `scp ${recursive}-P ${port} ${user}@${host}:${remote} ${local}`,
      `rsync -avz -e "ssh -p ${port}" ${local} ${user}@${host}:${remote}`,
      `rsync -avz -e "ssh -p ${port}" ${user}@${host}:${remote} ${local}`
    ].join("\n");
  }

  function renderCommandsForSelection() {
    if (!state.selectedRelPath) {
      return;
    }
    const entry = state.entries.find((value) => value.relPath === state.selectedRelPath);
    if (!entry) {
      state.selectedRelPath = "";
      els.commandsPane.textContent = "Select a file or folder to generate scp/rsync commands.";
      els.commandText.textContent = "";
      return;
    }
    const commands = renderCommands(entry.relPath, entry.isDir);
    els.commandsPane.textContent = commands;
    els.commandText.textContent = commands;
  }

  async function openShare(entry) {
    const expiry = window.prompt("Expiry duration", "24h");
    if (!expiry) {
      return;
    }
    const mode = window.prompt("Mode: browse, download, upload", "browse") || "browse";
    const payload = { path: entry.relPath, expiry, mode };
    const result = await api("/api/share/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    copyText(result.url);
    window.alert(`Share link created and copied:\n${result.url}`);
  }

  async function preview(entry) {
    if (entry.isDir) {
      navigate(entry.relPath);
      return;
    }

    try {
      const result = await api(`/api/preview?path=${encodeURIComponent(entry.relPath)}`);
      if (result.type === "image") {
        els.previewPane.innerHTML = `<img alt="preview" src="${basePath}/api/download?path=${encodeURIComponent(entry.relPath)}" />`;
      } else if (result.type === "text") {
        els.previewPane.textContent = result.content;
      } else {
        els.previewPane.textContent = "No preview available for this file type.";
      }
      state.selectedRelPath = entry.relPath;
      renderCommandsForSelection();
    } catch (err) {
      els.previewPane.textContent = String(err.message || err);
    }
  }

  function closeActionMenu(node) {
    const details = node.closest("details");
    if (details) {
      details.open = false;
    }
  }

  function actionButton(label, handler) {
    const btn = document.createElement("button");
    btn.className = "action-item";
    btn.type = "button";
    btn.textContent = label;
    btn.addEventListener("click", async (event) => {
      event.preventDefault();
      event.stopPropagation();
      closeActionMenu(btn);
      try {
        await handler();
      } catch (err) {
        window.alert(String(err.message || err));
      }
    });
    return btn;
  }

  function actionLink(label, href) {
    const a = document.createElement("a");
    a.className = "action-item";
    a.href = href;
    a.textContent = label;
    a.addEventListener("click", () => closeActionMenu(a));
    return a;
  }

  function buildActionMenu(entry) {
    const details = document.createElement("details");
    details.className = "action-menu";

    const summary = document.createElement("summary");
    summary.className = "button ghost action-trigger";
    summary.textContent = "⋯";
    summary.setAttribute("aria-label", "Open actions");

    const menu = document.createElement("div");
    menu.className = "action-menu-items";

    if (entry.isDir) {
      menu.appendChild(actionButton("Open folder", async () => navigate(entry.relPath)));
    } else {
      menu.appendChild(actionButton("Preview file", async () => preview(entry)));
    }

    menu.appendChild(actionLink("Download", `${basePath}/api/download?path=${encodeURIComponent(entry.relPath)}`));

    if (entry.isDir) {
      menu.appendChild(actionLink("Download ZIP", `${basePath}/api/zip?path=${encodeURIComponent(entry.relPath)}`));
    }

    menu.appendChild(actionButton("Copy name", async () => copyText(entry.name)));
    menu.appendChild(actionButton("Copy path", async () => copyText(entry.relPath || "")));
    menu.appendChild(actionButton("Transfer commands", async () => {
      state.selectedRelPath = entry.relPath;
      renderCommandsForSelection();
      els.commandModal.showModal();
    }));

    if (state.me?.permissions?.canShare) {
      menu.appendChild(actionButton("Create share link", async () => openShare(entry)));
    }

    if (state.me?.permissions?.canRename) {
      menu.appendChild(actionButton("Rename", async () => {
        const nextName = window.prompt("New name", entry.name);
        if (!nextName || nextName === entry.name) {
          return;
        }
        await api("/api/rename", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ path: entry.relPath, newName: nextName })
        });
        await loadList(state.path);
      }));
    }

    if (state.me?.permissions?.canDelete) {
      menu.appendChild(actionButton("Delete", async () => {
        if (!window.confirm(`Delete ${entry.name}?`)) {
          return;
        }
        await api("/api/delete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ path: entry.relPath })
        });
        await loadList(state.path);
      }));
    }

    details.appendChild(summary);
    details.appendChild(menu);
    return details;
  }

  function rowFor(entry) {
    const tr = document.createElement("tr");
    tr.className = "gh-row";

    const nameCell = document.createElement("td");
    const fileCell = document.createElement("div");
    fileCell.className = "gh-file-cell";

    const icon = document.createElement("span");
    icon.className = "gh-file-icon";
    icon.innerHTML = entryIcon(entry);

    const link = document.createElement("a");
    link.href = "#";
    link.className = "entry-link";
    link.textContent = entry.isDir ? `${entry.name}/` : entry.name;
    link.addEventListener("click", (event) => {
      event.preventDefault();
      if (entry.isDir) {
        navigate(entry.relPath);
      } else {
        preview(entry);
      }
    });

    fileCell.appendChild(icon);
    fileCell.appendChild(link);

    if (isHiddenEntry(entry)) {
      const hidden = document.createElement("span");
      hidden.className = "gh-hidden-pill";
      hidden.textContent = "hidden";
      fileCell.appendChild(hidden);
    }

    nameCell.appendChild(fileCell);

    const sizeCell = document.createElement("td");
    sizeCell.textContent = entry.isDir ? "-" : formatSize(entry.size);

    const modCell = document.createElement("td");
    modCell.textContent = new Date(entry.modTime).toLocaleString();

    const actionsCell = document.createElement("td");
    actionsCell.appendChild(buildActionMenu(entry));

    tr.appendChild(nameCell);
    tr.appendChild(sizeCell);
    tr.appendChild(modCell);
    tr.appendChild(actionsCell);
    return tr;
  }

  function cardFor(entry) {
    const card = document.createElement("article");
    card.className = "file-card";

    const head = document.createElement("div");
    head.className = "file-card-head";

    const left = document.createElement("div");
    left.className = "gh-file-cell";

    const icon = document.createElement("span");
    icon.className = "gh-file-icon";
    icon.innerHTML = entryIcon(entry);

    const name = document.createElement("a");
    name.href = "#";
    name.className = "file-card-name";
    name.textContent = entry.isDir ? `${entry.name}/` : entry.name;
    name.addEventListener("click", (event) => {
      event.preventDefault();
      if (entry.isDir) {
        navigate(entry.relPath);
      } else {
        preview(entry);
      }
    });

    left.appendChild(icon);
    left.appendChild(name);
    head.appendChild(left);

    const actions = document.createElement("div");
    actions.className = "file-card-actions";
    actions.appendChild(buildActionMenu(entry));
    head.appendChild(actions);

    const meta = document.createElement("p");
    meta.className = "file-card-meta muted small";
    const sizeText = entry.isDir ? "-" : formatSize(entry.size);
    const modText = new Date(entry.modTime).toLocaleString();
    meta.textContent = `Size: ${sizeText} | Updated: ${modText}`;

    card.appendChild(head);
    card.appendChild(meta);
    return card;
  }

  function renderEntries() {
    const entries = currentEntries();
    renderEntrySummary(entries.length);

    els.fileRows.innerHTML = "";
    els.gridView.innerHTML = "";

    entries.forEach((entry) => {
      els.fileRows.appendChild(rowFor(entry));
      els.gridView.appendChild(cardFor(entry));
    });
  }

  async function loadList(pathValue) {
    const data = await api(`/api/list?path=${encodeURIComponent(pathValue || "")}`);
    state.path = data.path || "";
    state.entries = data.entries || [];

    refreshPathActions();
    renderBreadcrumbs(data.breadcrumbs || []);
    renderEntries();

    if (state.selectedRelPath && !state.entries.find((entry) => entry.relPath === state.selectedRelPath)) {
      state.selectedRelPath = "";
      els.commandsPane.textContent = "Select a file or folder to generate scp/rsync commands.";
      els.commandText.textContent = "";
    }

    qs.set("path", state.path);
    const query = qs.toString();
    const nextURL = `${window.location.pathname}${query ? `?${query}` : ""}`;
    history.replaceState({}, "", nextURL);
  }

  async function uploadFiles(fileList) {
    if (!fileList.length) {
      return;
    }

    const form = new FormData();
    form.append("path", state.path || "");
    Array.from(fileList).forEach((file) => form.append("files", file));

    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${basePath}/api/upload`);
    if (state.me?.csrfToken) {
      xhr.setRequestHeader("X-CSRF-Token", state.me.csrfToken);
    }

    xhr.upload.onprogress = (event) => {
      if (!event.lengthComputable) {
        return;
      }
      const pct = Math.round((event.loaded / event.total) * 100);
      els.uploadProgress.textContent = `Uploading ${pct}%`;
    };

    xhr.onload = async () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        els.uploadProgress.textContent = "Upload complete";
        await loadList(state.path);
      } else {
        els.uploadProgress.textContent = `Upload failed: ${xhr.responseText}`;
      }
    };

    xhr.onerror = () => {
      els.uploadProgress.textContent = "Upload failed due to network error";
    };

    xhr.send(form);
  }

  function navigate(nextPath) {
    loadList(nextPath).catch((err) => {
      els.previewPane.textContent = String(err.message || err);
    });
  }

  function setupUploadUI() {
    if (!state.me?.permissions?.canUpload) {
      els.toggleUploadBtn.classList.add("hidden");
      setUploadVisibility(false);
      return;
    }

    els.toggleUploadBtn.classList.remove("hidden");
    setUploadVisibility(false);

    els.dropZone.addEventListener("dragover", (event) => {
      event.preventDefault();
      els.dropZone.classList.add("drag-over");
    });

    els.dropZone.addEventListener("dragleave", () => {
      els.dropZone.classList.remove("drag-over");
    });

    els.dropZone.addEventListener("drop", (event) => {
      event.preventDefault();
      els.dropZone.classList.remove("drag-over");
      uploadFiles(event.dataTransfer.files);
    });

    els.fileInput.addEventListener("change", () => uploadFiles(els.fileInput.files));
  }

  async function loadMe() {
    const me = await api("/api/me");
    state.me = me;

    applyTheme(me.theme);
    const role = me.authenticated ? `${me.username} (${me.role})` : `Guest (${me.guestMode})`;
    els.sessionInfo.textContent = `${role} | root ${me.rootPath}`;
    els.logoutCsrf.value = me.csrfToken || "";

    if (me.authenticated) {
      els.logoutForm.classList.remove("hidden");
    }
    if (me.permissions?.canAdmin) {
      els.adminLink.classList.remove("hidden");
    }
  }

  function bindEvents() {
    els.searchInput.addEventListener("input", renderEntries);
    els.sortSelect.addEventListener("change", renderEntries);
    els.refreshBtn.addEventListener("click", () => navigate(state.path));

    els.showHiddenToggle.addEventListener("change", () => setShowHidden(els.showHiddenToggle.checked));
    els.listViewBtn.addEventListener("click", () => setViewMode("list"));
    els.gridViewBtn.addEventListener("click", () => setViewMode("grid"));
    els.toggleUploadBtn.addEventListener("click", () => setUploadVisibility(!state.uploadVisible));

    [els.remoteUser, els.remoteHost, els.remotePort, els.remoteBase].forEach((el) => {
      el.addEventListener("change", saveRemoteParams);
      el.addEventListener("blur", saveRemoteParams);
    });

    els.copyCmd.addEventListener("click", () => copyText(els.commandText.textContent || ""));
    els.closeCmd.addEventListener("click", () => els.commandModal.close());

    document.addEventListener("click", (event) => {
      document.querySelectorAll(".action-menu[open]").forEach((menu) => {
        if (!menu.contains(event.target)) {
          menu.open = false;
        }
      });
    });
  }

  async function init() {
    loadUIPreferences();
    restoreRemoteParams();
    bindEvents();

    setViewMode(state.viewMode);
    setShowHidden(state.showHidden);

    await loadMe();
    setupUploadUI();
    await loadList(state.path);
  }

  init().catch((err) => {
    els.previewPane.textContent = `Failed to load: ${err.message || err}`;
  });
})();
