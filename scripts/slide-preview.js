#!/usr/bin/env node
/**
 * slide-preview.js
 * 幻灯片修复预览服务 —— 展示所有幻灯片，左右并排对比已替换的帧。
 *
 * 用法: node slide-preview.js <项目目录>
 *   项目目录下需包含 slides_all/ 和 slides_fixed/ 子目录。
 */

const http = require("http");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

// ── 参数解析 ──────────────────────────────────────────────
const projectDir = process.argv[2];
if (!projectDir || !fs.existsSync(projectDir)) {
  console.error("用法: node slide-preview.js <项目目录>");
  console.error("  项目目录下需包含 slides_all/ 和 slides_fixed/ 子目录。");
  process.exit(1);
}

const slidesAllDir = path.join(projectDir, "slides_all");
const slidesFixedDir = path.join(projectDir, "slides_fixed");

if (!fs.existsSync(slidesAllDir)) {
  console.error(`错误: 找不到 ${slidesAllDir}`);
  process.exit(1);
}

// ── 扫描与匹配 ────────────────────────────────────────────
function scanSlides() {
  const allFiles = fs.readdirSync(slidesAllDir).filter((f) => /^slide_\d+.*\.png$/i.test(f));
  allFiles.sort();

  const fixedFiles = fs.existsSync(slidesFixedDir)
    ? fs.readdirSync(slidesFixedDir).filter((f) => /^slide_\d+.*\.png$/i.test(f))
    : [];

  // 建立编号 → 修复文件名的映射
  const fixedMap = new Map();
  for (const f of fixedFiles) {
    const m = f.match(/^slide_(\d+)/);
    if (m) fixedMap.set(m[1], f);
  }

  const slides = allFiles.map((f) => {
    const m = f.match(/^slide_(\d+)/);
    const num = m ? m[1] : "000";
    const fixedFile = fixedMap.get(num) || null;
    return { num, originalFile: f, fixedFile, replaced: !!fixedFile };
  });

  return slides;
}

// ── MIME ───────────────────────────────────────────────────
function mimeOf(ext) {
  const map = { ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".svg": "image/svg+xml" };
  return map[ext.toLowerCase()] || "application/octet-stream";
}

// ── 图片服务 ──────────────────────────────────────────────
function serveImage(res, baseDir, filename) {
  const filePath = path.join(baseDir, filename);
  if (!fs.existsSync(filePath)) {
    res.writeHead(404);
    res.end("Not found");
    return;
  }
  const ext = path.extname(filePath);
  res.writeHead(200, { "Content-Type": mimeOf(ext), "Cache-Control": "public, max-age=3600" });
  fs.createReadStream(filePath).pipe(res);
}

// ── HTML 页面 ─────────────────────────────────────────────
function buildHTML() {
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>幻灯片修复预览</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif; background: #0f1117; color: #e0e0e0; }

  /* 顶部统计栏 */
  .stats-bar {
    position: sticky; top: 0; z-index: 100;
    display: flex; align-items: center; justify-content: center; gap: 32px;
    padding: 8px 24px;
    background: #1a1d27; border-bottom: 1px solid #2a2d3a;
    font-size: 13px;
  }
  .stats-bar .stat { display: flex; align-items: center; gap: 6px; }
  .stats-bar .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }
  .dot-total { background: #888; }
  .dot-replaced { background: #f59e0b; }
  .dot-pass { background: #22c55e; }

  /* 卡片列表：纵向，限制最大宽度 */
  .cards {
    display: flex; flex-direction: column; gap: 8px;
    padding: 8px;
  }

  /* 卡片公共 */
  .card {
    background: #1a1d27; border-radius: 8px; overflow: hidden;
    border: 1px solid #2a2d3a;
  }
  .card-replaced { border-color: #f59e0b; }

  .card-header {
    display: flex; align-items: center; gap: 8px;
    padding: 5px 10px; font-size: 12px; color: #aaa;
    border-bottom: 1px solid #2a2d3a;
  }
  .badge {
    display: inline-block; padding: 1px 8px; border-radius: 3px;
    font-size: 11px; font-weight: 600; letter-spacing: .3px;
  }
  .badge-replaced { background: #3b2a10; color: #f59e0b; }
  .badge-pass { background: #16382a; color: #22c55e; }

  /* 对比区：两张图并排，充满全宽 */
  .compare {
    display: grid; grid-template-columns: 1fr 1fr; gap: 6px;
    padding: 6px;
  }

  /* 单图区：全宽 */
  .single { padding: 6px; }

  .img-wrapper { position: relative; }
  .img-wrapper img {
    width: 100%; height: auto; border-radius: 4px;
    cursor: pointer; display: block;
  }
  .img-label {
    position: absolute; top: 6px; left: 6px;
    background: rgba(0,0,0,.7); color: #ccc;
    font-size: 11px; padding: 1px 6px; border-radius: 3px;
    pointer-events: none;
  }

  /* Lightbox */
  .lightbox {
    display: none; position: fixed; inset: 0; z-index: 1000;
    background: rgba(0,0,0,.92); justify-content: center; align-items: center;
    cursor: zoom-out;
  }
  .lightbox.active { display: flex; }
  .lightbox img {
    max-width: 96vw; max-height: 96vh; object-fit: contain; border-radius: 4px;
  }
</style>
</head>
<body>

<div class="stats-bar">
  <div class="stat"><span class="dot dot-total"></span> 总计 <strong id="total">-</strong> 张</div>
  <div class="stat"><span class="dot dot-replaced"></span> 已替换 <strong id="replaced">-</strong> 张</div>
  <div class="stat"><span class="dot dot-pass"></span> 未变更 <strong id="pass">-</strong> 张</div>
</div>

<div class="cards" id="cards"></div>

<div class="lightbox" id="lightbox" onclick="this.classList.remove('active')">
  <img id="lightbox-img" src="" alt="">
</div>

<script>
  fetch("/api/slides").then(r => r.json()).then(slides => {
    document.getElementById("total").textContent = slides.length;
    const replacedCount = slides.filter(s => s.replaced).length;
    document.getElementById("replaced").textContent = replacedCount;
    document.getElementById("pass").textContent = slides.length - replacedCount;

    const container = document.getElementById("cards");
    for (const s of slides) {
      const card = document.createElement("div");
      card.className = "card" + (s.replaced ? " card-replaced" : "");

      const badgeClass = s.replaced ? "badge-replaced" : "badge-pass";
      const badgeText = s.replaced ? "已替换" : "PASS";

      let bodyHTML;
      if (s.replaced) {
        bodyHTML =
          '<div class="compare">' +
            '<div class="img-wrapper">' +
              '<span class="img-label">原图</span>' +
              '<img src="/images/all/' + s.originalFile + '" onclick="openLightbox(this.src)" alt="原图">' +
            '</div>' +
            '<div class="img-wrapper">' +
              '<span class="img-label">修复后</span>' +
              '<img src="/images/fixed/' + s.fixedFile + '" onclick="openLightbox(this.src)" alt="修复后">' +
            '</div>' +
          '</div>';
      } else {
        bodyHTML =
          '<div class="single">' +
            '<div class="img-wrapper">' +
              '<img src="/images/all/' + s.originalFile + '" onclick="openLightbox(this.src)" alt="幻灯片">' +
            '</div>' +
          '</div>';
      }

      card.innerHTML =
        '<div class="card-header">' +
          '<span class="badge ' + badgeClass + '">' + badgeText + '</span>' +
          '<span>#' + s.num + ' — ' + s.originalFile + '</span>' +
        '</div>' + bodyHTML;

      container.appendChild(card);
    }
  });

  function openLightbox(src) {
    document.getElementById("lightbox-img").src = src;
    document.getElementById("lightbox").classList.add("active");
  }
  document.addEventListener("keydown", e => {
    if (e.key === "Escape") document.getElementById("lightbox").classList.remove("active");
  });
</script>
</body>
</html>`;
}

// ── HTTP 服务 ─────────────────────────────────────────────
const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  const pathname = url.pathname;

  // 首页
  if (pathname === "/") {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(buildHTML());
    return;
  }

  // 幻灯片数据接口
  if (pathname === "/api/slides") {
    const slides = scanSlides();
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(slides));
    return;
  }

  // 原图
  const allMatch = pathname.match(/^\/images\/all\/(.+)$/);
  if (allMatch) {
    serveImage(res, slidesAllDir, decodeURIComponent(allMatch[1]));
    return;
  }

  // 修复图
  const fixedMatch = pathname.match(/^\/images\/fixed\/(.+)$/);
  if (fixedMatch) {
    serveImage(res, slidesFixedDir, decodeURIComponent(fixedMatch[1]));
    return;
  }

  res.writeHead(404);
  res.end("Not found");
});

// 找一个空闲端口
server.listen(0, () => {
  const port = server.address().port;
  const url = `http://localhost:${port}`;
  console.log(`幻灯片预览服务已启动: ${url}`);
  console.log(`项目目录: ${path.resolve(projectDir)}`);
  console.log("按 Ctrl+C 退出\n");

  // 自动打开浏览器（Windows）
  try {
    execSync(`start "" "${url}"`, { stdio: "ignore" });
  } catch {
    console.log(`请手动打开浏览器访问: ${url}`);
  }
});
