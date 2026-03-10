---
name: video-slide-fixer
description: |
  从PPT风格视频中提取幻灯片画面，检查质量问题，通过 Gemini API 重新生成修复版图片，并可回写到视频中。
  适用场景：(1) AI生成的演示视频(NotebookLM等)质量修复 (2) 视频帧提取与场景检测 (3) 批量图片修复与视频回写。
  需要：Gemini API 密钥（.env 文件）。ffmpeg/ffprobe 已内置于技能 scripts/ 目录。
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Agent, ToolSearch
---

# Video Slide Fixer

从PPT风格视频中提取幻灯片、检查质量问题、通过 Gemini API 重新生成修复版图片。

---

## 一、前置检查

### 1.1 工具可用性

本技能的 `scripts/` 目录已内置所有必需的可执行文件：
```
scripts/ffmpeg.exe      ← 视频处理
scripts/ffprobe.exe     ← 视频元数据分析
scripts/slides-fix.exe  ← 幻灯片图片修复（Go 语言开发）
```

执行前先定义路径变量（后续所有命令均使用这些变量）：
```bash
SKILL_SCRIPTS="<技能目录>/scripts"
FFMPEG="$SKILL_SCRIPTS/ffmpeg.exe"
FFPROBE="$SKILL_SCRIPTS/ffprobe.exe"
SLIDES_FIX="$SKILL_SCRIPTS/slides-fix.exe"
```

> 若系统 PATH 中已有 ffmpeg/ffprobe，也可直接使用，但推荐使用技能内置版本以确保版本一致。

使用前需在项目工作目录下创建 `.env` 文件并填入 Gemini API 密钥（可从 `tool-source/slides-fix/.env.example` 复制）：
```
GEMINI_API_KEY=你的密钥
```

> 源码位于 `tool-source/slides-fix/`，可随时修改、编译后替换 `scripts/slides-fix.exe`。详见 `tool-source/slides-fix/README.md`。

### 1.2 视频元数据分析
```bash
$FFPROBE -v quiet -print_format json -show_format -show_streams "<视频路径>"
```
记录关键信息：时长、分辨率、帧率、编码格式、音频参数。

---

## 二、幻灯片提取

### 2.1 场景检测（获取时间戳）

```bash
$FFMPEG -i "<视频路径>" \
  -vf "select='gt(scene,<阈值>)',showinfo" \
  -vsync vfr -f null - 2>&1 | grep "pts_time"
```

**阈值选择指南**：
| 视频类型 | 推荐阈值 | 说明 |
|----------|----------|------|
| 纯PPT切换（无动画） | 0.3 - 0.4 | 场景变化大，高阈值避免误检 |
| 带动画的PPT（NotebookLM等） | 0.10 - 0.20 | 动画使场景变化平缓，需低阈值 |
| 混合内容视频 | 0.2 - 0.3 | 中间值平衡 |

**调参策略**：从 0.3 开始，若结果数量明显少于预期幻灯片数则降低；若结果过多（含大量动画中间帧）则升高。可多个阈值并行测试对比。

**提取时间戳的 grep 兼容性**：Windows Git Bash 下 `grep -P`（Perl正则）不可用，改用 sed：
```bash
$FFMPEG -i "<视频>" -vf "select='gt(scene,0.15)',showinfo" -vsync vfr -f null - 2>&1 \
  | grep "pts_time" | sed 's/.*pts_time:\([0-9.]*\).*/\1/'
```

### 2.2 按时间戳批量提取帧

对每个场景切换时间点加 **+3秒偏移**（等动画渲染完毕），提取该时刻的画面：

```bash
TIMES=(
  "<时间戳1>:<文件名1>"
  "<时间戳2>:<文件名2>"
  # ...
)

for entry in "${TIMES[@]}"; do
  IFS=':' read -r ts name <<< "$entry"
  $FFMPEG -y -ss "$ts" \
    -i "<视频路径>" \
    -frames:v 1 -q:v 2 \
    "<输出目录>/${name}.jpg"
done
```

**命名规范**：`slide_NNN_MMmSSs.jpg`（如 `slide_001_00m03s.jpg`）

**关键参数**：
- `-ss` 放在 `-i` 之前：快速输入定位（seek to keyframe）
- `-q:v 2`：高质量 JPEG（1最好，31最差）
- `-frames:v 1`：只取一帧

### 2.3 补充提取（长间隔场景）

对场景间隔 >40秒的片段，在中间额外提取1帧，防止遗漏渐进式动画内容。命名加 `b` 后缀如 `slide_007b_03m00s.jpg`。

> **实测经验**：补充帧经常与主帧内容完全一致（长间隔并不意味着有新内容，可能只是动画过渡较慢）。这些重复帧可在检查阶段标记忽略，无需删除。

### 2.4 Windows 路径注意事项

- ffmpeg 支持 `C:/path/to/file` 格式（正斜杠）
- 在 Git Bash 中，`/tmp` 映射到 `$TEMP`（AppData/Local/Temp）
- Read 工具需要 Windows 格式路径：`C:\Users\...\file.jpg`
- 用 `cygpath -w <路径>` 互转

---

## 三、视觉质量检查

### 3.1 逐帧读取与检查

用 Read 工具逐张读取 slides_all/ 中的图片（Read 支持 JPG/PNG），按以下清单检查：

| 检查项 | PASS 条件 | FAIL 特征 |
|--------|-----------|-----------|
| **中文字符清晰度** | 笔画完整，可正确辨认 | 字符融化、笔画缺失、扭曲变形、像素化 |
| **边缘异形** | 边缘/角落干净无多余元素 | 出现不明符号、半截文字、颜色溢出 |
| **内容完整性** | 文字/图表完整 | 文字截断、图表缺失元素 |
| **整体一致性** | 风格与其他幻灯片一致 | 配色异常、布局错乱 |

**AI生成视频的典型文字问题模式**（实测总结）：
| 模式 | 示例 | 说明 |
|------|------|------|
| **形近字替换** | "最优解"→"毬拔解"，"顶层设计"→"顶厝设计" | AI生成的字看起来像但实际是错字 |
| **完全乱码** | "汝小角"、"谓暑设计"、"计标芳会" | 边缘/角落区域常出现无意义的字符组合 |
| **多处聚集** | 同一张幻灯片上多处文字同时扭曲 | 一旦一帧有问题，通常不止一处 |
| **边缘高发** | 问题集中在画面边缘而非中心 | 中心区域的大号标题通常正确 |

**实测参考**：NotebookLM 生成的 ~11分钟视频，29帧中5帧有问题（约17%故障率）。

### 3.2 建立检查记录表

对每张幻灯片输出一行记录：
```
| 文件名 | 主要内容(简述) | 文字清晰度 | 边缘质量 | 完整性 | 判定 | 问题描述 |
```

**高效检查方法**：每次用 Read 工具并行读取 6-8 张图片（Read 支持多图并行），一批审查后记录结果，再读下一批。29帧约4批即可完成。

### 3.3 标记问题帧

将 FAIL 的帧复制到 slides_issues/：
```bash
cp "<slides_all/问题文件.jpg>" "<slides_issues/>"
```

---

## 四、Gemini API 重新生成（slides-fix 工具）

### 4.1 准备工作

1. **确认 .env 文件**已在项目工作目录中配置好 `GEMINI_API_KEY`
2. **确认 slides_issues/ 目录**中已放置需要修复的问题帧
3. **创建 slides_fixed/ 目录**用于存放输出

### 4.2 编写 tasks.json

在项目工作目录下创建 `tasks.json`，定义批量任务：

```json
[
  {
    "image": "slides_issues/slide_010_03m48s.jpg",
    "output": "slides_fixed/slide_010_fixed.png",
    "prompt": "请根据附图重新生成一张PPT幻灯片图片。要求：..."
  },
  {
    "image": "slides_issues/slide_018_07m18s.jpg",
    "output": "slides_fixed/slide_018_fixed.png",
    "prompt": "请根据附图重新生成这张PPT幻灯片。..."
  }
]
```

**提示词编写要点**：
- 明确指出原图中的错误文字及其正确内容
- 描述幻灯片的视觉风格（颜色、手绘/插画等）
- 指定输出尺寸（如 1280x720）
- 尽量详细描述幻灯片中应有的正确内容（标题、要点、图表元素）
- 参考第六节的提示词模板

### 4.3 运行批量修复

```bash
# 使用技能自带的预编译工具
$SLIDES_FIX -batch tasks.json -delay 10

# 或指定不同模型和并发数
$SLIDES_FIX -batch tasks.json -model gemini-3.1-flash-image-preview -delay 15 -concurrency 3
```

### 4.4 单张修复（调试/测试用）

```bash
$SLIDES_FIX \
  -image slides_issues/slide_010_03m48s.jpg \
  -prompt "请根据附图重新生成..." \
  -output slides_fixed/slide_010_fixed.png
```

### 4.5 处理失败任务

批量运行结束后会报告成功/失败数量。对失败的任务：

1. **网络错误（unexpected EOF）**：通常是临时性问题，创建仅含失败任务的 `tasks_retry.json`，重新运行
2. **429 / RESOURCE_EXHAUSTED**：已触发限流，工具会自动等待 60s 后重试；若仍失败，增大 `-delay` 参数或等待一段时间后重试
3. **无图片返回（text only）**：模型可能不支持该提示词的图片生成，调整提示词后重试

### 4.6 验证修复质量

用 Read 工具逐张查看 slides_fixed/ 中的图片，确认：
- 问题文字已修正
- 整体风格与原始幻灯片一致
- 未引入新的错误

若结果不满意，调整提示词后重新运行单张修复。

---

## 五、回写视频（用修复图替换原帧）

> **此步骤必须经用户明确确认后才能执行。** 操作前应向用户展示将要替换的帧范围列表，确认无误后再运行 ffmpeg 命令。输出为独立文件，不覆盖原视频。

### 5.1 计算每张幻灯片的帧范围

场景检测已给出每个幻灯片的切换时间戳。结合视频帧率即可算出帧序号：

```
起始帧 = floor(切换时间戳 × fps)
结束帧 = floor(下一个切换时间戳 × fps) - 1    # 最后一张用视频总帧数 - 1
```

获取帧率和总帧数：
```bash
# 帧率（注意：返回分数形式如 24/1 或 30000/1001，需计算为小数）
$FFPROBE -v quiet -select_streams v:0 -show_entries stream=r_frame_rate -of csv=p=0 "<视频>"
# 例如 24/1 → fps=24，30000/1001 → fps≈29.97

# 总帧数（-count_frames 需完整解码，大视频可能耗时数分钟）
$FFPROBE -v quiet -count_frames -select_streams v:0 -show_entries stream=nb_read_frames -of csv=p=0 "<视频>"
# 快速估算替代：总帧数 ≈ 时长(秒) × fps
$FFPROBE -v quiet -show_entries format=duration -of csv=p=0 "<视频>"
```

示例（24fps 视频，场景切换时间戳 25.958s、52.458s、85.375s）：
```
slide_007: 帧 623 ~ 1258    (25.958s ~ 52.458s)
slide_010: 帧 1259 ~ 2048   (52.458s ~ 85.375s)
```

### 5.2 构建 ffmpeg overlay 命令

单张替换：
```bash
$FFMPEG -i "<原视频>" -i "<修复图>.png" \
  -filter_complex "[1]scale=<W>:<H>[img];[0][img]overlay=enable='between(n,<起始帧>,<结束帧>)'[vout]" \
  -map "[vout]" -map 0:a -c:v libx264 -crf 18 -preset medium -c:a copy "<输出视频>"
```

多张同时替换（链式 overlay）：
```bash
$FFMPEG -i "<原视频>" \
  -i slides_fixed/slide_007_fixed.png \
  -i slides_fixed/slide_010_fixed.png \
  -i slides_fixed/slide_018_fixed.png \
  -filter_complex \
  "[1]scale=<W>:<H>[img1]; \
   [2]scale=<W>:<H>[img2]; \
   [3]scale=<W>:<H>[img3]; \
   [0][img1]overlay=enable='between(n,<start1>,<end1>)'[v1]; \
   [v1][img2]overlay=enable='between(n,<start2>,<end2>)'[v2]; \
   [v2][img3]overlay=enable='between(n,<start3>,<end3>)'[vout]" \
  -map "[vout]" -map 0:a -c:v libx264 -crf 18 -preset medium -c:a copy "<输出视频>"
```

> 编码参数说明：`-c:v libx264 -crf 18 -preset medium` 以接近无损的质量编码视频。`crf` 值越小质量越高（0=无损，18=视觉无损，23=默认）。如需完全匹配原视频参数，先用 `$FFPROBE` 查看原始比特率后用 `-b:v` 指定。

### 5.3 注意事项

- **尺寸必须匹配**：替换图片需与视频分辨率一致（如 1280×720），否则 overlay 只覆盖部分画面。用 `scale=W:H` 确保一致。
- **输出独立文件**：命名如 `<原名>_fixed.mp4`，绝不覆盖原视频。
- **音频直接拷贝**：`-c:a copy` 保留原始音轨不重新编码。
- **overlay 链过长时**：若替换帧较多（>10张），ffmpeg 单条命令可能很慢。可分批执行：先替换前几张生成中间文件，再在中间文件上替换后续。
- **帧号从 0 开始**：`between(n,100,200)` 表示第 100~200 帧（闭区间，共 101 帧）。
- **执行前必须确认**：向用户展示完整的帧范围替换表，获得确认后才执行。

### 5.4 执行流程

```
1. 列出所有需要替换的帧范围（文件名、起始帧、结束帧、时间范围）
2. 展示给用户确认
3. 用户确认后，构建并执行 ffmpeg 命令
4. 验证输出视频（抽查替换区间的画面）
```

---

## 六、提示词模板

### 提示词编写原则

1. **汉字重绘优先**：凡是画面中包含大量中文的帧，提示词必须明确列出所有正确的中文内容，并要求"重新绘制所有中文字符，确保笔画完整、无扭曲、无破损"。不要只说"保持不变"——AI倾向于原样复制错误。
2. **边缘异型字默认删除**：画面边缘/角落的乱码异型字，除非能轻松辨认出原意并纠正，否则一律要求删除。边缘装饰性文字的缺失不影响幻灯片的核心表达。
3. **正文内容全量提供**：不要依赖"参考原图"来保留文字——必须在提示词中逐字写出画面上应该出现的所有文字内容，作为生成的唯一依据。

### 模板A：中文字符修复（最常用）

适用：画面中有大量中文，部分出现扭曲/替换/破损。

```
请根据附图重新生成一张PPT幻灯片图片。

要求：
- 风格：{STYLE_DESCRIPTION}
- 尺寸：{WIDTH}x{HEIGHT}像素，{ASPECT_RATIO}比例
- 布局：保持与原图相同的布局结构和装饰元素
- 重新绘制画面中的所有中文字符，确保笔画完整、无扭曲、无破损
- 画面边缘如有无法辨认的异形文字，直接删除，保持边缘干净
- 背景：{BACKGROUND_DESCRIPTION}

画面中应出现的完整文字内容（以此为准，忽略原图中的乱码）：
{FULL_TEXT_CONTENT}

请使用标准中文字体渲染所有文字，字号清晰可读。
```

### 模板B：边缘伪影清理

适用：画面主体正常，但边缘/角落有异型字符或乱码噪点。

```
请根据附图重新生成这张PPT幻灯片。

原图问题：画面边缘/角落存在AI生成伪影（异形文字、乱码符号）。

要求：
- 保持中心区域的主要内容和布局完全不变
- 删除所有边缘区域的异常文字和视觉噪点，保持边缘干净
- 重新绘制画面中的所有中文字符，确保笔画完整清晰
- 风格：{STYLE_DESCRIPTION}
- 尺寸：{WIDTH}x{HEIGHT}像素

画面核心内容描述：{SLIDE_CONTENT_DESCRIPTION}
```

### 模板C：特定文字纠错

适用：仅个别文字被替换为形近字，其余正常。

```
请根据附图重新生成这张PPT幻灯片。

原图中以下文字存在错误："{PROBLEMATIC_TEXT}"
正确的文字应该是："{CORRECT_TEXT}"

要求：
- 修正上述错误文字，其他内容保持不变
- 重新绘制画面中的所有中文字符，确保无扭曲破损
- 画面边缘如有无法辨认的异形文字，直接删除
- {STYLE_DESCRIPTION}，{WIDTH}x{HEIGHT}像素
```

### 模板D：完整重建（原图损坏严重时）

适用：画面多处文字严重扭曲，修补不如全部重建。

```
请生成一张PPT风格的幻灯片图片。

主题：{TOPIC}
风格：{STYLE_DESCRIPTION}
尺寸：{WIDTH}x{HEIGHT}像素，{ASPECT_RATIO}比例

画面中应出现的完整文字内容：
{FULL_TEXT_CONTENT}

布局：{LAYOUT_DESCRIPTION}

请确保所有中文字符使用标准字体渲染，笔画完整，无扭曲、无破损、无乱码。
画面边缘保持干净，不要出现多余的装饰性文字。
```

### 变量填充参考（NotebookLM 粉色主题）

```
STYLE_DESCRIPTION = "粉色主题，手绘/插画风格，与NotebookLM生成的视频风格一致"
WIDTH = 1280
HEIGHT = 720
ASPECT_RATIO = "16:9"
BACKGROUND_DESCRIPTION = "浅粉色网格底纹，带有手绘纹理"
LAYOUT_DESCRIPTION = "标题在顶部，要点在中下方，右侧/底部有装饰性插画元素（齿轮、拼图、放大镜等），右下角保留 NotebookLM 标识"
```

---

## 七、完整目录结构模板

```
<项目目录>/
├── <源视频>.mp4
├── slides_all/          ← 全量幻灯片截图 (slide_NNN_MMmSSs.jpg)
├── slides_issues/       ← 有问题的帧（从 slides_all 复制）
├── slides_fixed/        ← Gemini 重新生成的修复版
├── slides-fix/          ← slides-fix 工具工作目录（可选，用于就地运行）
│   ├── .env             ← Gemini API 密钥
│   ├── tasks.json       ← 批量任务配置
│   └── slides-fix.exe   ← 可直接复制自技能 scripts/ 目录
└── prompt_templates.md  ← 该项目专用的提示词（从模板填充变量后的版本）
```

---

## 八、常见问题

### Q: 场景检测结果数量不对？
多个阈值并行测试：`0.1`, `0.15`, `0.2`, `0.3`, `0.4`。选择结果数量最接近预期幻灯片数的阈值。

### Q: 提取的帧是动画中间态（内容未渲染完）？
增加偏移量（从+3s增到+5s或更多），或在该场景结束前2秒提取（取场景末尾的画面）。

### Q: Read 工具读不到提取的图片？
Windows 上确保使用绝对路径且格式为 `C:\Users\...`（反斜杠）。用 `cygpath -w` 转换。

### Q: Gemini 生成的图片风格不一致？
- 上传原图作为参考（slides-fix 会自动附加原图）
- 在提示词中加入更多风格细节（色号、线条风格、装饰元素）
- 多次生成并选择最佳结果

### Q: Gemini 触发限流？
按问题优先级排序，先处理最严重的。增大 `-delay` 参数，分多次运行。

### Q: slides-fix 报 "unexpected EOF"？
临时性网络问题，工具会自动重试 2 次。若仍失败，等待几分钟后重新运行失败的任务。

### Q: 需要重新编译 slides-fix？
源码在技能的 `tool-source/slides-fix/` 目录：
```bash
cd tool-source/slides-fix/
go build -o ../../scripts/slides-fix.exe .
```

---

## 九、实战案例参考

**项目**：集成供应链计划管理的体系框架（NotebookLM 生成）
- 视频：670秒，1280x720，H.264 24fps，AAC mono
- 场景检测阈值：0.15 → 25个场景切换
- 提取帧：26主帧 + 3补充帧 = 29帧（其中3帧与主帧重复）
- 问题帧：5张（17%故障率）
- 问题分类：
  - 形近字替换 2张（slide_010 "毬拔解"→"最优解"，slide_025 "顶厝设计"→"顶层设计"）
  - 边缘乱码 1张（slide_007 "汝小角"）
  - 多处严重扭曲 2张（slide_018、slide_022 各有3-4处乱码）
- 处理优先级：严重度高的先处理（018 > 022 > 010 > 025 > 007）
- **修复结果**：使用 slides-fix 批量处理，模型 gemini-3.1-flash-image-preview，5张中3张一次成功，2张因网络 EOF 失败后重试成功

---

## 十、执行检查清单

```
□ 1. 设置 $FFMPEG/$FFPROBE/$SLIDES_FIX 路径变量（指向技能 scripts/ 目录）
□ 2. 配置 .env（填入 Gemini API 密钥）
□ 3. 获取视频元数据（$FFPROBE）
□ 4. 创建三个输出目录：slides_all/ slides_issues/ slides_fixed/
□ 5. 场景检测获取时间戳（$FFMPEG + scene filter）
□ 6. 计算提取时间点（场景切换 +3s 偏移）
□ 7. 批量提取帧（主帧 + 长间隔补充帧）
□ 8. 逐帧视觉检查（Read 工具，每批 6-8 张并行）
□ 9. 标记问题帧并复制到 slides_issues/
□ 10. 输出问题汇总表（文件名、问题类型、具体描述、正确文字）
□ 11. 生成项目专用提示词文档（prompt_templates.md）
□ 12. 编写 tasks.json（定义批量修复任务）
□ 13. 运行 $SLIDES_FIX -batch tasks.json
□ 14. 处理失败任务（重试或调整提示词）
□ 15. 验证修复质量（Read 工具逐张检查 slides_fixed/）
□ 16. 计算帧范围（场景时间戳 × fps → 起始帧/结束帧）
□ 17. 展示帧替换表，等待用户确认
□ 18. 执行 $FFMPEG overlay 回写视频（输出独立文件 *_fixed.mp4）
□ 19. 抽查输出视频中替换区间的画面
```
