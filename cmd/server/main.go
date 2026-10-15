package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

type Config struct {
	ServerStatus string
	MaxTurns int
}

func NewConfig() *Config {
	return &Config{ServerStatus: "DOWN", MaxTurns: 10}
}

type Prompt struct {
	SystemPrompt string
}

func NewPrompt() *Prompt {
	return &Prompt{
		SystemPrompt: 
					`あなたはサーバー管理エージェントです。以下のツールが使えます。
					- "check_server_status": サーバーステータス(RUNNING or DOWN)を取得
					- "fetch_error_logs": エラーログを取得
					- "restart_server": サーバーを再起動

					【重要ルール】
					ツールを使う必要がある場合は、必ず以下のJSON形式「のみ」を出力してください（マークダウンの装飾や余計な文章は一切含めないこと）。
					{"isToolUse": true, "reason": "なぜそのツールを使うかの理由", "action": "ツール名"}

					すべての確認・対応が完了し、ユーザーへ最終報告をする段階になったら、以下のJSON形式で回答してください。
					{"isToolUse": false, "reply": "日本語で返信を考え入力してください"}`,
	}
}

type Result struct {
	Value string
	IsSuccess bool
}

func (c *Config)executeTool(tool string) *Result {
	switch(tool) {
		case "check_server_status":
			return &Result{Value: fmt.Sprintf(`{"status": "%s"}`, c.ServerStatus), IsSuccess: true}
		case "fetch_error_logs":
			return &Result{Value: `{"log": "ERROR: Out of memory (OOM) killed process."}`, IsSuccess: true}
		case "restart_server":
			c.ServerStatus = "RUNNING"
			return &Result{Value: `{"result": "Restart command executed."}`, IsSuccess: true}
		default:
			return &Result{Value: "", IsSuccess: false} // 一致するツールがない
	}
}

type Response struct {
	IsToolUse bool `json:"isToolUse"`
	Reason *string `json:"reason,omitempty"`
	Action *string `json:"action,omitempty"`
	Reply *string `json:"reply"`
}

func main() {
	var err error

	ctx := context.Background()

	config := NewConfig()

	client := anthropic.NewClient()

	prompt := NewPrompt()


	log.Println("====== AgentLoop体験開始 ======")
	log.Println("開始前のサーバーの状態")
	printConfig(config)

	userPrompt := "本番サーバーの様子を確認し、もし落ちていたら再起動させるなどして復旧させるように努めてください"

	log.Printf("User Prompt: [%s]", userPrompt)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
	}

	log.Printf("---AgentLoopスタート---\n")

	for step := 1; step <config.MaxTurns+1; step++ {

		var resp *anthropic.Message

		resp, err = client.Messages.New(ctx, anthropic.MessageNewParams{
			Model: anthropic.ModelClaudeHaiku5_5,
			MaxTokens: 1024,
			System: []anthropic.TextBlockParam{{Text: prompt.SystemPrompt}},
			Messages: messages,
		})

		if err != nil {
			log.Fatalf("ツールの呼び出し中にエラーが発生しました: %e\n", err)
		}

		var text string
		for _, b := range resp.Content {
			if b.Type == "text" {
				text = b.Text
				break
			}
		}

		rawText := extractJSON(strings.TrimSpace(text))

		messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(rawText)))

		var modelResp Response

		err = json.Unmarshal([]byte(rawText), &modelResp)

		if err != nil {
			log.Fatalf("モデルのレスポンスをパースできませなでした: %e", err)
		}

		var toolResult string

		if modelResp.IsToolUse && modelResp.Action != nil {
			var result *Result
			result = config.executeTool(*modelResp.Action)

			if !result.IsSuccess {
				toolResult = fmt.Sprintf(`{"error": true, "toolResult": "指定されたツールは存在しません:%v"}`, *modelResp.Action)
			} else {
				toolResult = result.Value
			}
		} else {
			log.Printf("Agent実行完了:\n%v\nターン数: %v\n", *modelResp.Reply, step)
			log.Println("最終的なサーバーの状態")
			printConfig(config)
			log.Println("======= AgentLoop終了 ======")
			break
		}

		log.Printf("%v\n", toolResult)

		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(toolResult)))
	}
}

func printConfig(c *Config) {
	log.Printf("ServerStatus: %v\n", c.ServerStatus)
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}
	return s
}


