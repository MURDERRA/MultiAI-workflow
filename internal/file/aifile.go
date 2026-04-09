package file

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type QA struct {
	Question string
	Answer   string
}

type Message struct {
	Role    string
	Content string
}

type AIFile struct {
	Path     string
	Task     string
	QAs      []QA
	Prompt   string
	Route    string
	Provider string
	Reason   string
	History  []Message
}

func New(path, task string) (*AIFile, error) {
	af := &AIFile{Path: path, Task: task}
	return af, af.Write()
}

func Load(path string) (*AIFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	af := &AIFile{Path: path}
	if err := af.parse(string(data)); err != nil {
		return nil, err
	}
	return af, nil
}

func (af *AIFile) Write() error {
	return os.WriteFile(af.Path, []byte(af.Serialize()), 0644)
}

func (af *AIFile) Serialize() string {
	var sb strings.Builder

	sb.WriteString("[TASK]\n")
	sb.WriteString(strings.TrimSpace(af.Task))
	sb.WriteString("\n\n")

	if len(af.QAs) > 0 {
		sb.WriteString("[QUESTIONS]\n")
		for i, qa := range af.QAs {
			sb.WriteString(fmt.Sprintf("Q%d: %s\n", i+1, qa.Question))
			sb.WriteString(fmt.Sprintf("A%d: %s\n", i+1, qa.Answer))
		}
		sb.WriteString("\n")
	}

	if af.Prompt != "" {
		sb.WriteString("[PROMPT]\n")
		sb.WriteString("<<FINAL>>\n")
		sb.WriteString(strings.TrimSpace(af.Prompt))
		sb.WriteString("\n\n")
	}

	if af.Route != "" {
		sb.WriteString("[META]\n")
		sb.WriteString(fmt.Sprintf("route: %s\n", af.Route))
		sb.WriteString(fmt.Sprintf("provider: %s\n", af.Provider))
		sb.WriteString(fmt.Sprintf("reason: %s\n", af.Reason))
		sb.WriteString("\n")
	}

	if len(af.History) > 0 {
		sb.WriteString("[HISTORY]\n")
		for i, msg := range af.History {
			sb.WriteString(fmt.Sprintf("role: %s\n", msg.Role))
			sb.WriteString(msg.Content)
			sb.WriteString("\n")
			if i < len(af.History)-1 {
				sb.WriteString("---\n")
			}
		}
	}

	return sb.String()
}

func (af *AIFile) parse(content string) error {
	sections := splitSections(content)

	if task, ok := sections["TASK"]; ok {
		af.Task = strings.TrimSpace(task)
	}
	if qblock, ok := sections["QUESTIONS"]; ok {
		af.QAs = parseQAs(qblock)
	}
	if pblock, ok := sections["PROMPT"]; ok {
		p := strings.TrimPrefix(strings.TrimSpace(pblock), "<<FINAL>>")
		af.Prompt = strings.TrimSpace(p)
	}
	if mblock, ok := sections["META"]; ok {
		for _, line := range strings.Split(mblock, "\n") {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "route:"); ok {
				af.Route = strings.TrimSpace(after)
			}
			if after, ok := strings.CutPrefix(line, "provider:"); ok {
				af.Provider = strings.TrimSpace(after)
			}
			if after, ok := strings.CutPrefix(line, "reason:"); ok {
				af.Reason = strings.TrimSpace(after)
			}
		}
	}
	if hblock, ok := sections["HISTORY"]; ok {
		af.History = parseHistory(hblock)
	}
	return nil
}

func splitSections(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentSection string
	var currentLines []string

	flush := func() {
		if currentSection != "" {
			result[currentSection] = strings.Join(currentLines, "\n")
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flush()
			currentSection = trimmed[1 : len(trimmed)-1]
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush()
	return result
}

func parseQAs(block string) []QA {
	lines := strings.Split(block, "\n")
	qmap := make(map[int]QA)
	maxIdx := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, prefix := range []string{"Q", "A"} {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			rest := line[1:]
			colonIdx := strings.Index(rest, ":")
			if colonIdx < 0 {
				continue
			}
			idxStr := rest[:colonIdx]
			var idx int
			fmt.Sscanf(idxStr, "%d", &idx)
			if idx == 0 {
				continue
			}
			text := strings.TrimSpace(rest[colonIdx+1:])
			qa := qmap[idx]
			if prefix == "Q" {
				qa.Question = text
			} else {
				qa.Answer = text
			}
			qmap[idx] = qa
			if idx > maxIdx {
				maxIdx = idx
			}
		}
	}

	result := make([]QA, maxIdx)
	for i := 1; i <= maxIdx; i++ {
		result[i-1] = qmap[i]
	}
	return result
}

func parseHistory(block string) []Message {
	parts := strings.Split(block, "\n---\n")
	var msgs []Message
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lines := strings.SplitN(part, "\n", 2)
		if len(lines) < 2 {
			continue
		}
		role := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "role: "))
		content := strings.TrimSpace(lines[1])
		msgs = append(msgs, Message{Role: role, Content: content})
	}
	return msgs
}

func (af *AIFile) AddHistory(role, content string) error {
	af.History = append(af.History, Message{Role: role, Content: content})
	return af.Write()
}

func (af *AIFile) AnswersComplete() bool {
	if len(af.QAs) == 0 {
		return false
	}
	for _, qa := range af.QAs {
		if strings.TrimSpace(qa.Answer) == "" {
			return false
		}
	}
	return true
}

func (af *AIFile) OutputPath() string {
	base := strings.TrimSuffix(af.Path, ".ai")
	return base + ".output.md"
}

func HistoryPath(dir, taskName string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, taskName)
	ts := time.Now().Format("2006-01-02")
	return fmt.Sprintf("%s/%s-%s.ai", dir, ts, safe)
}
