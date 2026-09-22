(() => {
  const state = {
    file: null,
    toastTimer: null,
    view: "preview",
    settings: {
      defaultView: "preview",
      rememberRecentFiles: true,
      recentFilesLimit: 10,
    },
  };

  const elements = {};

  document.addEventListener("DOMContentLoaded", async () => {
    elements.settingsButton = document.getElementById("settingsButton");
    elements.openButton = document.getElementById("openButton");
    elements.exportButton = document.getElementById("exportButton");
    elements.emptyDetails = document.getElementById("emptyDetails");
    elements.fileDetails = document.getElementById("fileDetails");
    elements.fileName = document.getElementById("fileName");
    elements.filePath = document.getElementById("filePath");
    elements.fileSize = document.getElementById("fileSize");
    elements.fileModified = document.getElementById("fileModified");
    elements.suggestedOutput = document.getElementById("suggestedOutput");
    elements.recentEmpty = document.getElementById("recentEmpty");
    elements.recentList = document.getElementById("recentList");
    elements.clearRecentButton = document.getElementById("clearRecentButton");
    elements.viewLabel = document.getElementById("viewLabel");
    elements.documentLabel = document.getElementById("documentLabel");
    elements.previewTab = document.getElementById("previewTab");
    elements.sourceTab = document.getElementById("sourceTab");
    elements.previewView = document.getElementById("previewView");
    elements.previewScroll = document.getElementById("previewScroll");
    elements.previewContent = document.getElementById("previewContent");
    elements.outlineEmpty = document.getElementById("outlineEmpty");
    elements.outlineList = document.getElementById("outlineList");
    elements.sourceView = document.getElementById("sourceView");
    elements.statusText = document.getElementById("statusText");
    elements.versionLabel = document.getElementById("versionLabel");
    elements.toast = document.getElementById("toast");
    elements.settingsModal = document.getElementById("settingsModal");
    elements.settingsCloseButton = document.getElementById("settingsCloseButton");
    elements.settingsCancelButton = document.getElementById("settingsCancelButton");
    elements.settingsSaveButton = document.getElementById("settingsSaveButton");
    elements.defaultViewSelect = document.getElementById("defaultViewSelect");
    elements.rememberRecentCheckbox = document.getElementById("rememberRecentCheckbox");
    elements.recentLimitSelect = document.getElementById("recentLimitSelect");

    elements.settingsButton.addEventListener("click", openSettings);
    elements.openButton.addEventListener("click", openMarkdown);
    elements.exportButton.addEventListener("click", exportDOCX);
    elements.clearRecentButton.addEventListener("click", clearRecentFiles);
    elements.recentList.addEventListener("click", openRecentFile);
    elements.previewTab.addEventListener("click", () => setView("preview"));
    elements.sourceTab.addEventListener("click", () => setView("source"));
    elements.outlineList.addEventListener("click", navigateOutline);
    elements.previewContent.addEventListener("click", handlePreviewClick);
    elements.settingsCloseButton.addEventListener("click", closeSettings);
    elements.settingsCancelButton.addEventListener("click", closeSettings);
    elements.settingsSaveButton.addEventListener("click", saveSettings);
    elements.rememberRecentCheckbox.addEventListener("change", updateSettingsControls);
    elements.settingsModal.addEventListener("click", (event) => {
      if (event.target === elements.settingsModal) {
        closeSettings();
      }
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !elements.settingsModal.classList.contains("hidden")) {
        closeSettings();
      }
    });

    if (window.runtime && window.runtime.OnFileDrop) {
      window.runtime.OnFileDrop((_x, _y, paths) => {
        if (!paths || paths.length === 0) {
          return;
        }
        if (paths.length > 1) {
          showToast("Drop one Markdown file at a time.", true);
          return;
        }
        openMarkdownPath(paths[0], "Opening dropped Markdown...");
      }, true);
    }

    if (window.runtime && window.runtime.EventsOn) {
      window.runtime.EventsOn("mdtranscode:open-markdown", (path) => {
        if (!path) {
          return;
        }
        openMarkdownPath(path, "Opening Markdown from Windows...");
      });
    }

    await loadInitialState();
  });

  async function loadInitialState() {
    try {
      const version = await window.go.main.App.Version();
      elements.versionLabel.textContent = version;
    } catch (_error) {
      elements.versionLabel.textContent = "development build";
    }

    try {
      state.settings = await window.go.main.App.Settings();
      populateSettingsControls();
    } catch (error) {
      showToast(`Could not load settings: ${normalizeError(error)}`, true);
    }

    await refreshRecentFiles();

    try {
      const startupFile = await window.go.main.App.StartupFile();
      if (startupFile) {
        renderFile(startupFile);
        await refreshRecentFiles();
        return;
      }
    } catch (error) {
      handleError(error);
      return;
    }
    setStatus("Ready");
  }

  async function openMarkdown() {
    setBusy("Opening Markdown...");
    try {
      const file = await window.go.main.App.OpenMarkdown();
      if (file) {
        renderFile(file);
        await refreshRecentFiles();
      } else {
        setStatus("Ready");
      }
    } catch (error) {
      handleError(error);
    }
  }

  async function openMarkdownPath(path, busyText = "Opening Markdown...") {
    setBusy(busyText);
    try {
      const file = await window.go.main.App.OpenMarkdownPath(path);
      renderFile(file);
      await refreshRecentFiles();
    } catch (error) {
      handleError(error);
    }
  }

  async function openRecentFile(event) {
    const button = event.target.closest(".recent-item");
    if (!button || button.disabled || !button.dataset.path) {
      return;
    }
    await openMarkdownPath(button.dataset.path, "Opening recent Markdown...");
  }

  async function refreshRecentFiles() {
    try {
      const recent = await window.go.main.App.RecentFiles();
      renderRecentFiles(recent || []);
    } catch (error) {
      renderRecentFiles([]);
      showToast(`Could not load recent documents: ${normalizeError(error)}`, true);
    }
  }

  function renderRecentFiles(recent) {
    elements.recentList.replaceChildren();
    const remember = Boolean(state.settings.rememberRecentFiles);
    elements.clearRecentButton.classList.toggle("hidden", !remember || recent.length === 0);

    if (!remember) {
      elements.recentEmpty.textContent = "Recent documents are disabled in Settings.";
      elements.recentEmpty.classList.remove("hidden");
      elements.recentList.classList.add("hidden");
      return;
    }
    if (recent.length === 0) {
      elements.recentEmpty.textContent = "No recent documents yet.";
      elements.recentEmpty.classList.remove("hidden");
      elements.recentList.classList.add("hidden");
      return;
    }

    elements.recentEmpty.classList.add("hidden");
    elements.recentList.classList.remove("hidden");
    recent.forEach((item) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "recent-item";
      button.dataset.path = item.path;
      button.disabled = !item.available;
      button.title = item.available ? item.path : `File not found: ${item.path}`;

      const name = document.createElement("span");
      name.className = "recent-name";
      name.textContent = item.name || "Markdown document";
      const path = document.createElement("span");
      path.className = "recent-path";
      path.textContent = item.available ? item.path : `${item.path} (missing)`;
      button.append(name, path);
      elements.recentList.appendChild(button);
    });
  }

  async function clearRecentFiles() {
    try {
      await window.go.main.App.ClearRecentFiles();
      renderRecentFiles([]);
      showToast("Recent documents cleared.", false);
    } catch (error) {
      handleError(error);
    }
  }

  async function exportDOCX() {
    if (!state.file) {
      return;
    }

    setBusy("Exporting Word document...");
    try {
      const outputPath = await window.go.main.App.ExportDOCX(state.file.path);
      if (!outputPath) {
        setStatus("Export cancelled");
        return;
      }
      setStatus(`Created ${outputPath}`);
      showToast(`Word document created: ${outputPath}`, false);
    } catch (error) {
      handleError(error);
    }
  }

  function renderFile(file) {
    state.file = file;
    elements.emptyDetails.classList.add("hidden");
    elements.fileDetails.classList.remove("hidden");
    elements.exportButton.disabled = false;
    elements.fileName.textContent = file.name;
    elements.filePath.textContent = file.path;
    elements.fileSize.textContent = formatBytes(file.size);
    elements.fileModified.textContent = formatDate(file.modified);
    elements.suggestedOutput.textContent = file.suggestedOutput;
    elements.documentLabel.textContent = file.name;
    elements.sourceView.textContent = file.source;
    elements.previewContent.innerHTML = file.previewHtml || "<p>No rendered content.</p>";
    renderOutline(file.outline || []);
    elements.previewScroll.scrollTop = 0;
    setView(state.settings.defaultView || "preview");
    setStatus(`Opened ${file.name}`);
  }

  function renderOutline(outline) {
    elements.outlineList.replaceChildren();
    if (!outline || outline.length === 0) {
      elements.outlineEmpty.textContent = "This document has no headings.";
      elements.outlineEmpty.classList.remove("hidden");
      elements.outlineList.classList.add("hidden");
      return;
    }

    elements.outlineEmpty.classList.add("hidden");
    elements.outlineList.classList.remove("hidden");
    outline.forEach((item) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "outline-item";
      button.dataset.headingId = item.id;
      button.style.setProperty("--outline-level", String(Math.max(1, Math.min(6, item.level || 1))));
      button.textContent = item.text || "Untitled section";
      button.title = item.text || "Untitled section";
      elements.outlineList.appendChild(button);
    });
  }

  function navigateOutline(event) {
    const button = event.target.closest(".outline-item");
    if (!button) {
      return;
    }
    const heading = document.getElementById(button.dataset.headingId);
    if (!heading) {
      return;
    }
    setView("preview");
    heading.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function handlePreviewClick(event) {
    const link = event.target.closest("a");
    if (!link) {
      return;
    }
    event.preventDefault();
    const href = link.getAttribute("href");
    if (href) {
      showToast(`Link in preview: ${href}`, false);
    }
  }

  function setView(view) {
    state.view = view === "source" ? "source" : "preview";
    const previewActive = state.view === "preview";
    elements.previewView.classList.toggle("hidden", !previewActive);
    elements.sourceView.classList.toggle("hidden", previewActive);
    elements.previewTab.classList.toggle("active", previewActive);
    elements.sourceTab.classList.toggle("active", !previewActive);
    elements.previewTab.setAttribute("aria-selected", String(previewActive));
    elements.sourceTab.setAttribute("aria-selected", String(!previewActive));
    elements.viewLabel.textContent = previewActive ? "Rendered preview" : "Markdown source";
  }

  function openSettings() {
    populateSettingsControls();
    elements.settingsModal.classList.remove("hidden");
    elements.settingsModal.setAttribute("aria-hidden", "false");
    elements.defaultViewSelect.focus();
  }

  function closeSettings() {
    elements.settingsModal.classList.add("hidden");
    elements.settingsModal.setAttribute("aria-hidden", "true");
    elements.settingsButton.focus();
  }

  function populateSettingsControls() {
    elements.defaultViewSelect.value = state.settings.defaultView === "source" ? "source" : "preview";
    elements.rememberRecentCheckbox.checked = Boolean(state.settings.rememberRecentFiles);
    const limit = String(state.settings.recentFilesLimit || 10);
    if (["5", "10", "15", "20"].includes(limit)) {
      elements.recentLimitSelect.value = limit;
    } else {
      elements.recentLimitSelect.value = "10";
    }
    updateSettingsControls();
  }

  function updateSettingsControls() {
    elements.recentLimitSelect.disabled = !elements.rememberRecentCheckbox.checked;
  }

  async function saveSettings() {
    elements.settingsSaveButton.disabled = true;
    try {
      state.settings = await window.go.main.App.SaveSettings({
        defaultView: elements.defaultViewSelect.value,
        rememberRecentFiles: elements.rememberRecentCheckbox.checked,
        recentFilesLimit: Number.parseInt(elements.recentLimitSelect.value, 10),
      });
      closeSettings();
      await refreshRecentFiles();
      showToast("Settings saved.", false);
    } catch (error) {
      showToast(normalizeError(error), true);
    } finally {
      elements.settingsSaveButton.disabled = false;
    }
  }

  function setBusy(message) {
    elements.settingsButton.disabled = true;
    elements.openButton.disabled = true;
    elements.exportButton.disabled = true;
    setStatusText(message);
  }

  function setStatus(message) {
    setStatusText(message);
    elements.settingsButton.disabled = false;
    elements.openButton.disabled = false;
    elements.exportButton.disabled = !state.file;
  }

  function setStatusText(message) {
    elements.statusText.textContent = message;
  }

  function handleError(error) {
    const message = normalizeError(error);
    setStatus("Error");
    showToast(message, true);
  }

  function showToast(message, isError) {
    clearTimeout(state.toastTimer);
    elements.toast.textContent = message;
    elements.toast.classList.toggle("error", Boolean(isError));
    elements.toast.classList.remove("hidden");
    state.toastTimer = setTimeout(() => {
      elements.toast.classList.add("hidden");
    }, 6000);
  }

  function normalizeError(error) {
    if (!error) {
      return "An unexpected error occurred.";
    }
    if (typeof error === "string") {
      return error;
    }
    if (error.message) {
      return error.message;
    }
    return String(error);
  }

  function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes < 1024) {
      return `${bytes || 0} bytes`;
    }
    const units = ["KB", "MB", "GB"];
    let value = bytes / 1024;
    let unit = units[0];
    for (let i = 1; i < units.length && value >= 1024; i += 1) {
      value /= 1024;
      unit = units[i];
    }
    return `${value.toFixed(value >= 10 ? 1 : 2)} ${unit}`;
  }

  function formatDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return value;
    }
    return date.toLocaleString();
  }
})();
