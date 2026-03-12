# Video Slide Fixer

[English](README.md)

修复 AI 生成视频中的幻灯片画面质量问题。

## 它能做什么

AI 工具（如 NotebookLM）生成的演示视频，经常出现中文字符扭曲、边缘乱码等问题。这个技能可以帮你：

1. **提取** — 从视频中自动识别并截取每一页幻灯片
2. **检查** — 逐帧审查，找出文字模糊、乱码、伪影等问题
3. **修复** — 调用 Gemini API 重新生成有问题的画面
4. **回写** — 将修复后的图片替换回视频中的对应帧段

## 需要什么

- Gemini API 密钥（见下方配置说明）
- 其余工具（ffmpeg、ffprobe、fend、slides-fix）已内置于 `scripts/` 目录

## 配置 API 密钥

在你的**项目工作目录**（如视频文件所在目录）下创建 `.env` 文件：

```
GEMINI_API_KEY=你的API密钥
```

密钥获取地址：https://aistudio.google.com/apikey

> 参考模板：`tool-source/slides-fix/.env.example`
>
> 注意：`.env` 文件包含敏感密钥，请勿提交到版本控制。技能已在 `.gitignore` 中排除了该文件。

## 目录说明

```
video-slide-fixer/
├── SKILL.md           ← 完整操作指南（Claude Code 读取）
├── README.md          ← 你正在看的这个文件
├── scripts/
│   ├── ffmpeg.exe     ← 视频处理
│   ├── ffprobe.exe    ← 视频元数据分析
│   ├── fend.exe       ← 精确数学计算
│   ├── slides-fix.exe ← 图片修复工具（预编译）
│   └── model.conf     ← 模型配置文件
├── references/        ← 详细参考文档（按需加载）
└── tool-source/
    └── slides-fix/    ← 修复工具的 Go 源码（可自行修改编译）
```

## 模型切换

编辑 `scripts/model.conf` 切换 Gemini 模型。规则：`#` 开头为注释，第一个非注释非空行生效。

默认配置（使用 Pro）：
```
gemini-3-pro-image-preview
# gemini-3.1-flash-image-preview
```

切换到 Flash：
```
# gemini-3-pro-image-preview
gemini-3.1-flash-image-preview
```

优先级：命令行 `-model` 参数 > `model.conf` > 内置默认值

## 可选：Gemini 网页版修复表格密集文字

对于**含表格和大量中文字**的幻灯片，Gemini 网页版（`gemini.google.com`）的文字重绘质量**显著优于 API**。如果你配置了 [Playwright MCP Bridge 浏览器插件](https://chromewebstore.google.com/detail/playwright-mcp-bridge/mmlmfjhmonkocbjadbfplnigmagldckm) 服务器，Claude Code 在检测到此类问题帧时会自动询问是否使用网页版修复。

使用条件：
- 环境中有可用的 Playwright MCP 工具（`browser_navigate` 等）
- 浏览器已登录 Google 账号
- 浏览器已关闭"下载前询问保存位置"设置

## 局限性

- Gemini 生成的修复图不一定完美，可能需要多次调整提示词
- Gemini API 有调用频率限制，大批量处理需要耐心
- 修复效果取决于提示词的质量——采用 diff 风格（只说要改什么），越简洁精准结果越好
