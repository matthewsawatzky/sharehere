(() => {
  const boot = window.SHAREHERE_BOOT || { basePath: "", token: "" };
  const basePath = boot.basePath === "/" ? "" : (boot.basePath || "");
  const token = boot.token;
  const dz = document.getElementById("dropZone");
  const fileInput = document.getElementById("fileInput");
  const out = document.getElementById("uploadProgress");

  function upload(files) {
    if (!files || !files.length) return;
    const form = new FormData();
    Array.from(files).forEach((f) => form.append("files", f));
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${basePath}/s/${encodeURIComponent(token)}/upload`);
    xhr.upload.onprogress = (evt) => {
      if (!evt.lengthComputable) return;
      out.textContent = `Uploading ${Math.round((evt.loaded / evt.total) * 100)}%`;
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        out.textContent = "Upload complete";
      } else {
        out.textContent = `Upload failed: ${xhr.responseText}`;
      }
    };
    xhr.onerror = () => {
      out.textContent = "Upload failed due to network error";
    };
    xhr.send(form);
  }

  dz.addEventListener("dragover", (e) => {
    e.preventDefault();
    dz.classList.add("drag-over");
  });
  dz.addEventListener("dragleave", () => dz.classList.remove("drag-over"));
  dz.addEventListener("drop", (e) => {
    e.preventDefault();
    dz.classList.remove("drag-over");
    upload(e.dataTransfer.files);
  });
  fileInput.addEventListener("change", () => upload(fileInput.files));
})();
