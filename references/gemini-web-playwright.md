# Gemini 网页版修复（Playwright MCP）

表格密集文字场景下，Gemini 网页版的文字重绘质量**显著优于 API**（文字更清晰锐利，表格结构保持更完整）。

## 前置条件

1. 当前环境有可用的 Playwright MCP（`browser_navigate` 等工具可用）
2. 用户已在浏览器中登录 Google 账号（gemini.google.com 可正常访问）
3. 浏览器已关闭"下载前询问每个文件的保存位置"设置（Chrome: 设置 → 下载内容 → 关闭该选项），否则每次下载都会弹出系统对话框需要手动点击，Playwright 无法控制系统原生对话框

## 完整流程（每张图片 4 次工具调用）

> **设计原则**：用 `browser_run_code` 把多个浏览器交互合并为一次调用，减少工具调用往返。每张图片只需 4 次调用：Bash（剪贴板）→ browser_run_code（上传+输入+发送）→ browser_run_code（等待+下载）→ Bash（移动文件）。

### 步骤 1：导航到 Gemini（仅首张图片）
```
browser_navigate → https://gemini.google.com/app
```

### 步骤 2：复制图片到剪贴板

> ⚠️ **为什么必须走剪贴板**：Playwright 的 `browser_file_upload`、`setInputFiles`、`waitForEvent('filechooser')` 在 Gemini 网页上全部被 Chrome DevTools Protocol 拒绝（报错 `"Not allowed"`）。`browser_run_code` 也无法 `require('fs')` 读取本地文件。唯一可靠的路径是 **PowerShell 写剪贴板 → 浏览器 Ctrl+V 粘贴**。

```bash
powershell.exe -Command "Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile('<图片绝对路径>'))"
```

### 步骤 3：上传 + 输入提示词 + 发送（合并为一次调用）

用 `browser_run_code` 一次完成粘贴图片、确认缩略图、输入提示词、点击发送：

```javascript
// browser_run_code
async (page) => {
  // 点击输入框
  const inputBox = page.getByRole('textbox', { name: '为 Gemini 输入提示' });
  await inputBox.click();
  await page.waitForTimeout(300);

  // 粘贴剪贴板中的图片
  await page.keyboard.press('Control+v');

  // 等待缩略图出现（必须确认，否则会发送纯文字请求）
  await page.getByRole('button', { name: '图片预览' }).waitFor({ state: 'visible', timeout: 5000 });

  // 输入提示词（用 fill，不要用 type —— fill 更快且不会触发逐字事件）
  await inputBox.fill('<提示词>');

  // 点击发送按钮（Enter 键在 Gemini 不可靠，必须点击按钮）
  await page.getByRole('button', { name: '发送' }).click();

  return 'done';
}
```

**关键细节**：
- **必须 click "发送" 按钮**：`fill()` + `Enter` 和 `browser_type` 的 `submit=true` 在 Gemini 上都不能可靠触发提交
- **等待缩略图**：`getByRole('button', { name: '图片预览' })` 是粘贴成功后出现的缩略图元素，比 `waitForTimeout(3000)` 更精确

表格文字重绘的推荐提示词：
```
请重绘所有汉字，保证清晰、锐利、无扭曲、移除右下角内容，提升画面清晰度
```

如需更精细的修复（特定纠错、补字等），可参照 skill.md 第六节模板组合提示词。

### 步骤 4：等待生成 + 下载（合并为一次调用）

生成通常需要 15~45 秒。用 `browser_run_code` 等待下载按钮出现后直接点击：

```javascript
// browser_run_code
async (page) => {
  // 等待下载按钮出现（必须用 getByRole，getByText 匹配不到按钮元素）
  await page.getByRole('button', { name: '下载完整尺寸的图片' })
    .waitFor({ state: 'visible', timeout: 90000 });

  // 直接点击下载
  await page.getByRole('button', { name: '下载完整尺寸的图片' }).click();

  return 'downloaded';
}
```

**关键细节**：
- **必须用 `getByRole('button', ...)`**：下载按钮是 `<button>` 元素，`getByText()` 和 `browser_wait_for` 的 `text=` 参数都无法匹配到它
- 若 90 秒超时，说明生成失败——用 `browser_snapshot` 检查页面，可能返回了纯文字

### 步骤 5：移动下载文件

下载文件默认保存到**用户的下载目录**（`~/Downloads/`），除非用户明确指定了其他目录。

**必须使用 diff 方式定位新文件**，避免误取旧文件：

```bash
# ===== 在步骤 2（复制剪贴板）之前，先记录已有文件 =====
ls ~/Downloads/Gemini_Generated_Image_*.png 2>/dev/null | sort > /tmp/gemini_before.txt

# ===== 在点击下载按钮之后，等待并找到新增文件 =====
sleep 5
ls ~/Downloads/Gemini_Generated_Image_*.png 2>/dev/null | sort > /tmp/gemini_after.txt
NEW=$(comm -13 /tmp/gemini_before.txt /tmp/gemini_after.txt)
echo "New file: $NEW"

# 确认文件存在后再移动
if [ -n "$NEW" ]; then
  cp "$NEW" "<slides_fixed/slide_NNN_fixed.png>"
else
  echo "ERROR: No new file found!"
fi
```

> **重要**：
> - 不要用 `ls -t | head -1`，该方式会在存在旧文件时取错
> - 移动前必须用 Read 工具验证图片内容是否正确，确认后再移动
> - 记录已有文件的步骤应在**每轮处理开始时**执行（步骤 2 之前）

### 处理下一张（循环步骤 2-5）

无需重新导航。用 `browser_run_code` 点击"发起新对话"按钮开始下一轮：

```javascript
// browser_run_code
async (page) => {
  await page.getByRole('button', { name: '发起新对话' }).click();
  await page.waitForTimeout(1000);
  return 'ready';
}
```

然后从步骤 2（PowerShell 剪贴板）开始处理下一张。此步骤可与步骤 2 的 Bash 调用合并——先 `browser_run_code` 新建对话，再 Bash 复制下一张图片到剪贴板。

## 注意事项

- **每张图片单独一轮对话**：避免上下文干扰
- **生成失败处理**：如果 Gemini 返回纯文字无图片，在 `browser_run_code` 中点击"重做"按钮重试；连续失败则回退到 API 方式（slides-fix.exe）
- **分辨率适配**：网页版生成的图片分辨率由 Gemini 自动决定，可能与源视频不完全一致。回写视频时 ffmpeg 的 `scale=W:H` 会自动适配，无需担心
- **不要通过 curl 下载图片 URL**：Gemini 生成图片的 `lh3.googleusercontent.com` URL 需要认证 cookie，curl 直接下载只会得到 HTML 重定向页面，必须通过浏览器的下载按钮获取
