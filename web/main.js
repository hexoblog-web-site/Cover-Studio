// 封面工坊 Web 版前端：参数变更实时预览（HTTP 接口）+ 下载。
const PRESET_COLORS = [
  "#3B82F6", "#8B5CF6", "#EC4899", "#EF4444", "#F97316", "#EAB308",
  "#22C55E", "#10B981", "#0D9488", "#06B6D4", "#0EA5E9", "#6366F1",
  "#84CC16", "#14B8A6", "#A855F7", "#F43F5E", "#64748B", "#1E293B"
];
const ICON_COLORS = PRESET_COLORS;

const state = {
  left: "生成", right: "流程", bg: "#3B82F6", icon: "hexagon",
  fontColor: "#FFFFFF", fontSize: 128, iconColor: "#FFFFFF",
  iconScale: 100, glow: 55, iconOnTop: false,
  bg2: "", gradDir: 0, gradEnable: false,
  iconFrame: false, monoIcon: false, textShadow: false,
  icons: [], labelMap: {}
};

const $ = (id) => document.getElementById(id);

function req() {
  return {
    left: state.left, right: state.right, bg: state.bg, icon: state.icon,
    fontColor: state.fontColor, fontSize: state.fontSize,
    iconColor: state.iconColor, iconScale: state.iconScale / 100,
    glow: state.glow / 100, iconOnTop: state.iconOnTop,
    bg2: state.gradEnable ? state.bg2 : "", gradDir: state.gradDir,
    iconFrame: state.iconFrame, monoIcon: state.monoIcon, textShadow: state.textShadow
  };
}

let previewTimer = null;
function schedulePreview(delay = 90) {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(doPreview, delay);
}

// 用 POST + blob URL 预览：参数放 body，避免 URL 过长；blob 用完释放。
let lastBlobURL = null;
async function doPreview() {
  try {
    const res = await fetch("/api/render", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req())
    });
    if (!res.ok) {
      const j = await res.json().catch(() => ({}));
      setStatus(j.msg || ("HTTP " + res.status), "err");
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    $("preview").src = url;
    if (lastBlobURL) URL.revokeObjectURL(lastBlobURL);
    lastBlobURL = url;
  } catch (e) {
    setStatus("预览失败: " + e, "err");
  }
}

function setStatus(msg, kind) {
  const el = $("status");
  el.textContent = msg;
  el.className = "status" + (kind ? " " + kind : "");
}

function buildSwatches(boxId, palette, activeColor, onPick) {
  const box = $(boxId);
  box.innerHTML = "";
  palette.forEach((c) => {
    const d = document.createElement("div");
    d.className = "color-dot" + (c.toLowerCase() === activeColor.toLowerCase() ? " active" : "");
    d.style.background = c;
    d.title = c;
    d.onclick = () => onPick(c);
    box.appendChild(d);
  });
}
function buildColors() { buildSwatches("colors", PRESET_COLORS, state.bg, setColor); }
function buildIconColors() { buildSwatches("iconColors", ICON_COLORS, state.iconColor, setIconColor); }
function syncSwatches(boxId, c) {
  $(boxId).querySelectorAll(".color-dot").forEach((d) => {
    d.classList.toggle("active", d.title.toLowerCase() === c.toLowerCase());
  });
}
function setColor(c) {
  state.bg = c; $("bgPicker").value = c; $("bgText").value = c;
  $("curColor").textContent = c; syncSwatches("colors", c); schedulePreview();
}
function setFontColor(c) {
  state.fontColor = c; $("fontPicker").value = c; $("fontText").value = c; schedulePreview();
}
function setIconColor(c) {
  state.iconColor = c; $("iconPicker").value = c; $("iconText").value = c;
  syncSwatches("iconColors", c); schedulePreview();
}
function setLayer(iconOnTop) {
  state.iconOnTop = iconOnTop;
  $("layerText").classList.toggle("active", !iconOnTop);
  $("layerIcon").classList.toggle("active", iconOnTop);
  $("curLayer").textContent = iconOnTop ? "图标在上" : "文字在上";
  setStatus(iconOnTop ? "已切换为：图标在上" : "已切换为：文字在上", "ok");
  schedulePreview();
}
function bindColorText(textId, getter, setter) {
  $(textId).addEventListener("change", (e) => {
    let v = e.target.value.trim();
    if (!/^#[0-9a-fA-F]{6}$/.test(v)) {
      setStatus("颜色格式应为 #RRGGBB", "err");
      e.target.value = getter();
      return;
    }
    setter(v);
  });
}

function buildIcons() {
  const box = $("icons");
  box.innerHTML = "";
  state.icons.forEach((it) => {
    state.labelMap[it.key] = it.label;
    const wrap = document.createElement("div");
    wrap.className = "icon-cell";
    const b = document.createElement("button");
    b.className = "icon-btn" + (it.key === state.icon ? " active" : "");
    b.textContent = it.label;
    b.title = it.key;
    b.onclick = () => {
      state.icon = it.key;
      $("curIcon").textContent = it.label;
      document.querySelectorAll(".icon-btn").forEach((x) => x.classList.remove("active"));
      b.classList.add("active");
      schedulePreview();
    };
    wrap.appendChild(b);
    if (it.key.startsWith("custom:")) {
      const del = document.createElement("button");
      del.className = "icon-del";
      del.textContent = "×";
      del.title = "删除此图标";
      del.onclick = async (e) => {
        e.stopPropagation();
        try {
          const res = await fetch("/api/icons/delete", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ key: it.key })
          });
          if ((await res.json()).ok) {
            state.icons = state.icons.filter((x) => x.key !== it.key);
            if (state.icon === it.key) { state.icon = "hexagon"; $("curIcon").textContent = "六边形"; }
            buildIcons(); schedulePreview(); setStatus("已删除图标", "ok");
          }
        } catch (err) { setStatus("删除失败: " + err, "err"); }
      };
      wrap.appendChild(del);
    }
    box.appendChild(wrap);
  });
}

async function loadIcons() {
  const res = await fetch("/api/icons");
  const j = await res.json();
  state.icons = j.icons || [];
  buildIcons();
}

async function init() {
  state.bg2 = $("bg2Picker").value;
  buildColors(); buildIconColors();
  try { await loadIcons(); } catch (e) { setStatus("图标列表加载失败: " + e, "err"); }

  $("left").addEventListener("input", (e) => { state.left = e.target.value; schedulePreview(); });
  $("right").addEventListener("input", (e) => { state.right = e.target.value; schedulePreview(); });
  $("bgPicker").addEventListener("input", (e) => setColor(e.target.value));
  bindColorText("bgText", () => state.bg, setColor);
  $("fontPicker").addEventListener("input", (e) => setFontColor(e.target.value));
  bindColorText("fontText", () => state.fontColor, setFontColor);
  $("fontSize").addEventListener("input", (e) => {
    state.fontSize = Number(e.target.value);
    $("fontSizeVal").textContent = state.fontSize + " px";
    schedulePreview();
  });
  $("iconPicker").addEventListener("input", (e) => setIconColor(e.target.value));
  bindColorText("iconText", () => state.iconColor, setIconColor);
  $("iconScale").addEventListener("input", (e) => {
    state.iconScale = Number(e.target.value);
    $("iconScaleVal").textContent = state.iconScale + "%";
    schedulePreview();
  });
  $("glow").addEventListener("input", (e) => {
    state.glow = Number(e.target.value);
    $("glowVal").textContent = "光晕 " + state.glow + "%";
    schedulePreview();
  });
  $("layerText").addEventListener("click", () => setLayer(false));
  $("layerIcon").addEventListener("click", () => setLayer(true));
  $("gradEnable").addEventListener("change", (e) => { state.gradEnable = e.target.checked; schedulePreview(); });
  $("bg2Picker").addEventListener("input", (e) => {
    state.bg2 = e.target.value;
    if (!state.gradEnable) { state.gradEnable = true; $("gradEnable").checked = true; }
    schedulePreview();
  });
  $("gradDir").addEventListener("change", (e) => { state.gradDir = Number(e.target.value); schedulePreview(); });
  $("textShadow").addEventListener("change", (e) => { state.textShadow = e.target.checked; schedulePreview(); });
  $("iconFrame").addEventListener("change", (e) => { state.iconFrame = e.target.checked; schedulePreview(); });
  $("monoIcon").addEventListener("change", (e) => { state.monoIcon = e.target.checked; schedulePreview(); });

  // 上传自定义图标
  $("importIconBtn").addEventListener("click", () => $("iconFile").click());
  $("iconFile").addEventListener("change", async (e) => {
    const f = e.target.files[0];
    if (!f) return;
    setStatus("正在上传…", "");
    const fd = new FormData();
    fd.append("file", f);
    try {
      const res = await fetch("/api/icons/upload", { method: "POST", body: fd });
      const j = await res.json();
      if (j.ok) {
        await loadIcons();
        state.icon = j.key;
        $("curIcon").textContent = "我的图标";
        $("iconFrame").checked = true; state.iconFrame = true;
        setLayer(true);
        document.querySelectorAll(".icon-btn").forEach((x) => x.classList.remove("active"));
        const newBtn = [...document.querySelectorAll(".icon-btn")].find((x) => x.title === j.key);
        if (newBtn) newBtn.classList.add("active");
        setStatus("已上传: " + j.name, "ok");
        schedulePreview();
      } else setStatus(j.msg || "上传失败", "err");
    } catch (err) { setStatus("上传失败: " + err, "err"); }
    e.target.value = "";
  });

  // 下载：把当前预览的 PNG 存为文件
  $("saveBtn").addEventListener("click", async () => {
    setStatus("正在生成…", "");
    try {
      const res = await fetch("/api/render", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req())
      });
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      const safe = (state.left + "-" + state.right).replace(/[\\/:*?"<>|]/g, "");
      a.download = "cover-" + (safe || "image") + ".png";
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      setStatus("已下载封面", "ok");
    } catch (e) { setStatus("下载失败: " + e, "err"); }
  });

  await doPreview();
}

window.addEventListener("DOMContentLoaded", init);
