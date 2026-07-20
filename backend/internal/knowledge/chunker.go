package knowledge

import (
	"strings"
	"unicode/utf8"
)

const (
	defaultMaxChunkRunes = 800
	defaultOverlapRunes  = 100
	minChunkRunes        = 80
)

// Chunk splits document text into overlapping segments suitable for embedding.
// Splits prefer paragraph boundaries; oversized paragraphs are hard-split.
func Chunk(content string, maxRunes, overlapRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = defaultMaxChunkRunes
	}
	if overlapRunes <= 0 {
		overlapRunes = defaultOverlapRunes
	}
	if overlapRunes >= maxRunes {
		overlapRunes = maxRunes / 5
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if utf8.RuneCountInString(content) <= maxRunes {
		return []string{content}
	}

	paragraphs := splitParagraphs(content)
	var chunks []string
	var buf strings.Builder

	flush := func() {
		s := strings.TrimSpace(buf.String())
		if utf8.RuneCountInString(s) >= minChunkRunes {
			chunks = append(chunks, s)
		}
		buf.Reset()
	}

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if utf8.RuneCountInString(p) > maxRunes {
			flush()
			for _, part := range hardSplit(p, maxRunes, overlapRunes) {
				if utf8.RuneCountInString(part) >= minChunkRunes {
					chunks = append(chunks, part)
				}
			}
			continue
		}

		candidate := p
		if buf.Len() > 0 {
			candidate = buf.String() + "\n\n" + p
		}
		if utf8.RuneCountInString(candidate) <= maxRunes {
			buf.Reset()
			buf.WriteString(candidate)
			continue
		}
		flush()
		buf.WriteString(p)
	}
	flush()
	return chunks
}

func splitParagraphs(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	parts := strings.Split(normalized, "\n\n")
	if len(parts) == 1 {
		return parts
	}
	return parts
}

func hardSplit(text string, maxRunes, overlapRunes int) []string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	var out []string
	step := maxRunes - overlapRunes
	if step <= 0 {
		step = maxRunes
	}
	for start := 0; start < len(runes); start += step {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return out
}
