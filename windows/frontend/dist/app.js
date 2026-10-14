(() => {
  const state = {
    file: null,
    toastTimer: null,
    view: "preview",
    find: { matches: [], active: -1 },
    settings: {
      defaultView: "preview",
      rememberRecentFiles: true,
      recentFilesLimit: 10,
      watcher: {},
    },
  };

  const elements = {};

  document.addEventListener("DOMContentLoaded", async () => {
    for (const id of ["findButton", "findBar", "findInput", "findCount", "findPrevious", "findNext", "findClose"]) {
      elements[id] = document.getElementById(id);
    }
    elements.appShell = document.getElementById("appShell");
    elements.settingsButton = document.getElementById("settingsButton");
    elements.openButton = document.getElementById("openButton");
    elements.exportButton = document.getElementById("exportButton");
    elements.exportPDFButton = document.getElementById("exportPDFButton");
    for (const id of ["pdfPageSize","pdfOrientation","pdfMarginTop","pdfMarginRight","pdfMarginBottom","pdfMarginLeft","pdfHeaderText","pdfFooterText","pdfPageNumbers","pdfBookmarks","pdfBreakH1"]) elements[id] = document.getElementById(id);
    for (const id of ["wordStyle","wordTemplate","wordBrowseTemplate","wordCoverTitle","wordSubtitle","wordCompany","wordLogoPath","wordBrowseLogo","wordHeader","wordFooter","wordPaper","wordPageNumbers","wordTOC","wordBreakH1","wordLandscape"]) elements[id] = document.getElementById(id);
    elements.wordBrowseTemplate.addEventListener("click", browseWordTemplate);
    elements.wordBrowseLogo.addEventListener("click", browseWordLogo);
    elements.wordStyle.addEventListener("change", updateSettingsControls);
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
    elements.outlinePanel = document.getElementById("outlinePanel");
    elements.outlineSidebarToggle = document.getElementById("outlineSidebarToggle");
    elements.sourceView = document.getElementById("sourceView");
    elements.statusText = document.getElementById("statusText");
    elements.createdOpenButton = document.getElementById("createdOpenButton");
    elements.createdRevealButton = document.getElementById("createdRevealButton");
    elements.versionLabel = document.getElementById("versionLabel");
    elements.toast = document.getElementById("toast");
    elements.settingsFeedback = document.getElementById("settingsFeedback");
    elements.wordTemplate.addEventListener("change", inspectSelectedWordTemplate);
    elements.settingsModal = document.getElementById("settingsModal");
    elements.settingsCloseButton = document.getElementById("settingsCloseButton");
    elements.settingsCancelButton = document.getElementById("settingsCancelButton");
    elements.settingsSaveButton = document.getElementById("settingsSaveButton");
    for (const id of ["openWatchFolderButton", "openOutputFolderButton", "windowsStartupButton", "closeToTrayCheckbox", "openStoreButton", "storeUpdatesSection"]) elements[id] = document.getElementById(id);
    elements.defaultViewSelect = document.getElementById("defaultViewSelect");
    elements.rememberRecentCheckbox = document.getElementById("rememberRecentCheckbox");
    elements.recentLimitSelect = document.getElementById("recentLimitSelect");
    for (const id of ["watcherEnabled", "watcherPaused", "watcherInputPath", "watcherOutputPath", "watcherBrowseInput", "watcherBrowseOutput", "watcherRecursive", "watcherConvertExisting", "watcherStatusLine", "watcherLastScan", "watcherHistory"]) {
      elements[id] = document.getElementById(id);
    }

    elements.findButton.addEventListener("click", openFind);
    elements.findClose.addEventListener("click", closeFind);
    elements.findInput.addEventListener("input", updateFind);
    elements.findNext.addEventListener("click", () => stepFind(1));
    elements.findPrevious.addEventListener("click", () => stepFind(-1));
    elements.findInput.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        stepFind(event.shiftKey ? -1 : 1);
      }
    });
    elements.settingsButton.addEventListener("click", openSettings);
    elements.openButton.addEventListener("click", openMarkdown);
    elements.exportButton.addEventListener("click", exportDOCX);
    elements.exportPDFButton.addEventListener("click", exportPDF);
    elements.createdOpenButton.addEventListener("click", () => runExportAction("OpenLastExportedFile"));
    elements.createdRevealButton.addEventListener("click", () => runExportAction("ShowLastExportedFileInFolder"));
    elements.clearRecentButton.addEventListener("click", clearRecentFiles);
    elements.recentList.addEventListener("click", openRecentFile);
    elements.previewTab.addEventListener("click", () => setView("preview"));
    elements.sourceTab.addEventListener("click", () => setView("source"));
    elements.outlineList.addEventListener("click", navigateOutline);
    elements.outlineSidebarToggle.addEventListener("click", toggleOutlineSidebar);
    elements.previewContent.addEventListener("click", handlePreviewClick);
    elements.settingsCloseButton.addEventListener("click", closeSettings);
    elements.settingsCancelButton.addEventListener("click", closeSettings);
    elements.settingsSaveButton.addEventListener("click", saveSettings);
    elements.rememberRecentCheckbox.addEventListener("change", updateSettingsControls);
    elements.watcherBrowseInput.addEventListener("click", () => browseWatcherFolder("watcherInputPath"));
    elements.watcherBrowseOutput.addEventListener("click", () => browseWatcherFolder("watcherOutputPath"));
    elements.openWatchFolderButton.addEventListener("click", () => runFolderAction("OpenWatchedFolder"));
    elements.openOutputFolderButton.addEventListener("click", () => runFolderAction("OpenConvertedFolder"));
    elements.windowsStartupButton.addEventListener("click", () => runFolderAction("OpenWindowsStartupSettings"));
    elements.openStoreButton.addEventListener("click", () => window.runtime.BrowserOpenURL("https://apps.microsoft.com/detail/9MXCQXBPS8K1"));
    window.setInterval(() => {
      if (!elements.settingsModal.classList.contains("hidden")) refreshWatcherStatus();
    }, 5000);
    document.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === "f") {
        event.preventDefault();
        if (state.file && elements.settingsModal.classList.contains("hidden")) openFind();
        return;
      }
      if (event.key === "Escape" && !elements.findBar.classList.contains("hidden")) {
        event.preventDefault();
        closeFind();
        return;
      }
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
      window.runtime.EventsOn("mdtranscode:open-settings", () => openSettings());
      window.runtime.EventsOn("mdtranscode:tray-error", (message) => showToast(message, true));
      window.runtime.EventsOn("mdtranscode:open-updates", async () => {
        await openSettings();
        elements.storeUpdatesSection.scrollIntoView({ block: "start", behavior: "smooth" });
        elements.openStoreButton.focus({ preventScroll: true });
      });
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
      elements.versionLabel.textContent = "";
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
      setCreatedStatus(outputPath);
      showToast("Word document created.", false);
    } catch (error) {
      handleError(error);
    }
  }

  async function exportPDF() {
    if (!state.file) return;
    setBusy("Exporting PDF...");
    try {
      const path = await window.go.main.App.ExportPDF(state.file.path);
      if (!path) { setStatus("Export cancelled"); return; }
      setCreatedStatus(path);
      showToast("PDF created.", false);
    } catch(error) {handleError(error);}
  }

  function renderFile(file) {
    clearFindMarks();
    state.file = file;
    elements.findButton.disabled = false;
    elements.emptyDetails.classList.add("hidden");
    elements.fileDetails.classList.remove("hidden");
    elements.exportButton.disabled = false;
    elements.exportPDFButton.disabled = false;
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
    if (!elements.findBar.classList.contains("hidden")) updateFind();
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
    // Build a nested outline according to heading levels. When levels skip
    // (H1 -> H3), attach the new heading to its closest existing ancestor.
    const stack = [{ level: 0, children: elements.outlineList, toggle: null }];
    outline.forEach((item, index) => {
      const level = Math.max(1, Math.min(6, Number(item.level) || 1));
      while (stack.length > 1 && stack[stack.length - 1].level >= level) stack.pop();
      const parent = stack[stack.length - 1];
      if (parent.toggle) {
        parent.toggle.hidden = false;
      }
      const node = document.createElement("div");
      node.className = "outline-node";
      const row = document.createElement("div");
      row.className = "outline-row";
      row.style.setProperty("--outline-level", String(level));
      const toggle = document.createElement("button");
      toggle.type = "button";
      toggle.className = "outline-toggle";
      toggle.hidden = true;
      toggle.textContent = "▾";
      toggle.title = "Collapse section";
      toggle.setAttribute("aria-label", `Collapse ${item.text || "Untitled section"}`);
      toggle.setAttribute("aria-expanded", "true");
      const childId = `outline-children-${index}`;
      toggle.setAttribute("aria-controls", childId);
      const button = document.createElement("button");
      button.type = "button";
      button.className = "outline-item";
      button.dataset.headingId = item.id;
      button.textContent = item.text || "Untitled section";
      button.dataset.headingLabel = `H${level}`;
      button.title = `H${level}: ${item.text || "Untitled section"}`;
      button.setAttribute("aria-label", button.title);
      const children = document.createElement("div");
      children.className = "outline-children";
      children.id = childId;
      const toggleSlot = document.createElement("span");
      toggleSlot.className = "outline-toggle-slot";
      toggleSlot.appendChild(toggle);
      row.append(toggleSlot, button);
      node.append(row, children);
      parent.children.appendChild(node);
      stack.push({ level, children, toggle });
    });
  }

  function setOutlineToggle(toggle, expanded) {
    const node = toggle.closest(".outline-node");
    const children = node && node.querySelector(":scope > .outline-children");
    if (!children) return;
    children.hidden = !expanded;
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.textContent = expanded ? "▾" : "▸";
    const title = node.querySelector(":scope > .outline-row > .outline-item")?.textContent || "section";
    toggle.title = `${expanded ? "Collapse" : "Expand"} section`;
    toggle.setAttribute("aria-label", `${expanded ? "Collapse" : "Expand"} ${title}`);
  }

  // The sidebar toggle changes its WIDTH, not the document heading hierarchy.
  // Expanded is the default on every launch. In compact mode CSS shows all
  // heading levels as short H1/H2/H3 labels while retaining navigation.
  function toggleOutlineSidebar() {
    const compact = elements.outlinePanel.classList.toggle("is-collapsed");
    elements.previewView.classList.toggle("outline-compact", compact);
    elements.outlineSidebarToggle.setAttribute("aria-expanded", String(!compact));
    const label = compact ? "Expand outline sidebar" : "Collapse outline sidebar";
    elements.outlineSidebarToggle.setAttribute("aria-label", label);
    elements.outlineSidebarToggle.title = label;
  }

  function navigateOutline(event) {
    const toggle = event.target.closest(".outline-toggle");
    if (toggle && elements.outlineList.contains(toggle)) {
      setOutlineToggle(toggle, toggle.getAttribute("aria-expanded") !== "true");
      return;
    }
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
    if (!elements.findBar.classList.contains("hidden")) updateFind();
  }

  function openFind() {
    if (!state.file) return;
    elements.findBar.classList.remove("hidden");
    elements.findInput.focus();
    elements.findInput.select();
    updateFind();
  }

  function closeFind() {
    elements.findBar.classList.add("hidden");
    clearFindMarks();
    elements.findButton.focus();
  }

  function clearFindMarks() {
    for (const root of [elements.previewContent, elements.sourceView]) {
      if (!root) continue;
      root.querySelectorAll("mark.md-find-match").forEach((mark) => {
        mark.replaceWith(document.createTextNode(mark.textContent));
      });
      root.normalize();
    }
    state.find.matches = [];
    state.find.active = -1;
  }

  function updateFind() {
    clearFindMarks();
    const term = elements.findInput.value;
    if (!state.file || !term) {
      elements.findCount.textContent = "0 matches";
      elements.findPrevious.disabled = true;
      elements.findNext.disabled = true;
      return;
    }
    const root = state.view === "preview" ? elements.previewContent : elements.sourceView;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        if (!node.nodeValue || !node.nodeValue.toLocaleLowerCase().includes(term.toLocaleLowerCase())) return NodeFilter.FILTER_REJECT;
        if (node.parentElement.closest("script, style, mark")) return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      },
    });
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    for (const node of nodes) {
      const value = node.nodeValue;
      const lower = value.toLocaleLowerCase();
      const query = term.toLocaleLowerCase();
      const fragment = document.createDocumentFragment();
      let pos = 0;
      let match;
      while ((match = lower.indexOf(query, pos)) !== -1) {
        fragment.append(document.createTextNode(value.slice(pos, match)));
        const mark = document.createElement("mark");
        mark.className = "md-find-match";
        mark.textContent = value.slice(match, match + term.length);
        fragment.append(mark);
        state.find.matches.push(mark);
        pos = match + term.length;
      }
      fragment.append(document.createTextNode(value.slice(pos)));
      node.replaceWith(fragment);
    }
    elements.findPrevious.disabled = state.find.matches.length === 0;
    elements.findNext.disabled = state.find.matches.length === 0;
    if (state.find.matches.length) stepFind(1);
    else elements.findCount.textContent = "0 matches";
  }

  function stepFind(direction) {
    const matches = state.find.matches;
    if (!matches.length) return;
    if (state.find.active >= 0) matches[state.find.active].classList.remove("current");
    state.find.active = (state.find.active + direction + matches.length) % matches.length;
    const mark = matches[state.find.active];
    mark.classList.add("current");
    mark.scrollIntoView({ block: "center", behavior: "auto" });
    elements.findCount.textContent = `${state.find.active + 1} of ${matches.length}`;
  }

  async function openSettings() {
    try { state.settings = await window.go.main.App.Settings(); }
    catch (error) { showToast(`Could not load settings: ${normalizeError(error)}`, true); }
    populateSettingsControls();
    refreshWatcherStatus();
    elements.appShell.classList.add("settings-open");
    elements.settingsModal.classList.remove("hidden");
    elements.settingsModal.setAttribute("aria-hidden", "false");
    clearSettingsFeedback();
    if (elements.wordStyle.value === "custom" && elements.wordTemplate.value.trim()) {
      await inspectSelectedWordTemplate();
    }
    elements.defaultViewSelect.focus();
  }

  function closeSettings() {
    elements.appShell.classList.remove("settings-open");
    elements.settingsModal.classList.add("hidden");
    elements.settingsModal.setAttribute("aria-hidden", "true");
    clearSettingsFeedback();
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
    const watcher = state.settings.watcher || {};
    elements.watcherEnabled.checked = Boolean(watcher.enabled);
    elements.watcherPaused.checked = Boolean(watcher.paused);
    elements.watcherInputPath.value = watcher.watchFolder || "";
    elements.watcherOutputPath.value = watcher.outputFolder || "";
    elements.watcherRecursive.checked = Boolean(watcher.recursive);
    elements.watcherConvertExisting.checked = Boolean(watcher.convertExisting);
    elements.closeToTrayCheckbox.checked = Boolean(state.settings.closeToTray);
    const pdf = state.settings.pdf || {};
    elements.pdfPageSize.value = pdf.pageSize || "Letter";
    elements.pdfOrientation.value = pdf.orientation || "portrait";
    for (const side of ["Top","Right","Bottom","Left"]) elements[`pdfMargin${side}`].value=pdf[`margin${side}`] ?? 19;
    elements.pdfHeaderText.value=pdf.headerText || "";
    elements.pdfFooterText.value=pdf.footerText || "";
    elements.pdfPageNumbers.checked=pdf.pageNumbers !== false;
    elements.pdfBookmarks.checked=pdf.bookmarks !== false;
    elements.pdfBreakH1.checked=Boolean(pdf.pageBreakBeforeH1);
    const word = state.settings.docx || {};
    elements.wordStyle.value = word.style || "standard";
    elements.wordTemplate.value = word.templatePath || "";
    elements.wordCoverTitle.value = word.coverTitle || "";
    elements.wordSubtitle.value = word.subtitle || "";
    elements.wordCompany.value = word.company || "";
    elements.wordLogoPath.value = word.logoPath || "";
    elements.wordHeader.value = word.headerText || "";
    elements.wordFooter.value = word.footerText || "";
    elements.wordPaper.value = word.paperSize || "Letter";
    elements.wordPageNumbers.checked = Boolean(word.pageNumbers);
    elements.wordTOC.checked = Boolean(word.toc);
    elements.wordBreakH1.checked = Boolean(word.breakBeforeH1);
    elements.wordLandscape.checked = Boolean(word.landscape);
    updateSettingsControls();
  }

  async function browseWordTemplate() {
    try {
      const path = await window.go.main.App.BrowseWordTemplate();
      if(path) {elements.wordTemplate.value=path; elements.wordStyle.value="custom";updateSettingsControls();await inspectSelectedWordTemplate();}
    } catch(error) {showToast(normalizeError(error),true);}
  }

  function clearSettingsFeedback() {
    elements.settingsFeedback.textContent = "";
    elements.settingsFeedback.classList.add("hidden");
    elements.settingsFeedback.classList.remove("error", "warning");
  }

  function showSettingsFeedback(message, kind) {
    elements.settingsFeedback.textContent = message;
    elements.settingsFeedback.classList.remove("hidden", "error", "warning");
    elements.settingsFeedback.classList.add(kind === "warning" ? "warning" : "error");
  }

  async function inspectSelectedWordTemplate() {
    clearSettingsFeedback();
    if (elements.wordStyle.value !== "custom" || !elements.wordTemplate.value.trim()) return [];
    try {
      const warnings = await window.go.main.App.InspectWordTemplate(elements.wordTemplate.value.trim());
      if (Array.isArray(warnings) && warnings.length) {
        showSettingsFeedback(`Word template warning: ${warnings.join(" ")}`, "warning");
      }
      return warnings || [];
    } catch (error) {
      // SaveSettings remains the authoritative validation gate.
      showSettingsFeedback(`Word template: ${normalizeError(error)}`, "error");
      return [];
    }
  }

  async function browseWordLogo() {
    try {
      const path = await window.go.main.App.BrowseWordLogo();
      // A cancelled native picker returns an empty path; keep the prior logo.
      if (path) elements.wordLogoPath.value = path;
    } catch (error) {
      showToast(normalizeError(error), true);
    }
  }

  async function browseWatcherFolder(id) {
    try {
      const selected = await window.go.main.App.BrowseWatcherFolder();
      if (selected) elements[id].value = selected;
    } catch (error) {
      showToast(normalizeError(error), true);
    }
  }

  async function refreshWatcherStatus() {
    try {
      const status = await window.go.main.App.WatcherStatus();
      if (elements.settingsModal.classList.contains("hidden")) return;
      const condition = status.running ? "Monitoring" : status.paused ? "Paused" : status.enabled ? "Stopped" : "Disabled";
      elements.watcherStatusLine.textContent = `${condition} · ${status.converted || 0} converted · ${status.pending || 0} settling · ${status.errors || 0} errors`;
      elements.watcherLastScan.textContent = status.lastScan ? `Last scan: ${formatDate(status.lastScan)}` : "No successful scan yet";
      elements.watcherHistory.replaceChildren();
      (status.history || []).slice(0, 5).forEach((entry) => {
        const node = document.createElement("div");
        if (entry.result === "error") node.className = "watcher-error";
        node.textContent = `${entry.result}: ${entry.source || "Watcher"}${entry.message ? " — " + entry.message : ""}`;
        elements.watcherHistory.appendChild(node);
      });
    } catch (error) {
      elements.watcherStatusLine.textContent = `Status unavailable: ${normalizeError(error)}`;
    }
  }

  function updateSettingsControls() {
    elements.recentLimitSelect.disabled = !elements.rememberRecentCheckbox.checked;
    elements.wordTemplate.disabled = elements.wordStyle.value !== "custom";
    elements.wordBrowseTemplate.disabled = elements.wordStyle.value !== "custom";
    if (elements.wordStyle.value !== "custom") clearSettingsFeedback();
  }

  async function saveSettings() {
    elements.settingsSaveButton.disabled = true;
    try {
      state.settings = await window.go.main.App.SaveSettings({
        defaultView: elements.defaultViewSelect.value,
        rememberRecentFiles: elements.rememberRecentCheckbox.checked,
        recentFilesLimit: Number.parseInt(elements.recentLimitSelect.value, 10),
        // Preserve legacy saved flag without initiating GitHub checks.
        updateNotifications: Boolean(state.settings.updateNotifications),
        closeToTray: elements.closeToTrayCheckbox.checked,
        docx: {
          style: elements.wordStyle.value,
          templatePath: elements.wordStyle.value==="custom" ? elements.wordTemplate.value.trim():"",
          coverTitle:elements.wordCoverTitle.value,
          subtitle:elements.wordSubtitle.value,
          company:elements.wordCompany.value,
          logoPath:elements.wordLogoPath.value.trim(),
          headerText:elements.wordHeader.value,
          footerText:elements.wordFooter.value,
          paperSize:elements.wordPaper.value,
          pageNumbers:elements.wordPageNumbers.checked,
          toc:elements.wordTOC.checked,
          breakBeforeH1:elements.wordBreakH1.checked,
          landscape:elements.wordLandscape.checked,
        },
        pdf: {
          pageSize:elements.pdfPageSize.value,
          orientation:elements.pdfOrientation.value,
          marginTop:Number(elements.pdfMarginTop.value),
          marginRight:Number(elements.pdfMarginRight.value),
          marginBottom:Number(elements.pdfMarginBottom.value),
          marginLeft:Number(elements.pdfMarginLeft.value),
          headerText:elements.pdfHeaderText.value,
          footerText:elements.pdfFooterText.value,
          pageNumbers:elements.pdfPageNumbers.checked,
          bookmarks:elements.pdfBookmarks.checked,
          pageBreakBeforeH1:elements.pdfBreakH1.checked,
        },
        watcher: {
          enabled: elements.watcherEnabled.checked,
          paused: elements.watcherPaused.checked,
          watchFolder: elements.watcherInputPath.value.trim(),
          outputFolder: elements.watcherOutputPath.value.trim(),
          recursive: elements.watcherRecursive.checked,
          convertExisting: elements.watcherConvertExisting.checked,
        },
      });
      await refreshRecentFiles();
      if (elements.wordStyle.value === "custom") {
        const warnings = await inspectSelectedWordTemplate();
        if (warnings.length) {
          showSettingsFeedback(`Settings saved. Word template warning: ${warnings.join(" ")}`, "warning");
          return;
        }
      }
      closeSettings();
      showToast("Settings saved.", false);
    } catch (error) {
      showToast(normalizeError(error), true);
    } finally {
      elements.settingsSaveButton.disabled = false;
    }
  }

  async function runFolderAction(name) {
    try { await window.go.main.App[name](); }
    catch (error) { showToast(normalizeError(error), true); }
  }

  function setBusy(message) {
    elements.settingsButton.disabled = true;
    elements.openButton.disabled = true;
    elements.exportButton.disabled = true;
    elements.exportPDFButton.disabled = true;
    setStatusText(message);
  }

  function setStatus(message) {
    setStatusText(message);
    elements.settingsButton.disabled = false;
    elements.openButton.disabled = false;
    elements.exportButton.disabled = !state.file;
    elements.exportPDFButton.disabled = !state.file;
  }

  function setStatusText(message) {
    elements.statusText.textContent = message;
    elements.createdOpenButton.classList.add("hidden");
    elements.createdRevealButton.classList.add("hidden");
  }

  function setCreatedStatus(path) {
    setStatus("Created");
    const name = path.split(/[\\/]/).pop() || path;
    elements.createdOpenButton.textContent = name;
    elements.createdOpenButton.title = `Open ${path}`;
    elements.createdOpenButton.setAttribute("aria-label", `Open exported document ${name}`);
    elements.createdRevealButton.title = `Show ${path} in File Explorer`;
    elements.createdOpenButton.classList.remove("hidden");
    elements.createdRevealButton.classList.remove("hidden");
  }

  async function runExportAction(name) {
    try {
      await window.go.main.App[name]();
    } catch (error) {
      showToast(normalizeError(error), true);
    }
  }

  function handleError(error) {
    const message = normalizeError(error);
    setStatus("Error");
    showToast(message, true);
  }

  function showToast(message, isError) {
    if (isError && !elements.settingsModal.classList.contains("hidden")) {
      showSettingsFeedback(message, "error");
      return;
    }
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
