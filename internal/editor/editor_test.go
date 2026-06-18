package editor

import (
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "empty string",
			input: "",
			want:  0,
		},
		{
			name:  "plain ASCII",
			input: "hello world",
			want:  11,
		},
		{
			name:  "default colored prompt",
			input: "\033[32mscout@\033[36mscout:\033[34m/mnt\033[32m$ \033[0m",
			want:  18,
		},
		{
			name:  "home dir prompt",
			input: "\033[32mscout@\033[36mscout:\033[34m~\033[32m$ \033[0m",
			want:  15,
		},
		{
			name:  "SGR reset only",
			input: "hello\033[0m world",
			want:  11,
		},
		{
			name:  "non-SGR CSI cursor up",
			input: "line1\033[Aline2",
			want:  10,
		},
		{
			name:  "non-SGR CSI erase display",
			input: "before\033[2Jafter",
			want:  11,
		},
		{
			name:  "non-SGR CSI with parameters",
			input: "a\033[12Cb",
			want:  2,
		},
		{
			name:  "multiple consecutive escapes",
			input: "\033[31m\033[1m\033[4mbold red underline\033[0m",
			want:  18,
		},
		{
			name:  "escape at end",
			input: "hello\033[31m",
			want:  5,
		},
		{
			name:  "no escapes",
			input: "plain text with spaces 123",
			want:  26,
		},
		{
			name:  "CSI with semicolon parameters",
			input: "\033[38;5;196mred\033[0m",
			want:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibleWidth(tt.input)
			if got != tt.want {
				t.Errorf("visibleWidth(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCursorPosition(t *testing.T) {
	prompt := "\033[32mscout@\033[36mscout:\033[34m/mnt\033[32m$ \033[0m"
	promptWidth := visibleWidth(prompt)
	if promptWidth != 18 {
		t.Fatalf("visibleWidth(prompt) = %d, want 18", promptWidth)
	}

	tests := []struct {
		name     string
		line     string
		pos      int
		wantCol  int
	}{
		{
			name:    "empty line",
			line:    "",
			pos:     0,
			wantCol: 18,
		},
		{
			name:    "ASCII at start",
			line:    "abc",
			pos:     0,
			wantCol: 18,
		},
		{
			name:    "ASCII at end",
			line:    "abc",
			pos:     3,
			wantCol: 21,
		},
		{
			name:    "ASCII middle",
			line:    "abcdef",
			pos:     3,
			wantCol: 21,
		},
		{
			name:    "multi-byte UTF-8 single char",
			line:    "\u00f1",
			pos:     2,
			wantCol: 19,
		},
		{
			name:    "mixed ASCII and multi-byte",
			line:    "a\u00f1b",
			pos:     4,
			wantCol: 21,
		},
		{
			name:    "wide CJK character",
			line:    "\u4e2d",
			pos:     3,
			wantCol: 20,
		},
		{
			name:    "mixed ASCII and CJK",
			line:    "a\u4e2db",
			pos:     5,
			wantCol: 22,
		},
		{
			name:    "emoji wide",
			line:    "\U0001f680",
			pos:     4,
			wantCol: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(func() string { return prompt }, false)
			e.line = []byte(tt.line)
			e.pos = tt.pos

			gotCol := visibleWidth(prompt) + runewidth.StringWidth(string(e.line[:e.pos]))
			if gotCol != tt.wantCol {
				t.Errorf("cursor column = %d, want %d (promptWidth=%d, line=%q, pos=%d, linePrefix=%q)",
					gotCol, tt.wantCol, promptWidth, tt.line, tt.pos, string(e.line[:e.pos]))
			}
		})
	}
}

func TestVisibleWidthEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "CSI without final byte skipped",
			input: "a\033[b",
			want:  1,
		},
		{
			name:  "esc followed by non-bracket counted as visible",
			input: "a\033Xb",
			want:  4,
		},
		{
			name:  "lone esc at end",
			input: "hello\033",
			want:  6,
		},
		{
			name:  "esc bracket at end starts CSI",
			input: "hello\033[",
			want:  5,
		},
		{
			name:  "truecolor sequence",
			input: "\033[48;2;255;0;0mbg-red\033[0m",
			want:  6,
		},
		{
			name:  "hide/show cursor sequences",
			input: "\033[?25lhidden\033[?25h",
			want:  6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibleWidth(tt.input)
			if got != tt.want {
				t.Errorf("visibleWidth(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCursorPosWithGhost(t *testing.T) {
	prompt := "\033[32mscout@\033[36mscout:\033[34m/mnt\033[32m$ \033[0m"
	promptWidth := visibleWidth(prompt)

	e := New(func() string { return prompt }, false)
	e.line = []byte("ls")
	e.pos = 2
	e.ghost = "-la"
	e.ghostAt = 2

	gotCol := promptWidth + runewidth.StringWidth(string(e.line[:e.pos]))
	if gotCol != 20 {
		t.Errorf("cursor column with ghost = %d, want 20 (ghost should not affect cursor)", gotCol)
	}
}
