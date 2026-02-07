(() => {
  const boot = window.SHAREHERE_BOOT || { basePath: "" };
  const basePath = boot.basePath === "/" ? "" : (boot.basePath || "");

  const els = {
    guestMode: document.getElementById("guestMode"),
    maxUploadSizeMB: document.getElementById("maxUploadSizeMB"),
    uploadAllowRegex: document.getElementById("uploadAllowRegex"),
    uploadDenyRegex: document.getElementById("uploadDenyRegex"),
    uploadSubdir: document.getElementById("uploadSubdir"),
    collisionPolicy: document.getElementById("collisionPolicy"),
    defaultShareExpiry: document.getElementById("defaultShareExpiry"),
    allowDelete: document.getElementById("allowDelete"),
    allowRename: document.getElementById("allowRename"),
    readOnly: document.getElementById("readOnly"),
    theme: document.getElementById("theme"),
    themeOverridesJSON: document.getElementById("themeOverridesJSON"),
    virusScanCommand: document.getElementById("virusScanCommand"),
    saveSettings: document.getElementById("saveSettings"),
    settingsStatus: document.getElementById("settingsStatus"),
    newUsername: document.getElementById("newUsername"),
    newRole: document.getElementById("newRole"),
    newPassword: document.getElementById("newPassword"),
    createUser: document.getElementById("createUser"),
    userRows: document.getElementById("userRows"),
    linkRows: document.getElementById("linkRows"),
    refreshAudit: document.getElementById("refreshAudit"),
    auditRows: document.getElementById("auditRows")
  };

  let csrfToken = "";

  function isStateChange(method) {
    return ["POST", "PUT", "PATCH", "DELETE"].includes((method || "GET").toUpperCase());
  }

  async function api(path, opts = {}) {
    const method = (opts.method || "GET").toUpperCase();
    const headers = Object.assign({ "Accept": "application/json" }, opts.headers || {});
    if (isStateChange(method) && csrfToken) {
      headers["X-CSRF-Token"] = csrfToken;
    }
    const res = await fetch(basePath + path, Object.assign({}, opts, { method, headers }));
    if (!res.ok) {
      const txt = await res.text();
      throw new Error(txt || `request failed ${res.status}`);
    }
    const contentType = res.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      return res.json();
    }
    return res.text();
  }

  async function loadThemes() {
    const result = await api("/api/themes");
    els.theme.innerHTML = "";
    result.themes.forEach((t) => {
      const opt = document.createElement("option");
      opt.value = t.name;
      opt.textContent = `${t.label} (${t.name})`;
      els.theme.appendChild(opt);
    });
  }

  async function loadSettings() {
    const result = await api("/api/admin/settings");
    const s = result.settings;
    els.guestMode.value = s.guest_mode;
    els.maxUploadSizeMB.value = s.max_upload_size_mb;
    els.uploadAllowRegex.value = s.upload_allow_regex;
    els.uploadDenyRegex.value = s.upload_deny_regex;
    els.uploadSubdir.value = s.upload_subdir;
    els.collisionPolicy.value = s.collision_policy;
    els.defaultShareExpiry.value = s.default_share_expiry;
    els.allowDelete.checked = !!s.allow_delete;
    els.allowRename.checked = !!s.allow_rename;
    els.readOnly.checked = !!s.read_only;
    els.theme.value = s.theme;
    els.themeOverridesJSON.value = s.theme_overrides_json || "{}";
    els.virusScanCommand.value = s.virus_scan_command || "";
  }

  async function saveSettings() {
    const payload = {
      guest_mode: els.guestMode.value,
      max_upload_size_mb: Number(els.maxUploadSizeMB.value || 1024),
      upload_allow_regex: els.uploadAllowRegex.value,
      upload_deny_regex: els.uploadDenyRegex.value,
      upload_subdir: els.uploadSubdir.value,
      collision_policy: els.collisionPolicy.value,
      default_share_expiry: els.defaultShareExpiry.value,
      allow_delete: els.allowDelete.checked,
      allow_rename: els.allowRename.checked,
      read_only: els.readOnly.checked,
      theme: els.theme.value,
      theme_overrides_json: els.themeOverridesJSON.value,
      virus_scan_command: els.virusScanCommand.value
    };
    await api("/api/admin/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    els.settingsStatus.textContent = "Settings saved";
    setTimeout(() => (els.settingsStatus.textContent = ""), 2200);
  }

  function rowForUser(u) {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td>${u.username}</td><td>${u.role}</td><td>${u.disabled ? "disabled" : "active"}</td><td></td>`;
    const actions = tr.children[3];
    const wrap = document.createElement("div");
    wrap.className = "row";

    const passwd = document.createElement("button");
    passwd.className = "button ghost";
    passwd.textContent = "Set password";
    passwd.onclick = async () => {
      const value = window.prompt(`New password for ${u.username}`);
      if (!value) return;
      await api("/api/admin/users/password", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: u.username, password: value })
      });
      await loadUsers();
    };

    const toggle = document.createElement("button");
    toggle.className = "button ghost";
    toggle.textContent = u.disabled ? "Enable" : "Disable";
    toggle.onclick = async () => {
      await api("/api/admin/users/disable", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: u.username, disabled: !u.disabled })
      });
      await loadUsers();
    };

    const remove = document.createElement("button");
    remove.className = "button ghost";
    remove.textContent = "Remove";
    remove.onclick = async () => {
      if (!window.confirm(`Remove ${u.username}?`)) return;
      await api("/api/admin/users/delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: u.username })
      });
      await loadUsers();
    };

    wrap.append(passwd, toggle, remove);
    actions.appendChild(wrap);
    return tr;
  }

  async function loadUsers() {
    const result = await api("/api/admin/users");
    els.userRows.innerHTML = "";
    result.users.forEach((u) => els.userRows.appendChild(rowForUser(u)));
  }

  async function createUser() {
    const payload = {
      username: els.newUsername.value,
      password: els.newPassword.value,
      role: els.newRole.value
    };
    await api("/api/admin/users/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    els.newUsername.value = "";
    els.newPassword.value = "";
    await loadUsers();
  }

  function rowForLink(link) {
    const tr = document.createElement("tr");
    const expires = new Date(link.expires_at).toLocaleString();
    const last = link.last_accessed ? new Date(link.last_accessed).toLocaleString() : "-";
    tr.innerHTML = `<td><code>${link.token}</code></td><td><code>${link.path}</code></td><td>${link.mode}</td><td>${expires}</td><td>${last}</td><td></td>`;
    const actions = tr.children[5];
    const wrap = document.createElement("div");
    wrap.className = "row";

    const open = document.createElement("a");
    open.className = "button ghost";
    open.textContent = "Open";
    open.href = `${window.location.origin}${basePath}/s/${link.token}`;
    open.target = "_blank";

    const revoke = document.createElement("button");
    revoke.className = "button ghost";
    revoke.textContent = "Revoke";
    revoke.onclick = async () => {
      await api("/api/share/revoke", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: link.token })
      });
      await loadLinks();
    };

    wrap.append(open, revoke);
    actions.appendChild(wrap);
    return tr;
  }

  async function loadLinks() {
    const result = await api("/api/admin/links");
    els.linkRows.innerHTML = "";
    result.links.forEach((l) => els.linkRows.appendChild(rowForLink(l)));
  }

  async function loadAudit() {
    const result = await api("/api/admin/audit?limit=200");
    els.auditRows.innerHTML = "";
    result.logs.forEach((l) => {
      const li = document.createElement("li");
      const ts = new Date(l.created_at).toLocaleString();
      const actor = l.username || "anonymous";
      li.textContent = `[${ts}] ${actor} · ${l.action} · ${l.target}`;
      els.auditRows.appendChild(li);
    });
  }

  async function init() {
    const me = await api("/api/me");
    csrfToken = me.csrfToken || "";
    if (!me.permissions?.canAdmin) {
      window.location.href = `${basePath}/`;
      return;
    }
    await loadThemes();
    await loadSettings();
    await loadUsers();
    await loadLinks();
    await loadAudit();

    els.saveSettings.onclick = () => saveSettings().catch((e) => window.alert(e.message || e));
    els.createUser.onclick = () => createUser().catch((e) => window.alert(e.message || e));
    els.refreshAudit.onclick = () => loadAudit().catch((e) => window.alert(e.message || e));
  }

  init().catch((e) => {
    window.alert(`failed to initialize admin view: ${e.message || e}`);
  });
})();
