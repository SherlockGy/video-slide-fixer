package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
)

const defaultModel = "gemini-3.1-flash-image-preview"

type Task struct {
	Image  string `json:"image"`
	Output string `json:"output"`
	Prompt string `json:"prompt"`
}

func main() {
	imagePath := flag.String("image", "", "输入图片路径（单张模式）")
	prompt := flag.String("prompt", "", "提示词文本（单张模式）")
	output := flag.String("output", "", "输出图片路径（单张模式，留空自动生成）")
	batchFile := flag.String("batch", "", "批量任务 JSON 文件路径")
	model := flag.String("model", defaultModel, "Gemini 模型名称")
	envFile := flag.String("env", ".env", ".env 文件路径")
	delay := flag.Int("delay", 10, "批量请求间隔秒数")
	concurrency := flag.Int("concurrency", 1, "批量并发数（同时处理的任务数）")
	flag.Parse()

	if _, err := os.Stat(*envFile); os.IsNotExist(err) {
		fmt.Printf("错误：当前目录下未找到 %s 文件。\n", *envFile)
		fmt.Println("请创建该文件并填入 Gemini API 密钥：")
		fmt.Println()
		fmt.Println("  GEMINI_API_KEY=你的API密钥")
		fmt.Println()
		fmt.Println("密钥获取地址：https://aistudio.google.com/apikey")
		os.Exit(1)
	} else if err := godotenv.Load(*envFile); err != nil {
		fmt.Printf("错误：无法加载 %s: %v\n", *envFile, err)
		os.Exit(1)
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" || apiKey == "在此填写你的API密钥" {
		fmt.Printf("错误：%s 中未设置 GEMINI_API_KEY。\n", *envFile)
		fmt.Println("请填入你的 API 密钥。")
		os.Exit(1)
	}

	ctx := context.Background()

	if *batchFile != "" {
		runBatch(ctx, apiKey, *batchFile, *model, *delay, *concurrency)
	} else if *imagePath != "" && *prompt != "" {
		outPath := *output
		if outPath == "" {
			base := strings.TrimSuffix(filepath.Base(*imagePath), filepath.Ext(*imagePath))
			outPath = base + "_fixed.png"
		}
		runSingle(ctx, apiKey, *imagePath, *prompt, outPath, *model)
	} else {
		fmt.Println("用法：")
		fmt.Println("  单张模式：slides-fix -image <路径> -prompt <提示词> [-output <输出路径>]")
		fmt.Println("  批量模式：slides-fix -batch <tasks.json>")
		fmt.Println()
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func runSingle(ctx context.Context, apiKey, imagePath, prompt, output, model string) {
	fmt.Printf("处理中：%s\n", imagePath)
	fmt.Printf("  模型：%s\n", model)
	promptRunes := []rune(prompt)
	if len(promptRunes) > 40 {
		promptRunes = promptRunes[:40]
	}
	fmt.Printf("  提示词：%s...\n", string(promptRunes))

	gc, err := NewGeminiClient(ctx, apiKey, model)
	if err != nil {
		fmt.Printf("  失败：%v\n", err)
		os.Exit(1)
	}

	data, mime, err := gc.EditImageWithRetry(ctx, imagePath, prompt, 2)
	if err != nil {
		fmt.Printf("  失败：%v\n", err)
		os.Exit(1)
	}

	if err := saveImage(output, data, mime); err != nil {
		fmt.Printf("  保存失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  完成 -> %s (%d 字节)\n", output, len(data))
}

type indexedTask struct {
	index int
	task  Task
}

func runBatch(ctx context.Context, apiKey, batchFile, model string, delay, concurrency int) {
	raw, err := os.ReadFile(batchFile)
	if err != nil {
		fmt.Printf("错误：读取 %s 失败: %v\n", batchFile, err)
		os.Exit(1)
	}

	var tasks []Task
	if err := json.Unmarshal(raw, &tasks); err != nil {
		fmt.Printf("错误：解析 %s 失败: %v\n", batchFile, err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("警告：任务列表为空，无需处理。")
		return
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 20 {
		fmt.Printf("警告：并发数已限制为最大值 20（原始值 %d）\n", concurrency)
		concurrency = 20
	}
	if concurrency > len(tasks) {
		concurrency = len(tasks)
	}

	fmt.Printf("批量任务：共 %d 项，模型=%s，间隔=%d秒，并发=%d\n\n", len(tasks), model, delay, concurrency)

	// 创建共享客户端（复用连接）
	gc, err := NewGeminiClient(ctx, apiKey, model)
	if err != nil {
		fmt.Printf("错误：%v\n", err)
		os.Exit(1)
	}

	var successCount, failedCount atomic.Int32
	var mu sync.Mutex
	var failedNames []string // 记录失败任务
	taskCh := make(chan indexedTask, len(tasks))
	var wg sync.WaitGroup

	// 启动 worker
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range taskCh {
				i, task := it.index, it.task
				tag := fmt.Sprintf("[%d/%d] %s", i+1, len(tasks), filepath.Base(task.Image))

				if _, err := os.Stat(task.Image); os.IsNotExist(err) {
					mu.Lock()
					fmt.Printf("%s\n  跳过：文件不存在\n", tag)
					failedNames = append(failedNames, filepath.Base(task.Image))
					mu.Unlock()
					failedCount.Add(1)
					continue
				}

				outDir := filepath.Dir(task.Output)
				if outDir != "" && outDir != "." {
					if err := os.MkdirAll(outDir, 0755); err != nil {
						mu.Lock()
						fmt.Printf("%s\n  创建输出目录失败：%v\n", tag, err)
						failedNames = append(failedNames, filepath.Base(task.Image))
						mu.Unlock()
						failedCount.Add(1)
						continue
					}
				}

				mu.Lock()
				fmt.Printf("%s\n  请求中...\n", tag)
				mu.Unlock()

				data, mime, err := gc.EditImageWithRetry(ctx, task.Image, task.Prompt, 2)
				if err != nil {
					mu.Lock()
					fmt.Printf("%s\n  失败：%v\n", tag, err)
					failedNames = append(failedNames, filepath.Base(task.Image))
					mu.Unlock()
					failedCount.Add(1)
				} else {
					if err := saveImage(task.Output, data, mime); err != nil {
						mu.Lock()
						fmt.Printf("%s\n  保存失败：%v\n", tag, err)
						failedNames = append(failedNames, filepath.Base(task.Image))
						mu.Unlock()
						failedCount.Add(1)
					} else {
						mu.Lock()
						fmt.Printf("%s\n  完成 -> %s (%d 字节)\n", tag, task.Output, len(data))
						mu.Unlock()
						successCount.Add(1)
					}
				}

				// 请求间隔（限流保护）
				if delay > 0 {
					time.Sleep(time.Duration(delay) * time.Second)
				}
			}
		}()
	}

	// 分发任务
	for i, task := range tasks {
		taskCh <- indexedTask{index: i, task: task}
	}
	close(taskCh)

	wg.Wait()

	fmt.Printf("\n完成：%d 成功，%d 失败，共 %d 项\n", successCount.Load(), failedCount.Load(), len(tasks))
	if len(failedNames) > 0 {
		fmt.Println("\n失败任务列表：")
		for _, name := range failedNames {
			fmt.Printf("  - %s\n", name)
		}
	}
}

func saveImage(path string, data []byte, mime string) error {
	ext := filepath.Ext(path)
	if ext == "" {
		switch mime {
		case "image/png":
			path += ".png"
		case "image/webp":
			path += ".webp"
		default:
			path += ".png"
		}
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return os.WriteFile(path, data, 0644)
}
