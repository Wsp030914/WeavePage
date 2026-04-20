package service

import (
	"ToDoList/server/config"
	apperrors "ToDoList/server/errors"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	einodeepseek "github.com/cloudwego/eino-ext/components/model/deepseek"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type AIDraftRequest struct {
	TaskID      int    `json:"task_id"`
	Title       string `json:"title"`
	Instruction string `json:"instruction"`
	DocType     string `json:"doc_type"`
}

type AIContinueRequest struct {
	TaskID       int    `json:"task_id"`
	Title        string `json:"title"`
	SelectedText string `json:"selected_text"`
	FullContext  string `json:"full_context"`
	Instruction  string `json:"instruction"`
}

type AIMeetingRequest struct {
	TaskID      int    `json:"task_id"`
	Title       string `json:"title"`
	Transcript  string `json:"transcript"`
	Notes       string `json:"notes"`
	Instruction string `json:"instruction"`
}

type AIMeetingAction struct {
	Title     string `json:"title"`
	OwnerHint string `json:"owner_hint"`
	DueHint   string `json:"due_hint"`
}

type AIMeetingPreview struct {
	MinutesMarkdown string            `json:"minutes_markdown"`
	Summary         string            `json:"summary"`
	Decisions       []string          `json:"decisions"`
	Actions         []AIMeetingAction `json:"actions"`
}

type AIService struct {
	cfg            config.AIConfig
	chatModel      model.BaseChatModel
	draftRunner    compose.Runnable[map[string]any, *schema.Message]
	continueRunner compose.Runnable[map[string]any, *schema.Message]
	meetingRunner  compose.Runnable[meetingWorkflowInput, *AIMeetingPreview]
}

type meetingWorkflowInput struct {
	Topic       string
	Transcript  string
	Notes       string
	Instruction string
}

type meetingNormalizedInput struct {
	SourceText string
}

type meetingSummaryResult struct {
	Summary   string   `json:"summary"`
	Decisions []string `json:"decisions"`
}

type meetingTaskExtractInput struct {
	SourceText string   `json:"source_text"`
	Decisions  []string `json:"decisions"`
}

type meetingTaskExtractResult struct {
	Actions []AIMeetingAction `json:"actions"`
}

type meetingAssembleInput struct {
	Topic     string
	Summary   string
	Decisions []string
	Actions   []AIMeetingAction
}

func NewAIService(ctx context.Context, cfg config.AIConfig) (*AIService, error) {
	svc := &AIService{cfg: cfg}
	if !svc.IsConfigured() {
		return svc, nil
	}

	chatModel, err := newAIChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	draftRunner, err := buildDraftWorkflow(ctx, chatModel)
	if err != nil {
		return nil, err
	}
	continueRunner, err := buildContinueWorkflow(ctx, chatModel)
	if err != nil {
		return nil, err
	}
	meetingRunner, err := buildMeetingWorkflow(ctx, chatModel)
	if err != nil {
		return nil, err
	}

	svc.chatModel = chatModel
	svc.draftRunner = draftRunner
	svc.continueRunner = continueRunner
	svc.meetingRunner = meetingRunner
	return svc, nil
}

func (s *AIService) IsConfigured() bool {
	return strings.TrimSpace(s.cfg.APIKey) != "" && strings.TrimSpace(s.cfg.Model) != ""
}

func (s *AIService) ensureReady() error {
	if s == nil || !s.IsConfigured() || s.chatModel == nil || s.draftRunner == nil || s.continueRunner == nil || s.meetingRunner == nil {
		return apperrors.NewInternalError("AI service is not configured")
	}
	return nil
}

func (s *AIService) StreamDraft(ctx context.Context, req AIDraftRequest, onChunk func(string) error) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return apperrors.NewParamError("title is required")
	}
	stream, err := s.draftRunner.Stream(ctx, map[string]any{
		"title":       clipAIInput(title, s.cfg.MaxInputChars),
		"instruction": clipAIInput(defaultDraftInstruction(req.Instruction), s.cfg.MaxInputChars),
		"doc_type":    clipAIInput(req.DocType, 64),
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	return consumeMessageStream(stream, onChunk)
}

func (s *AIService) StreamContinue(ctx context.Context, req AIContinueRequest, onChunk func(string) error) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		return apperrors.NewParamError("instruction is required")
	}
	selectedText := clipAIInput(req.SelectedText, s.cfg.MaxInputChars/2)
	fullContext := clipAIInput(req.FullContext, s.cfg.MaxInputChars)
	if selectedText == "" && fullContext == "" {
		return apperrors.NewParamError("selected_text or full_context is required")
	}

	stream, err := s.continueRunner.Stream(ctx, map[string]any{
		"title":         clipAIInput(req.Title, 256),
		"selected_text": selectedText,
		"full_context":  fullContext,
		"instruction":   clipAIInput(instruction, 1000),
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	return consumeMessageStream(stream, onChunk)
}

func (s *AIService) GenerateMeetingPreview(ctx context.Context, req AIMeetingRequest) (*AIMeetingPreview, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	transcript := clipAIInput(req.Transcript, s.cfg.MaxInputChars)
	notes := clipAIInput(req.Notes, s.cfg.MaxInputChars)
	if transcript == "" && notes == "" {
		return nil, apperrors.NewParamError("transcript or notes is required")
	}

	preview, err := s.meetingRunner.Invoke(ctx, meetingWorkflowInput{
		Topic:       clipAIInput(req.Title, 256),
		Transcript:  transcript,
		Notes:       notes,
		Instruction: clipAIInput(defaultMeetingInstruction(req.Instruction), 1000),
	})
	if err != nil {
		return nil, err
	}
	if preview == nil {
		return nil, apperrors.NewInternalError("meeting preview is empty")
	}
	return preview, nil
}

func buildDraftWorkflow(ctx context.Context, chatModel model.BaseChatModel) (compose.Runnable[map[string]any, *schema.Message], error) {
	template := prompt.FromMessages(
		schema.FString,
		schema.SystemMessage(`你是一个专业的 Markdown 文稿起草助手。
你会根据标题、文档类型和补充要求，生成一份可继续编辑的中文文稿初稿。

输出要求：
1. 只输出 Markdown 正文，不要解释过程。
2. 标题明确，结构清晰。
3. 默认包含 3-5 个二级标题。
4. 如果适合，补一组列表项。
5. 输出要像真实工作文稿，而不是模板占位符。`),
		schema.UserMessage(`文档类型：{doc_type}
标题：{title}
补充要求：{instruction}

请直接输出 Markdown 初稿。`),
	)

	chain := compose.NewChain[map[string]any, *schema.Message]()
	chain.AppendChatTemplate(template).AppendChatModel(chatModel)
	return chain.Compile(ctx)
}

func buildContinueWorkflow(ctx context.Context, chatModel model.BaseChatModel) (compose.Runnable[map[string]any, *schema.Message], error) {
	template := prompt.FromMessages(
		schema.FString,
		schema.SystemMessage(`你是一个专业的中文写作助手。
你会根据用户给出的选中文本、全文上下文和编辑指令，对内容进行续写、改写、扩写或压缩。

输出要求：
1. 只输出处理后的结果，不要解释。
2. 保持语气、术语和上下文连续。
3. 如果用户提供了选中文本，优先围绕选中文本处理。
4. 输出使用 Markdown 兼容文本。`),
		schema.UserMessage(`标题：{title}

选中文本：
{selected_text}

全文上下文：
{full_context}

编辑指令：
{instruction}

请直接输出结果。`),
	)

	chain := compose.NewChain[map[string]any, *schema.Message]()
	chain.AppendChatTemplate(template).AppendChatModel(chatModel)
	return chain.Compile(ctx)
}

func buildMeetingWorkflow(ctx context.Context, chatModel model.BaseChatModel) (compose.Runnable[meetingWorkflowInput, *AIMeetingPreview], error) {
	normalize := newMeetingNormalizeLambda()
	summaryChain := buildMeetingSummaryChain(chatModel)
	taskChain := buildMeetingTaskExtractChain(chatModel)
	assemble := newMeetingAssembleLambda()

	wf := compose.NewWorkflow[meetingWorkflowInput, *AIMeetingPreview]()

	wf.AddLambdaNode("normalize", normalize).
		AddInput(compose.START)

	wf.AddGraphNode("summary", summaryChain).
		AddInput("normalize", compose.MapFields("SourceText", "SourceText"))

	wf.AddGraphNode("task_extract", taskChain).
		AddInput("summary", compose.MapFields("Decisions", "Decisions")).
		AddInputWithOptions(
			"normalize",
			[]*compose.FieldMapping{compose.MapFields("SourceText", "SourceText")},
			compose.WithNoDirectDependency(),
		)

	wf.AddLambdaNode("assemble", assemble).
		AddInput("summary",
			compose.MapFields("Summary", "Summary"),
			compose.MapFields("Decisions", "Decisions"),
		).
		AddInput("task_extract", compose.MapFields("Actions", "Actions")).
		AddInputWithOptions(
			compose.START,
			[]*compose.FieldMapping{compose.MapFields("Topic", "Topic")},
			compose.WithNoDirectDependency(),
		)

	wf.End().AddInput("assemble")

	return wf.Compile(ctx)
}

func newMeetingNormalizeLambda() *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, input meetingWorkflowInput) (*meetingNormalizedInput, error) {
		sourceText := normalizeMeetingSourceText(input.Transcript, input.Notes, input.Instruction)
		if sourceText == "" {
			return nil, apperrors.NewParamError("transcript or notes is required")
		}
		return &meetingNormalizedInput{
			SourceText: sourceText,
		}, nil
	})
}

func buildMeetingSummaryChain(chatModel model.BaseChatModel) *compose.Chain[meetingNormalizedInput, *meetingSummaryResult] {
	template := prompt.FromMessages(
		schema.FString,
		schema.SystemMessage(`你是会议总结助手。请根据输入的会议原始内容提炼核心摘要和明确结论。
只输出 JSON，不要输出 Markdown、解释、前后缀或代码块。

输出格式必须严格为：
{
  "summary": "string",
  "decisions": ["string"]
}

要求：
1. summary 用 2-4 句概括会议核心信息。
2. decisions 只保留已经明确达成的决定，不要写讨论过程。
3. 如果没有明确结论，decisions 返回空数组。
4. 不要输出 JSON 之外的任何内容。`),
		schema.UserMessage(`会议原始内容：
{SourceText}`),
	)

	return compose.NewChain[meetingNormalizedInput, *meetingSummaryResult]().
		AppendChatTemplate(template).
		AppendChatModel(chatModel).
		AppendLambda(compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*meetingSummaryResult, error) {
			if msg == nil {
				return nil, apperrors.NewInternalError("meeting summary is empty")
			}

			result, err := parseAIJSONResponse[meetingSummaryResult](msg.Content)
			if err != nil {
				return nil, err
			}

			result.Summary = strings.TrimSpace(result.Summary)
			result.Decisions = compactNonEmptyStrings(result.Decisions)
			return result, nil
		}))
}

func buildMeetingTaskExtractChain(chatModel model.BaseChatModel) *compose.Chain[meetingTaskExtractInput, *meetingTaskExtractResult] {
	template := prompt.FromMessages(
		schema.FString,
		schema.SystemMessage(`你是会议行动项提取助手。请根据会议原始内容和已确认结论，提取可以执行的行动项候选。
只输出 JSON，不要输出 Markdown、解释、前后缀或代码块。

输出格式必须严格为：
{
  "actions": [
    {
      "title": "string",
      "owner_hint": "string",
      "due_hint": "string"
    }
  ]
}

要求：
1. 只提取可执行、可跟进的行动项，避免空泛表述。
2. owner_hint 和 due_hint 在原文不明确时可返回空字符串。
3. 如果没有明确行动项，actions 返回空数组。
4. 不要输出 JSON 之外的任何内容。`),
		schema.UserMessage(`已确认结论：
{Decisions}

会议原始内容：
{SourceText}`),
	)

	return compose.NewChain[meetingTaskExtractInput, *meetingTaskExtractResult]().
		AppendChatTemplate(template).
		AppendChatModel(chatModel).
		AppendLambda(compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*meetingTaskExtractResult, error) {
			if msg == nil {
				return nil, apperrors.NewInternalError("meeting task extraction is empty")
			}

			result, err := parseAIJSONResponse[meetingTaskExtractResult](msg.Content)
			if err != nil {
				return nil, err
			}

			result.Actions = normalizeMeetingActions(result.Actions)
			return result, nil
		}))
}

func newMeetingAssembleLambda() *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, input meetingAssembleInput) (*AIMeetingPreview, error) {
		summary := strings.TrimSpace(input.Summary)
		decisions := compactNonEmptyStrings(input.Decisions)
		actions := normalizeMeetingActions(input.Actions)

		return &AIMeetingPreview{
			MinutesMarkdown: buildMeetingMinutesMarkdown(input.Topic, summary, decisions, actions),
			Summary:         summary,
			Decisions:       decisions,
			Actions:         actions,
		}, nil
	})
}

func newAIChatModel(ctx context.Context, cfg config.AIConfig) (model.BaseChatModel, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "deepseek":
		cm, err := einodeepseek.NewChatModel(ctx, &einodeepseek.ChatModelConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			Timeout: cfg.Timeout,
		})
		if err != nil {
			return nil, err
		}
		return cm, nil
	default:
		maxCompletion := cfg.MaxCompletionTokens
		cm, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
			APIKey:              cfg.APIKey,
			BaseURL:             cfg.BaseURL,
			Model:               cfg.Model,
			Timeout:             cfg.Timeout,
			MaxCompletionTokens: &maxCompletion,
		})
		if err != nil {
			return nil, err
		}
		return cm, nil
	}
}

func consumeMessageStream(stream *schema.StreamReader[*schema.Message], onChunk func(string) error) error {
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg == nil || msg.Content == "" {
			continue
		}
		if err := onChunk(msg.Content); err != nil {
			return err
		}
	}
}

func parseAIJSONResponse[T any](raw string) (*T, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, io.EOF
	}
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}

	var out T
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func clipAIInput(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func defaultDraftInstruction(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return "生成一份结构清晰、可直接编辑的 Markdown 初稿。"
}

func defaultMeetingInstruction(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return "整理 transcript 或笔记，输出会议纪要、结论和行动项候选。"
}

func normalizeMeetingSourceText(transcript, notes, instruction string) string {
	parts := make([]string, 0, 3)

	if value := strings.TrimSpace(transcript); value != "" {
		parts = append(parts, "## Transcript\n"+value)
	}
	if value := strings.TrimSpace(notes); value != "" {
		parts = append(parts, "## Notes\n"+value)
	}
	if value := strings.TrimSpace(instruction); value != "" {
		parts = append(parts, "## Extra Instruction\n"+value)
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func compactNonEmptyStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, item)
	}

	if len(result) == 0 {
		return []string{}
	}
	return result
}

func normalizeMeetingActions(actions []AIMeetingAction) []AIMeetingAction {
	if len(actions) == 0 {
		return []AIMeetingAction{}
	}

	result := make([]AIMeetingAction, 0, len(actions))
	for _, action := range actions {
		title := strings.TrimSpace(action.Title)
		if title == "" {
			continue
		}

		result = append(result, AIMeetingAction{
			Title:     title,
			OwnerHint: strings.TrimSpace(action.OwnerHint),
			DueHint:   strings.TrimSpace(action.DueHint),
		})
	}

	if len(result) == 0 {
		return []AIMeetingAction{}
	}
	return result
}

func buildMeetingMinutesMarkdown(topic, summary string, decisions []string, actions []AIMeetingAction) string {
	var builder strings.Builder

	title := strings.TrimSpace(topic)
	if title == "" {
		title = "会议纪要"
	}

	builder.WriteString("# ")
	builder.WriteString(title)
	builder.WriteString("\n\n")

	builder.WriteString("## 摘要\n")
	if summary == "" {
		builder.WriteString("暂无摘要。\n\n")
	} else {
		builder.WriteString(summary)
		builder.WriteString("\n\n")
	}

	builder.WriteString("## 结论\n")
	if len(decisions) == 0 {
		builder.WriteString("- 暂无明确结论\n\n")
	} else {
		for _, decision := range decisions {
			builder.WriteString("- ")
			builder.WriteString(decision)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## 行动项\n")
	if len(actions) == 0 {
		builder.WriteString("- 暂无明确行动项\n")
		return builder.String()
	}

	for _, action := range actions {
		builder.WriteString("- [ ] ")
		builder.WriteString(action.Title)

		meta := make([]string, 0, 2)
		if action.OwnerHint != "" {
			meta = append(meta, fmt.Sprintf("负责人：%s", action.OwnerHint))
		}
		if action.DueHint != "" {
			meta = append(meta, fmt.Sprintf("时间：%s", action.DueHint))
		}
		if len(meta) > 0 {
			builder.WriteString("（")
			builder.WriteString(strings.Join(meta, "；"))
			builder.WriteString("）")
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
