# slides-fix

基于 Google Gemini API 的幻灯片图片修复工具。用于修复 AI 生成视频（如 NotebookLM）中存在文字扭曲、乱码、伪影等问题的 PPT 帧截图。

## 功能

- **单张修复**：指定一张图片和提示词，调用 Gemini 生成修复版
- **批量修复**：通过 `tasks.json` 配置文件批量处理多张问题图片
- **自动重试**：网络错误时指数退避重试（5s → 10s → 20s），限流时等待 60s
- **可控并发**：支持多路并发处理（`-concurrency`），加速批量任务
- **灵活配置**：支持自定义模型、请求间隔、并发数、输出路径

## 依赖

- Go 1.24+（编译，go.mod 指定 1.26.1）
- `google.golang.org/genai` v1.49.0 — Google Gemini Go SDK
- `github.com/joho/godotenv` v1.5.1 — .env 文件加载

## 编译

```bash
cd tool-source/slides-fix/
go build -o ../../scripts/slides-fix.exe .
```

## 配置

在工作目录下创建 `.env` 文件：

```
GEMINI_API_KEY=你的Gemini_API密钥
```

API 密钥获取：https://aistudio.google.com/apikey

## 使用方式

### 单张模式

```bash
slides-fix -image <图片路径> -prompt "修复提示词" [-output <输出路径>]
```

- `-image`：输入图片路径（必填）
- `-prompt`：发送给 Gemini 的提示词（必填）
- `-output`：输出路径（可选，默认为 `<原文件名>_fixed.png`）
- `-model`：Gemini 模型名（默认 `gemini-3.1-flash-image-preview`）
- `-env`：.env 文件路径（默认 `.env`）

### 批量模式

```bash
slides-fix -batch tasks.json [-delay 10] [-concurrency 3]
```

- `-batch`：任务配置文件路径（必填）
- `-delay`：每个 worker 的请求间隔秒数（默认 10）
- `-concurrency`：并发 worker 数（默认 1，即串行）

### tasks.json 格式

```json
[
  {
    "image": "slides_issues/slide_010_03m48s.jpg",
    "output": "slides_fixed/slide_010_fixed.png",
    "prompt": "请根据附图重新生成..."
  }
]
```

## 架构

```
main.go     — CLI 入口，参数解析，单张/批量模式调度，worker pool，文件保存
client.go   — GeminiClient 封装（连接复用），API 调用，重试逻辑
go.mod      — 模块定义与依赖
tasks.json  — 批量任务配置示例
```

### 关键技术细节

- **ResponseModalities**：必须设置为 `["TEXT", "IMAGE"]` 才能让 Gemini 返回图片
- **Part 构造**：使用 `genai.NewPartFromText()` + `genai.NewPartFromBytes()` 组合文本提示与参考图片
- **响应解析**：遍历 `result.Candidates[0].Content.Parts`，找到 `InlineData` 不为空的 Part 即为生成的图片
- **重试策略**：普通错误指数退避（最多 2 次），429/RESOURCE_EXHAUSTED 额外等待 60s
