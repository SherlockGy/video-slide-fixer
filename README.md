# Video Slide Fixer

[中文说明](README.zh.md)

Fix slide quality issues in AI-generated videos.

## What It Does

AI tools (e.g., NotebookLM) often produce presentation videos with distorted CJK characters, garbled edges, and visual artifacts. This skill helps you:

1. **Extract** — Detect scene changes and capture each slide frame
2. **Inspect** — Review frames for blurry text, garbled characters, and artifacts
3. **Fix** — Regenerate problematic frames via Gemini API
4. **Replace** — Write fixed images back into the video

## Prerequisites

- Gemini API key (see setup below)
- All other tools (ffmpeg, ffprobe, fend, slides-fix) are bundled in `scripts/`

## API Key Setup

Create a `.env` file in your **project working directory** (e.g., where the video file is):

```
GEMINI_API_KEY=your_api_key_here
```

Get your key at: https://aistudio.google.com/apikey

> Template: `tool-source/slides-fix/.env.example`
>
> The `.env` file contains sensitive credentials — do not commit it. It is already excluded in `.gitignore`.

## Directory Structure

```
video-slide-fixer/
├── SKILL.md           ← Full instruction guide (read by Claude Code)
├── README.md          ← This file
├── scripts/
│   ├── ffmpeg.exe     ← Video processing
│   ├── ffprobe.exe    ← Video metadata analysis
│   ├── fend.exe       ← Precise math calculations
│   ├── slides-fix.exe ← Image repair tool (prebuilt)
│   └── model.conf     ← Model configuration
├── references/        ← Detailed reference docs (loaded on demand)
└── tool-source/
    └── slides-fix/    ← Go source for the repair tool
```

## Model Switching

Edit `scripts/model.conf` to switch Gemini models. Lines starting with `#` are comments; the first non-comment, non-empty line is used.

Default (Pro):
```
gemini-3-pro-image-preview
# gemini-3.1-flash-image-preview
```

Switch to Flash:
```
# gemini-3-pro-image-preview
gemini-3.1-flash-image-preview
```

Priority: `-model` CLI flag > `model.conf` > built-in default

## Limitations

- Gemini-generated fixes may not be perfect — prompt tuning is often needed
- Gemini API has rate limits; batch processing requires patience
- Fix quality depends on prompt quality — use diff-style prompts (describe only what to change) for best results
