package ipc

import (
	"encoding/json"
	"time"
)

const (
	ProtocolVersion = 1
	SocketPath      = "/tmp/saneshell-%d.sock"
)

type MessageType string

const (
	// Core -> Intel
	MsgComplete   MessageType = "complete"
	MsgPreview    MessageType = "preview"
	MsgLearn      MessageType = "learn"
	MsgSuggest    MessageType = "suggest"
	MsgConfig     MessageType = "config"

	// Intel -> Core
	MsgCompletions  MessageType = "completions"
	MsgPreviewResp  MessageType = "preview"
	MsgAck          MessageType = "ack"
	MsgSuggestions  MessageType = "suggestions"
	MsgGhost        MessageType = "ghost"
	MsgWorkflow     MessageType = "workflow"
	MsgError        MessageType = "error"
)

type BaseMessage struct {
	Type    MessageType `json:"type"`
	Proto   int         `json:"proto"`
	ID      int64       `json:"id,omitempty"`
	TS      int64       `json:"ts"`
	Session string      `json:"session,omitempty"`
}

type CompletionRequest struct {
	BaseMessage
	Ctx CompletionContext `json:"ctx"`
}

type CompletionContext struct {
	Line   string   `json:"line"`
	Cursor int      `json:"cursor"`
	Words  []string `json:"words"`
	CWD    string   `json:"cwd"`
	Env    map[string]string `json:"env,omitempty"`
}

type PreviewRequest struct {
	BaseMessage
	Ctx PreviewContext `json:"ctx"`
}

type PreviewContext struct {
	Cmd string `json:"cmd"`
	CWD string `json:"cwd"`
}

type LearnRequest struct {
	BaseMessage
	Event LearnEvent `json:"event"`
}

type LearnEvent struct {
	Type        string `json:"type"` // "exec", "correction", "workflow"
	Cmd         string `json:"cmd,omitempty"`
	Corrected   string `json:"corrected,omitempty"`
	RC          int    `json:"rc,omitempty"`
	DurationMs  int64  `json:"duration_ms,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	Workflow    *Workflow `json:"workflow,omitempty"`
}

type Workflow struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
	Trigger string `json:"trigger"`
}

type SuggestRequest struct {
	BaseMessage
	Ctx SuggestContext `json:"ctx"`
}

type SuggestContext struct {
	History []string `json:"history"`
	CWD     string   `json:"cwd"`
}

type CompletionResponse struct {
	BaseMessage
	Items []CompletionItem `json:"items"`
}

type CompletionItem struct {
	Text        string `json:"text"`
	Kind        string `json:"kind"` // "command", "flag", "path", "branch", "variable", "alias"
	Description string `json:"desc,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

type PreviewResponse struct {
	BaseMessage
	Risk    string `json:"risk"` // "none", "low", "medium", "high"
	Impact  *Impact `json:"impact,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type Impact struct {
	Files     int   `json:"files,omitempty"`
	SizeBytes int64 `json:"size_bytes,omitempty"`
}

type SuggestionsResponse struct {
	BaseMessage
	Items []SuggestionItem `json:"items"`
}

type SuggestionItem struct {
	Cmd        string  `json:"cmd"`
	Desc       string  `json:"desc"`
	Confidence float64 `json:"confidence"`
}

type GhostNotification struct {
	BaseMessage
	Text string `json:"text"`
	At   int    `json:"at"`
}

type WorkflowNotification struct {
	BaseMessage
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
	Trigger string `json:"trigger"`
}

type ErrorResponse struct {
	BaseMessage
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewBaseMessage(msgType MessageType, id int64, session string) BaseMessage {
	return BaseMessage{
		Type:    msgType,
		Proto:   ProtocolVersion,
		ID:      id,
		TS:      time.Now().UnixMilli(),
		Session: session,
	}
}

func (m *BaseMessage) UnmarshalJSON(data []byte) error {
	type Alias BaseMessage
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	return json.Unmarshal(data, aux)
}

func UnmarshalMessage(data []byte) (Message, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	switch base.Type {
	case MsgCompletions:
		var msg CompletionResponse
		return &msg, json.Unmarshal(data, &msg)
	case MsgPreviewResp:
		var msg PreviewResponse
		return &msg, json.Unmarshal(data, &msg)
	case MsgAck:
		var msg BaseMessage
		return &msg, json.Unmarshal(data, &msg)
	case MsgSuggestions:
		var msg SuggestionsResponse
		return &msg, json.Unmarshal(data, &msg)
	case MsgGhost:
		var msg GhostNotification
		return &msg, json.Unmarshal(data, &msg)
	case MsgWorkflow:
		var msg WorkflowNotification
		return &msg, json.Unmarshal(data, &msg)
	case MsgError:
		var msg ErrorResponse
		return &msg, json.Unmarshal(data, &msg)
	default:
		return &base, nil
	}
}

type Message interface {
	GetBase() *BaseMessage
}

func (m *BaseMessage) GetBase() *BaseMessage { return m }
func (m *CompletionResponse) GetBase() *BaseMessage { return &m.BaseMessage }
func (m *PreviewResponse) GetBase() *BaseMessage { return &m.BaseMessage }
func (m *SuggestionsResponse) GetBase() *BaseMessage { return &m.BaseMessage }
func (m *GhostNotification) GetBase() *BaseMessage { return &m.BaseMessage }
func (m *WorkflowNotification) GetBase() *BaseMessage { return &m.BaseMessage }
func (m *ErrorResponse) GetBase() *BaseMessage { return &m.BaseMessage }