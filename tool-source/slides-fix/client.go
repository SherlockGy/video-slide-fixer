package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/genai"
)

// GeminiClient 封装 Gemini API 客户端，支持复用连接。
type GeminiClient struct {
	client  *genai.Client
	model   string
	timeout time.Duration
}

// NewGeminiClient 创建一个可复用的 Gemini 客户端。
// timeout 用于设置 HTTP 请求超时（建议 ≥120s，图片生成较慢）。
func NewGeminiClient(ctx context.Context, apiKey, model string, timeout time.Duration) (*GeminiClient, error) {
	httpClient := &http.Client{
		Timeout: timeout,
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("创建客户端失败: %w", err)
	}
	return &GeminiClient{client: client, model: model, timeout: timeout}, nil
}

// EditImage 将图片+提示词发送到 Gemini API，返回生成的图片字节和 MIME 类型。
func (gc *GeminiClient) EditImage(ctx context.Context, imagePath, prompt string) ([]byte, string, error) {
	imgBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, "", fmt.Errorf("读取图片 %s 失败: %w", imagePath, err)
	}

	var mimeType string
	switch strings.ToLower(filepath.Ext(imagePath)) {
	case ".png":
		mimeType = "image/png"
	case ".webp":
		mimeType = "image/webp"
	case ".gif":
		mimeType = "image/gif"
	default:
		mimeType = "image/jpeg"
	}

	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
	}

	result, err := gc.client.Models.GenerateContent(
		ctx,
		gc.model,
		[]*genai.Content{{
			Parts: []*genai.Part{
				genai.NewPartFromText(prompt),
				genai.NewPartFromBytes(imgBytes, mimeType),
			},
		}},
		config,
	)
	if err != nil {
		return nil, "", fmt.Errorf("生成内容失败: %w", err)
	}

	if len(result.Candidates) == 0 || result.Candidates[0].Content == nil {
		return nil, "", fmt.Errorf("API 未返回候选结果")
	}

	for _, part := range result.Candidates[0].Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			return part.InlineData.Data, part.InlineData.MIMEType, nil
		}
	}

	// 收集文本部分用于调试（返回给调用方，不直接打印，以免并发输出交错）
	var textParts []string
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}

	errMsg := "响应中无图片（模型可能仅返回了文本）"
	if len(textParts) > 0 {
		errMsg += "；Gemini 文本回复: " + strings.Join(textParts, " | ")
	}
	return nil, "", fmt.Errorf("%s", errMsg)
}

// EditImageWithRetry 封装 EditImage，增加重试逻辑。
func (gc *GeminiClient) EditImageWithRetry(ctx context.Context, imagePath, prompt string, maxRetries int) ([]byte, string, error) {
	var lastErr error
	rateLimited := false
	for i := 0; i <= maxRetries; i++ {
		if i > 0 && !rateLimited {
			wait := time.Duration(5*(1<<(i-1))) * time.Second // 5s, 10s, 20s...
			fmt.Printf("  第 %d/%d 次重试，等待 %v...\n", i, maxRetries, wait)
			time.Sleep(wait)
		}
		rateLimited = false
		data, mime, err := gc.EditImage(ctx, imagePath, prompt)
		if err == nil {
			return data, mime, nil
		}
		lastErr = err
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "RESOURCE_EXHAUSTED") {
			fmt.Println("  触发限流，等待 60 秒...")
			time.Sleep(60 * time.Second)
			rateLimited = true
			continue
		}
		if i < maxRetries {
			fmt.Printf("  错误：%v\n", err)
		}
	}
	return nil, "", lastErr
}
