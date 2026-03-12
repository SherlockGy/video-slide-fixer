# Gemini 网页版修复（Playwright MCP）

表格密集文字场景下，Gemini 网页版的文字重绘质量**显著优于 API**（文字更清晰锐利，表格结构保持更完整）。

## 前置条件

1. 当前环境有可用的 Playwright MCP（`browser_navigate` 等工具可用）
2. 用户已在浏览器中登录 Google 账号（gemini.google.com 可正常访问）
3. 浏览器已关闭"下载前询问每个文件的保存位置"设置（Chrome: 设置 → 下载内容 → 关闭该选项），否则每次下载都会弹出系统对话框需要手动点击，Playwright 无法控制系统原生对话框

## 完整流程

### 步骤 1：导航到 Gemini
```
browser_navigate → https://gemini.google.com/app
```
等待页面加载完成后，用 `browser_snapshot` 确认看到输入框。

### 步骤 2：上传图片（剪贴板粘贴法）

> ⚠️ **关键经验**：Playwright 的 `browser_file_upload` 和 `setInputFiles` 在 Gemini 网页上会被 Chrome DevTools Protocol 拒绝（报错 `"Not allowed"`），即使尝试以下方法也均无效：
> - 修改 input 元素的 `display`/`visibility`/`aria-hidden` 属性后再调用 `setInputFiles`（超时）
> - 通过 `waitForEvent('filechooser')` 拦截文件选择器再 `setFiles`（同样 `"Not allowed"`）
> - 通过 `page.evaluate` + `input.click()` 触发文件选择器（`"Not allowed"`）
>
> **唯一可靠的方法是剪贴板粘贴**。

```bash
# 1. 用 PowerShell 将图片复制到 Windows 剪贴板
powershell.exe -Command "Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile('<图片绝对路径>'))"
```

```
# 2. 在浏览器中点击输入框
browser_click → textbox "为 Gemini 输入提示"

# 3. 粘贴图片
browser_press_key → Control+v
```

粘贴后用 `browser_snapshot` 或 `browser_take_screenshot` 确认输入框上方出现图片缩略图预览。**必须确认看到缩略图后再进入下一步**，否则会发送纯文字请求。

### 步骤 3：输入提示词并发送
```
browser_type → ref=<输入框ref> text="<提示词>" submit=true
```

表格文字重绘的推荐提示词：
```
请重绘所有汉字，保证清晰、锐利、无扭曲、移除右下角内容，提升画面清晰度
```

如需更精细的修复（特定纠错、补字等），可参照 skill.md 第六节模板组合提示词。

### 步骤 4：等待生成完成
```
browser_wait_for → textGone="正在加载" time=120
```
生成完成后页面会出现 AI 生成的图片和"下载完整尺寸的图片"按钮。

### 步骤 5：下载生成图片
```
# 1. 获取快照找到下载按钮
browser_snapshot

# 2. 点击"下载完整尺寸的图片"按钮
browser_click → "下载完整尺寸的图片"

# 3. 等待下载完成
browser_wait_for → textGone="正在下载完整尺寸的图片" time=60
```

### 步骤 6：定位下载文件并移至项目目录

下载的文件名格式为 `Gemini_Generated_Image_*.png`。按浏览器默认下载位置查找：
```bash
# 按修改时间排序，取最新的 Gemini 生成文件
# 先查桌面，再查 Downloads
ls -t ~/Desktop/Gemini_Generated_Image_*.png 2>/dev/null | head -1
ls -t ~/Downloads/Gemini_Generated_Image_*.png 2>/dev/null | head -1

# 移动到项目的 slides_fixed/ 目录并重命名
mv "<下载文件>" "<slides_fixed/slide_NNN_fixed.png>"
```

### 步骤 7：处理下一张（循环步骤 2-6）

对于多张图片，无需重新导航。在当前页面点击"发起新对话"按钮（`browser_snapshot` 找到后点击），然后从步骤 2 开始处理下一张。

## 注意事项

- **每张图片单独一轮对话**：避免上下文干扰
- **确认上传成功再发送**：粘贴后必须通过 snapshot/screenshot 确认看到图片缩略图
- **生成失败处理**：如果 Gemini 返回纯文字无图片，点击"重做"按钮重试；连续失败则回退到 API 方式（slides-fix.exe）
- **分辨率适配**：网页版生成的图片分辨率由 Gemini 自动决定，可能与源视频不完全一致。回写视频时 ffmpeg 的 `scale=W:H` 会自动适配，无需担心
- **不要通过 curl 下载图片 URL**：Gemini 生成图片的 `lh3.googleusercontent.com` URL 需要认证 cookie，curl 直接下载只会得到 HTML 重定向页面，必须通过浏览器的下载按钮获取
