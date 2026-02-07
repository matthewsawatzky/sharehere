(() => {
  const boot = window.SHAREHERE_BOOT || { basePath: "" };
  const basePath = boot.basePath === "/" ? "" : (boot.basePath || "");
  const qs = new URLSearchParams(window.location.search);

  const els = {
    sessionInfo: document.getElementById("sessionInfo"),
    breadcrumbs: document.getElementById("breadcrumbs"),
    fileRows: document.getElementById("fileRows"),
    previewPane: document.getElementById("previewPane"),
    searchInput: document.getElementById("searchInput"),
    sortSelect: document.getElementById("sortSelect"),
    refreshBtn: document.getElementById("refreshBtn"),
    uploadPanel: document.getElementById("uploadPanel"),
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

  const state = {
    me: null,
    entries: [],
    path: qs.get("path") || "",
    selectedRelPath: ""
  };

  function isStateChange(method) {
    return ["POST", "PUT", "PATCH", "DELETE"].includes((method || "GET").toUpperCase());
  }

  async function api(path, opts = {}) {
    const method = (opts.method || "GET").toUpperCase();
    const headers = Object.assign({ "Accept": "application/json" }, opts.headers || {});
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
    if (!theme || !theme.css_variables) return;
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
    localStorage.setItem("sharehere_remote", JSON.stringify(payload));
    renderCommandsForSelection();
  }

  function restoreRemoteParams() {
    const raw = localStorage.getItem("sharehere_remote");
    if (!raw) return;
    try {
      const data = JSON.parse(raw);
      els.remoteUser.value = data.remoteUser || "";
      els.remoteHost.value = data.remoteHost || "";
      els.remotePort.value = data.remotePort || "22";
      els.remoteBase.value = data.remoteBase || "";
    } catch (_) {
      // ignore
    }
  }

  function formatSize(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    const units = ["KB", "MB", "GB", "TB"];
    let n = bytes / 1024;
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
      n /= 1024;
      i++;
    }
    return `${n.toFixed(1)} ${units[i]}`;
  }

  function breadcrumbButton(label, path) {
    const btn = document.createElement("button");
    btn.textContent = label;
    btn.addEventListener("click", () => navigate(path));
    return btn;
  }

  function renderBreadcrumbs(items) {
    els.breadcrumbs.innerHTML = "";
    if (!items || !items.length) {
      els.breadcrumbs.appendChild(breadcrumbButton("/", ""));
      return;
    }
    items.forEach((crumb) => {
      els.breadcrumbs.appendChild(breadcrumbButton(crumb.name, crumb.path));
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

  function filterEntries(entries) {
    const q = (els.searchInput.value || "").trim().toLowerCase();
    if (!q) return entries;
    return entries.filter((e) => e.name.toLowerCase().includes(q));
  }

  function copyText(value) {
    navigator.clipboard.writeText(value).catch(() => {});
  }

  function createActionButton(label, handler, style = "ghost") {
    const b = document.createElement("button");
    b.className = `button ${style}`;
    b.textContent = label;
    b.addEventListener("click", handler);
    return b;
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
    const entry = state.entries.find((v) => v.relPath === state.selectedRelPath);
    if (!entry) return;
    const commands = renderCommands(entry.relPath, entry.isDir);
    els.commandsPane.textContent = commands;
    els.commandText.textContent = commands;
  }

  async function openShare(entry) {
    const expiry = window.prompt("Expiry duration", "24h");
    if (!expiry) return;
    const mode = window.prompt("Mode: browse, download, upload", "browse") || "browse";
    try {
      const payload = { path: entry.relPath, expiry, mode };
      const result = await api("/api/share/create", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      copyText(result.url);
      window.alert(`Share link created and copied:\n${result.url}`);
    } catch (err) {
      window.alert(String(err.message || err));
    }
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

  function rowFor(entry) {
    const tr = document.createElement("tr");
    const nameCell = document.createElement("td");
    const link = document.createElement("a");
    link.href = "#";
    link.textContent = entry.isDir ? `${entry.name}/` : entry.name;
    link.addEventListener("click", (e) => {
      e.preventDefault();
      if (entry.isDir) {
        navigate(entry.relPath);
      } else {
        preview(entry);
      }
    });
    nameCell.appendChild(link);

    const sizeCell = document.createElement("td");
    sizeCell.textContent = entry.isDir ? "-" : formatSize(entry.size);

    const modCell = document.createElement("td");
    modCell.textContent = new Date(entry.modTime).toLocaleString();

    const actions = document.createElement("td");
    const actionRow = document.createElement("div");
    actionRow.className = "row";

    const download = document.createElement("a");
    download.className = "button ghost";
    download.textContent = "Download";
    download.href = `${basePath}/api/download?path=${encodeURIComponent(entry.relPath)}`;
    actionRow.appendChild(download);

    if (entry.isDir) {
      const zip = document.createElement("a");
      zip.className = "button ghost";
      zip.textContent = "ZIP";
      zip.href = `${basePath}/api/zip?path=${encodeURIComponent(entry.relPath)}`;
      actionRow.appendChild(zip);
    }

    actionRow.appendChild(createActionButton("Copy Name", () => copyText(entry.name)));
    actionRow.appendChild(createActionButton("Copy Path", () => copyText(entry.relPath || "")));
    actionRow.appendChild(createActionButton("Commands", () => {
      state.selectedRelPath = entry.relPath;
      renderCommandsForSelection();
      els.commandModal.showModal();
    }));

    if (state.me?.permissions?.canShare) {
      actionRow.appendChild(createActionButton("Share", () => openShare(entry)));
    }

    if (state.me?.permissions?.canDelete) {
      actionRow.appendChild(createActionButton("Delete", async () => {
        if (!window.confirm(`Delete ${entry.name}?`)) return;
        await api("/api/delete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ path: entry.relPath })
        });
        await loadList(state.path);
      }));
    }

    if (state.me?.permissions?.canRename) {
      actionRow.appendChild(createActionButton("Rename", async () => {
        const next = window.prompt("New name", entry.name);
        if (!next || next === entry.name) return;
        await api("/api/rename", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ path: entry.relPath, newName: next })
        });
        await loadList(state.path);
      }));
    }

    actions.appendChild(actionRow);
    tr.appendChild(nameCell);
    tr.appendChild(sizeCell);
    tr.appendChild(modCell);
    tr.appendChild(actions);
    return tr;
  }

  function renderRows() {
    els.fileRows.innerHTML = "";
    const filtered = filterEntries(sortEntries(state.entries));
    filtered.forEach((entry) => els.fileRows.appendChild(rowFor(entry)));
  }

  async function loadList(pathValue) {
    const data = await api(`/api/list?path=${encodeURIComponent(pathValue || "")}`);
    state.path = data.path || "";
    state.entries = data.entries || [];
    renderBreadcrumbs(data.breadcrumbs || []);
    renderRows();

    qs.set("path", state.path);
    const query = qs.toString();
    const next = `${window.location.pathname}${query ? "?" + query : ""}`;
    history.replaceState({}, "", next);
  }

  async function uploadFiles(fileList) {
    if (!fileList.length) return;
    const form = new FormData();
    form.append("path", state.path || "");
    Array.from(fileList).forEach((f) => form.append("files", f));

    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${basePath}/api/upload`);
    if (state.me?.csrfToken) {
      xhr.setRequestHeader("X-CSRF-Token", state.me.csrfToken);
    }

    xhr.upload.onprogress = (evt) => {
      if (!evt.lengthComputable) return;
      const pct = Math.round((evt.loaded / evt.total) * 100);
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
      els.uploadPanel.classList.add("hidden");
      return;
    }
    els.uploadPanel.classList.remove("hidden");
    const dz = els.dropZone;

    dz.addEventListener("dragover", (e) => {
      e.preventDefault();
      dz.classList.add("drag-over");
    });

    dz.addEventListener("dragleave", () => dz.classList.remove("drag-over"));

    dz.addEventListener("drop", (e) => {
      e.preventDefault();
      dz.classList.remove("drag-over");
      uploadFiles(e.dataTransfer.files);
    });

    els.fileInput.addEventListener("change", () => uploadFiles(els.fileInput.files));
  }

  async function loadMe() {
    const me = await api("/api/me");
    state.me = me;
    applyTheme(me.theme);
    const role = me.authenticated ? `${me.username} (${me.role})` : `Guest (${me.guestMode})`;
    els.sessionInfo.textContent = `${role} · share root: ${me.rootPath}`;
    els.logoutCsrf.value = me.csrfToken || "";
    if (me.authenticated) {
      els.logoutForm.classList.remove("hidden");
    }
    if (me.permissions?.canAdmin) {
      els.adminLink.classList.remove("hidden");
    }
  }

  function bindEvents() {
    els.searchInput.addEventListener("input", renderRows);
    els.sortSelect.addEventListener("change", renderRows);
    els.refreshBtn.addEventListener("click", () => navigate(state.path));
    [els.remoteUser, els.remoteHost, els.remotePort, els.remoteBase].forEach((el) => {
      el.addEventListener("change", saveRemoteParams);
      el.addEventListener("blur", saveRemoteParams);
    });
    els.copyCmd.addEventListener("click", () => copyText(els.commandText.textContent || ""));
    els.closeCmd.addEventListener("click", () => els.commandModal.close());
  }

  async function init() {
    restoreRemoteParams();
    bindEvents();
    await loadMe();
    setupUploadUI();
    await loadList(state.path);
  }

  init().catch((err) => {
    els.previewPane.textContent = `Failed to load: ${err.message || err}`;
  });
})();
