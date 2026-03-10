# 回写视频（用修复图替换原帧）

> **此步骤必须经用户明确确认后才能执行。** 操作前应向用户展示将要替换的帧范围列表，确认无误后再运行 ffmpeg 命令。输出为独立文件，不覆盖原视频。

## 计算每张幻灯片的帧范围

场景检测已给出每个幻灯片的切换时间戳。结合视频帧率即可算出帧序号：

```
起始帧 = floor(切换时间戳 × fps)
结束帧 = floor(下一个切换时间戳 × fps) - 1    # 最后一张用视频总帧数 - 1
```

获取帧率和总帧数：
```bash
# 帧率
ffprobe -v quiet -select_streams v:0 -show_entries stream=r_frame_rate -of csv=p=0 "<视频>"

# 时长（用于估算总帧数）
ffprobe -v quiet -show_entries format=duration -of csv=p=0 "<视频>"
```

**帧号计算必须使用 fend**（精确数学计算工具，已内置于 scripts/ 目录），禁止心算或用 python/bc 等替代：

```bash
# 总帧数
fend "floor(670.94 * 24)"

# 起始帧 = floor(场景开始时间 × fps)
fend "floor(141.958333 * 24)"

# 结束帧 = floor(下一场景开始时间 × fps) - 1
fend "floor(185.166667 * 24) - 1"
```

可以在一条命令中批量计算多个帧号（每个 fend 调用独立）：
```bash
echo "=== slide_007 ===" && fend "floor(141.958 * 24)" && fend "floor(185.167 * 24) - 1" && \
echo "=== slide_010 ===" && fend "floor(225.042 * 24)" && fend "floor(251.167 * 24) - 1"
```

示例输出（24fps 视频）：
```
=== slide_007 ===
3406      ← 起始帧
4443      ← 结束帧
=== slide_010 ===
5401
6027
```

## 构建 ffmpeg overlay 命令

单张替换：
```bash
ffmpeg -i "<原视频>" -i "<修复图>.png" \
  -filter_complex "[1]scale=<W>:<H>[img];[0][img]overlay=enable='between(n,<起始帧>,<结束帧>)'[vout]" \
  -map "[vout]" -map 0:a -c:v libx264 -crf 18 -preset medium -c:a copy "<输出视频>"
```

多张同时替换（链式 overlay）：
```bash
ffmpeg -i "<原视频>" \
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

> 编码参数说明：`-c:v libx264 -crf 18 -preset medium` 以接近无损的质量编码视频。`crf` 值越小质量越高（0=无损，18=视觉无损，23=默认）。如需完全匹配原视频参数，先用 `ffprobe` 查看原始比特率后用 `-b:v` 指定。

## 注意事项

- **尺寸必须匹配**：替换图片需与视频分辨率一致（如 1280×720），否则 overlay 只覆盖部分画面。用 `scale=W:H` 确保一致。
- **输出独立文件**：命名如 `<原名>_fixed.mp4`，绝不覆盖原视频。
- **音频直接拷贝**：`-c:a copy` 保留原始音轨不重新编码。
- **overlay 链过长时**：若替换帧较多（>10张），ffmpeg 单条命令可能很慢。可分批执行：先替换前几张生成中间文件，再在中间文件上替换后续。
- **帧号从 0 开始**：`between(n,100,200)` 表示第 100~200 帧（闭区间，共 101 帧）。
- **执行前必须确认**：向用户展示完整的帧范围替换表，获得确认后才执行。

## 执行流程

```
1. 列出所有需要替换的帧范围（文件名、起始帧、结束帧、时间范围）
2. 展示给用户确认
3. 用户确认后，构建并执行 ffmpeg 命令
4. 验证输出视频（抽查替换区间的画面）
```
