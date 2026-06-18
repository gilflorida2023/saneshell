package editor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/saneshell/saneshell/internal/pty"
)

type CompletionItem struct {
	Text        string
	Kind        string `json:"kind"`
	Description string `json:"desc"`
	Detail      string `json:"detail"`
	Score       float64 `json:"score"`
}

type Editor struct {
	fd          int
	oldState    *term.State
	line        []byte
	pos         int
	promptFunc  func() string
	history     []string
	histIdx     int
	ghost       string
	ghostAt     int
	completions []CompletionItem
	compIdx     int
	showComp    bool
	showKind    bool
	compPrefix  string
	scanner     *bufio.Scanner
	viMode      bool  // true = normal mode, false = insert mode
	viPending   byte  // for multi-key vi commands (e.g. 'd' then 'd')
	useVI       bool  // whether vi mode is enabled
}

func New(promptFunc func() string, useVI bool) *Editor {
	return &Editor{
		fd:         int(os.Stdin.Fd()),
		line:       []byte{},
		pos:        0,
		promptFunc: promptFunc,
		history:    []string{},
		histIdx:    -1,
		useVI:      useVI,
		viMode:     false,
	}
}

func (e *Editor) ReadLine() (string, error) {
	if !term.IsTerminal(e.fd) {
		return e.readLineSimple()
	}

	var err error
	e.oldState, err = term.MakeRaw(e.fd)
	if err != nil {
		return e.readLineSimple()
	}
	defer term.Restore(e.fd, e.oldState)

	e.line = e.line[:0]
	e.pos = 0
	e.ghost = ""
	e.ghostAt = -1
	e.completions = nil
	e.showComp = false
	e.compIdx = -1
	e.compPrefix = ""

	e.render()

	return e.readRaw()
}

func (e *Editor) readLineSimple() (string, error) {
	if e.scanner == nil {
		e.scanner = bufio.NewScanner(os.Stdin)
	}
	if !e.scanner.Scan() {
		if err := e.scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return strings.TrimRight(e.scanner.Text(), "\r\n"), nil
}

func (e *Editor) readRaw() (string, error) {
	buf := make([]byte, 32)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return "", err
		}
		for i := 0; i < n; i++ {
			b := buf[i]
			if b == '\x1b' && i+1 < n {
				seq := parseEscape(buf[i:])
				i += seq.consumed - 1
				if seq.consumed == 0 {
					continue
				}
				switch seq.action {
				case actUp:
					e.historyPrev()
				case actDown:
					e.historyNext()
				case actRight:
					e.moveCursor(1)
				case actLeft:
					e.moveCursor(-1)
				case actHome:
					e.pos = 0
				case actEnd:
					e.pos = len(e.line)
					if e.ghost != "" && e.ghostAt >= 0 && e.pos >= e.ghostAt {
						e.acceptGhost()
					}
				case actDelete:
					e.deleteChar()
				case actWordLeft:
					e.moveWordLeft()
				case actWordRight:
					e.moveWordRight()
				}
				continue
			}
		switch b {
			case 0x1b: // standalone Esc (not part of escape sequence)
				if e.useVI {
					e.viMode = true
					e.viPending = 0
				}
			case 0x03: // Ctrl+C — clear line, show new prompt
				e.line = e.line[:0]
				e.pos = 0
				e.render()
				fmt.Print("\r\n")
				return "", fmt.Errorf("interrupted")
			case 0x04: // Ctrl+D — EOF on empty line
				if len(e.line) == 0 {
					fmt.Print("\r\n")
					return "", io.EOF
				}
			case 0x08, 0x7f: // Backspace
				e.backspace()
			case 0x09: // Tab
				e.doComplete()
			case 0x0a, 0x0d: // Enter
				line := string(e.line)
				fmt.Print("\r\n")
				if line != "" {
					e.history = append([]string{line}, e.history...)
					if len(e.history) > 10000 {
						e.history = e.history[:10000]
					}
				}
				e.histIdx = -1
				return line, nil
			case 0x0c: // Ctrl+L
				fmt.Print("\033[2J\033[H")
				e.render()
			case 0x0b: // Ctrl+K
				e.line = e.line[:e.pos]
			case 0x15: // Ctrl+U
				e.line = e.line[:0]
				e.pos = 0
			case 0x17: // Ctrl+W
				e.deleteWord()
			case 0x01: // Ctrl+A
				e.pos = 0
			case 0x05: // Ctrl+E
				e.pos = len(e.line)
			case 0x0e: // Ctrl+N
				e.historyNext()
			case 0x10: // Ctrl+P
				e.historyPrev()
			default:
				if e.useVI && e.viMode {
					e.handleViKey(b)
				} else if b >= 0x20 && b <= 0x7e {
					e.insertByte(b)
				}
			}
		}
		e.render()
		if len(e.ghost) > 0 {
			e.ghost = ""
			e.ghostAt = -1
		}
	}
}

type escapeAction int

const (
	actNone escapeAction = iota
	actUp
	actDown
	actRight
	actLeft
	actHome
	actEnd
	actDelete
	actWordLeft
	actWordRight
)

type escapeSeq struct {
	consumed int
	action   escapeAction
}

func parseEscape(buf []byte) escapeSeq {
	n := len(buf)
	if n < 2 || buf[0] != '\x1b' {
		return escapeSeq{consumed: 1}
	}
	if buf[1] == '[' {
		if n < 3 {
			return escapeSeq{consumed: 2}
		}
		switch buf[2] {
		case 'A':
			return escapeSeq{consumed: 3, action: actUp}
		case 'B':
			return escapeSeq{consumed: 3, action: actDown}
		case 'C':
			return escapeSeq{consumed: 3, action: actRight}
		case 'D':
			return escapeSeq{consumed: 3, action: actLeft}
		case 'F':
			return escapeSeq{consumed: 3, action: actEnd}
		case 'H':
			return escapeSeq{consumed: 3, action: actHome}
		case '3':
			if n >= 4 && buf[3] == '~' {
				return escapeSeq{consumed: 4, action: actDelete}
			}
		case '1', '7':
			if n >= 4 && buf[3] == '~' {
				return escapeSeq{consumed: 4, action: actHome}
			}
		case '4', '8':
			if n >= 4 && buf[3] == '~' {
				return escapeSeq{consumed: 4, action: actEnd}
			}
		}
		// Ctrl+Arrow: ESC [ 1 ; 5 C / D
		if n >= 6 && buf[2] == '1' && buf[3] == ';' && buf[4] == '5' {
			switch buf[5] {
			case 'C':
				return escapeSeq{consumed: 6, action: actWordRight}
			case 'D':
				return escapeSeq{consumed: 6, action: actWordLeft}
			}
		}
	}
	return escapeSeq{consumed: 1}
}

func (e *Editor) render() {
	fmt.Print("\r\033[K")
	if e.useVI && e.viMode {
		fmt.Print("\033[31m-- NORMAL --\033[0m\r\n")
		fmt.Print("\r\033[K")
	}
	prompt := e.promptFunc()
	fmt.Print(prompt)

	if e.ghost != "" && e.ghostAt >= 0 && e.ghostAt <= len(e.line) {
		before := string(e.line[:e.ghostAt])
		after := string(e.line[e.ghostAt:])
		fmt.Print(before)
		fmt.Print("\033[90m" + e.ghost + "\033[0m")
		fmt.Print(after)
	} else {
		fmt.Print(string(e.line))
	}

	if e.showComp && len(e.completions) > 0 {
		comps := e.completions
		if len(comps) > 10 {
			comps = comps[:10]
		}
		fmt.Print("\r\n")
		maxW := 0
		for _, item := range comps {
			if l := len(item.Text); l > maxW {
				maxW = l
			}
		}
		for idx, item := range comps {
			prefix := "  "
			if idx == e.compIdx {
				prefix = "\033[32m> \033[0m"
			}
			kind := ""
			if e.showKind && item.Kind != "" {
				kind = " \033[90m[" + item.Kind + "]\033[0m"
			}
			desc := ""
			if item.Description != "" {
				desc = "  \033[90m" + item.Description + "\033[0m"
			}
			padded := item.Text + strings.Repeat(" ", maxW-len(item.Text))
			fmt.Print(prefix + padded + kind + desc + "\r\n")
		}
	}

	cursorPos := visibleWidth(prompt) + runewidth.StringWidth(string(e.line[:e.pos]))
	fmt.Printf("\033[%dG", cursorPos+1)
}

// visibleWidth returns the number of visible columns occupied by s,
// stripping ANSI escape sequences which have zero display width.
func visibleWidth(s string) int {
	w := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] >= 0x20 && s[i] <= 0x2F || s[i] >= 0x30 && s[i] <= 0x3F) {
				i++
			}
			continue
		}
		w++
	}
	return w
}

func (e *Editor) moveCursor(delta int) {
	newPos := e.pos + delta
	if newPos < 0 {
		newPos = 0
	}
	if newPos > len(e.line) {
		newPos = len(e.line)
	}
	e.pos = newPos
	if e.ghost != "" && e.ghostAt >= 0 && e.pos >= e.ghostAt {
		e.acceptGhost()
	}
}

func (e *Editor) moveWordLeft() {
	for e.pos > 0 && e.line[e.pos-1] == ' ' {
		e.pos--
	}
	for e.pos > 0 && e.line[e.pos-1] != ' ' {
		e.pos--
	}
}

func (e *Editor) moveWordRight() {
	for e.pos < len(e.line) && e.line[e.pos] == ' ' {
		e.pos++
	}
	for e.pos < len(e.line) && e.line[e.pos] != ' ' {
		e.pos++
	}
	if e.ghost != "" && e.ghostAt >= 0 && e.pos >= e.ghostAt {
		e.acceptGhost()
	}
}

func (e *Editor) acceptGhost() {
	if e.ghost == "" || e.ghostAt < 0 {
		return
	}
	before := e.line[:e.ghostAt]
	after := make([]byte, len(e.line)-e.ghostAt)
	copy(after, e.line[e.ghostAt:])
	e.line = append(append(before, []byte(e.ghost)...), after...)
	e.pos = e.ghostAt + len(e.ghost)
	e.ghost = ""
	e.ghostAt = -1
}

func (e *Editor) insertByte(b byte) {
	e.line = append(e.line[:e.pos], append([]byte{b}, e.line[e.pos:]...)...)
	e.pos++
}

func (e *Editor) backspace() {
	if e.pos > 0 {
		e.line = append(e.line[:e.pos-1], e.line[e.pos:]...)
		e.pos--
	}
	e.showComp = false
	e.completions = nil
}

func (e *Editor) deleteChar() {
	if e.pos < len(e.line) {
		e.line = append(e.line[:e.pos], e.line[e.pos+1:]...)
	}
}

func (e *Editor) deleteWord() {
	start := e.pos
	for start > 0 && e.line[start-1] == ' ' {
		start--
	}
	for start > 0 && e.line[start-1] != ' ' {
		start--
	}
	e.line = append(e.line[:start], e.line[e.pos:]...)
	e.pos = start
}

func (e *Editor) historyPrev() {
	if len(e.history) == 0 {
		return
	}
	if e.histIdx == -1 {
		e.histIdx = 0
	} else if e.histIdx < len(e.history)-1 {
		e.histIdx++
	}
	e.line = []byte(e.history[e.histIdx])
	e.pos = len(e.line)
	e.showComp = false
	e.completions = nil
}

func (e *Editor) historyNext() {
	if e.histIdx <= 0 {
		e.histIdx = -1
		e.line = e.line[:0]
		e.pos = 0
	} else {
		e.histIdx--
		e.line = []byte(e.history[e.histIdx])
		e.pos = len(e.line)
	}
	e.showComp = false
	e.completions = nil
}

func (e *Editor) handleViKey(b byte) {
	if e.viPending != 0 {
		if e.viPending == 'd' && b == 'd' {
			e.line = e.line[:0]
			e.pos = 0
		} else if e.viPending == 'd' && b == 'w' {
			e.deleteWord()
		} else if e.viPending == 'd' && b == '$' {
			e.line = e.line[:e.pos]
		} else if e.viPending == 'y' && b == 'y' {
			// yy: yank line (no-op for now)
		}
		e.viPending = 0
		return
	}
	switch b {
	case 'h':
		e.moveCursor(-1)
	case 'j':
		e.historyNext()
	case 'k':
		e.historyPrev()
	case 'l':
		e.moveCursor(1)
	case 'i':
		e.viMode = false
	case 'I':
		e.pos = 0
		e.viMode = false
	case 'a':
		e.moveCursor(1)
		e.viMode = false
	case 'A':
		e.pos = len(e.line)
		e.viMode = false
	case 'x':
		e.deleteChar()
	case 'X':
		e.backspace()
	case '0':
		e.pos = 0
	case '$':
		e.pos = len(e.line)
	case 'w':
		e.moveWordRight()
	case 'b':
		e.moveWordLeft()
	case 'd':
		e.viPending = 'd'
	case 'D':
		e.line = e.line[:e.pos]
	case 'y':
		e.viPending = 'y'
	case 'u':
		e.line = e.line[:0]
		e.pos = 0
	case '~':
		if e.pos < len(e.line) {
			c := e.line[e.pos]
			if c >= 'a' && c <= 'z' {
				e.line[e.pos] = c - 32
			} else if c >= 'A' && c <= 'Z' {
				e.line[e.pos] = c + 32
			}
			e.moveCursor(1)
		}
	case 0x09: // Tab
		e.doComplete()
	}
}

func (e *Editor) doComplete() {
	line := string(e.line[:e.pos])
	word := line
	isFirstWord := true
	if idx := strings.LastIndex(line, " "); idx >= 0 {
		before := strings.Fields(line[:idx])
		isFirstWord = len(before) == 0
		word = line[idx+1:]
	}

	if isFirstWord {
		e.completions = completeCommand(word)
	} else {
		e.completions = completeFile(word)
		if idx := strings.LastIndex(word, "/"); idx >= 0 {
			e.compPrefix = word[:idx+1]
		} else {
			e.compPrefix = ""
		}
	}

	if len(e.completions) == 0 {
		return
	}

	if len(e.completions) == 1 {
		e.applyCompletion(0)
		return
	}

	common := longestCommonPrefix(e.completions)
	if len(common) > len(word) {
		e.applyCompletionText(common)
		return
	}

	e.showComp = true
	e.compIdx = 0
	e.render()
}

func (e *Editor) applyCompletion(idx int) {
	if idx < 0 || idx >= len(e.completions) {
		return
	}
	e.applyCompletionText(e.completions[idx].Text)
}

func (e *Editor) applyCompletionText(text string) {
	fullText := e.compPrefix + text
	e.compPrefix = ""

	line := string(e.line[:e.pos])
	wordStart := e.pos
	if idx := strings.LastIndex(line, " "); idx >= 0 {
		wordStart = idx + 1
	} else {
		wordStart = 0
	}
	after := make([]byte, len(e.line)-e.pos)
	copy(after, e.line[e.pos:])
	e.line = append(append(e.line[:wordStart], []byte(fullText)...), after...)
	e.pos = wordStart + len(fullText)
	e.showComp = false
	e.completions = nil
}

func (e *Editor) SetCompletions(items []CompletionItem) {
	e.completions = items
	e.showComp = len(items) > 0
	e.compIdx = 0
	if len(items) > 0 {
		e.applyCompletion(0)
	}
	e.render()
}

func (e *Editor) SetShowKind(v bool) {
	e.showKind = v
}

func (e *Editor) Line() string {
	return string(e.line)
}

func (e *Editor) CursorPos() int {
	return e.pos
}

// --- Completion sources (no shell dependent) ---

func completeCommand(prefix string) []CompletionItem {
	if prefix == "" {
		return nil
	}
	seen := make(map[string]bool)
	var items []CompletionItem

	// Commands from PATH
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			items = append(items, CompletionItem{Text: name, Kind: "command"})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Text < items[j].Text
	})
	if len(items) > 50 {
		items = items[:50]
	}
	return items
}

func completeFile(prefix string) []CompletionItem {
	if prefix == "" {
		return nil
	}

	dir := "."
	filePrefix := prefix

	if strings.HasPrefix(prefix, "~") {
		home, _ := os.UserHomeDir()
		prefix = home + prefix[1:]
	}

	if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
		dir = prefix[:idx+1]
		filePrefix = prefix[idx+1:]
		if !strings.HasPrefix(dir, "/") {
			cwd, _ := os.Getwd()
			dir = filepath.Join(cwd, dir)
		}
	} else {
		cwd, _ := os.Getwd()
		dir = cwd
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var items []CompletionItem
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, filePrefix) {
			continue
		}
		kind := "file"
		if entry.IsDir() {
			kind = "dir"
			name += "/"
		}
		suffix := ""
		if entry.IsDir() {
			suffix = "/"
		}
		items = append(items, CompletionItem{
			Text: name,
			Kind: kind,
		})
		_ = suffix
	}

	sort.Slice(items, func(i, j int) bool {
		// Dirs first
		if items[i].Kind != items[j].Kind {
			if items[i].Kind == "dir" {
				return true
			}
			return false
		}
		return items[i].Text < items[j].Text
	})
	if len(items) > 50 {
		items = items[:50]
	}
	return items
}

func longestCommonPrefix(items []CompletionItem) string {
	if len(items) == 0 {
		return ""
	}
	prefix := items[0].Text
	for _, item := range items[1:] {
		for !strings.HasPrefix(item.Text, prefix) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-utf8.RuneCountInString(prefix)]
		}
	}
	return prefix
}

func RunCmd(cmd string) (int, string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	rc := pty.ExecPTY(shell, cmd)
	if rc < 0 {
		return rc, "command failed"
	}
	return rc, ""
}