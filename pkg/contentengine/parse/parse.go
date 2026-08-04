package parse

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	secretOpenRe = regexp.MustCompile(`^:::secret\s*$`)
	cardOpenRe   = regexp.MustCompile(`^:::card\s+(\S+)\s*$`)
	fenceCloseRe = regexp.MustCompile(`^:::\s*$`)
	wikilinkRe   = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
)

// Parse turns raw entry .md bytes into a structured Entry. It performs no
// I/O of its own — data is the entire file's contents, already read by
// the caller.
func Parse(data []byte) (*Entry, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	fm, bodyLines := extractFrontmatter(lines)

	blocks, _, err := parseBlocks(bodyLines, 0, 0)
	if err != nil {
		return nil, err
	}

	return &Entry{Frontmatter: fm, Body: blocks}, nil
}

// extractFrontmatter strips a leading "---\n...\n---\n" YAML block, per
// CLI spec §4's example. If the file doesn't open with a "---" line, or
// the block is never closed, or the YAML itself doesn't parse, the whole
// file is treated as body with a zero-value Frontmatter — this package
// only fails loud on directive-block problems (see MalformedDirective),
// not on frontmatter typos.
func extractFrontmatter(lines []string) (Frontmatter, []string) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Frontmatter{}, lines
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" {
			continue
		}
		var fm Frontmatter
		raw := strings.Join(lines[1:i], "\n")
		if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
			return Frontmatter{}, lines
		}
		return fm, lines[i+1:]
	}
	return Frontmatter{}, lines
}

// parseBlocks parses lines[start:] into a flat list of top-level Blocks.
//
// containerOpenLine is 0 when parsing at the top level (not inside a
// :::secret/:::card block); otherwise it's the 1-indexed source line of
// the container's opening fence, used both to decide whether a bare :::
// line means "close this container" (containerOpenLine != 0) or "stray
// fence, error" (containerOpenLine == 0), and to report unclosed-block
// errors against the fence that was actually left open.
//
// next is the index of the line right after wherever parsing stopped: the
// line after a successfully-matched closing ::: fence when inside a
// container, or len(lines) at top level.
func parseBlocks(lines []string, start, containerOpenLine int) (blocks []Block, next int, err error) {
	inContainer := containerOpenLine != 0

	var proseBuf []string
	flushProse := func() {
		content := strings.TrimSpace(strings.Join(proseBuf, "\n"))
		proseBuf = nil
		if content == "" {
			return
		}
		blocks = append(blocks, Block{Kind: BlockProse, Content: content, Links: extractLinks(content)})
	}

	i := start
	for i < len(lines) {
		line := lines[i]

		switch {
		case cardOpenRe.MatchString(line):
			flushProse()
			facet := cardOpenRe.FindStringSubmatch(line)[1]
			nested, nextIdx, err := parseBlocks(lines, i+1, i+1)
			if err != nil {
				return nil, 0, err
			}
			content, childBlocks, links := splitDirect(nested)
			blocks = append(blocks, Block{Kind: BlockCard, Facet: facet, Content: content, Links: links, Blocks: childBlocks})
			i = nextIdx

		case secretOpenRe.MatchString(line):
			flushProse()
			nested, nextIdx, err := parseBlocks(lines, i+1, i+1)
			if err != nil {
				return nil, 0, err
			}
			content, childBlocks, links := splitDirect(nested)
			blocks = append(blocks, Block{Kind: BlockSecret, Content: content, Links: links, Blocks: childBlocks})
			i = nextIdx

		case fenceCloseRe.MatchString(line):
			if !inContainer {
				return nil, 0, MalformedDirective{Line: i + 1, Reason: "unexpected closing ::: with no open block"}
			}
			flushProse()
			return blocks, i + 1, nil

		default:
			proseBuf = append(proseBuf, line)
			i++
		}
	}

	if inContainer {
		return nil, 0, MalformedDirective{Line: containerOpenLine, Reason: "unclosed block (reached end of file looking for its closing :::)"}
	}
	flushProse()
	return blocks, i, nil
}

// splitDirect partitions a container's directly-nested blocks (as
// returned by parsing its inner lines) into its own direct prose content
// and links, plus the non-prose (Secret/Card) blocks that become its
// Blocks — so a container's Content never duplicates the raw source of
// blocks nested inside it.
func splitDirect(nested []Block) (content string, childBlocks []Block, links []Wikilink) {
	var proseParts []string
	for _, b := range nested {
		if b.Kind == BlockProse {
			proseParts = append(proseParts, b.Content)
			links = append(links, b.Links...)
			continue
		}
		childBlocks = append(childBlocks, b)
	}
	return strings.Join(proseParts, "\n\n"), childBlocks, links
}

// extractLinks finds every [[path]] / [[path|Label]] wikilink in content.
func extractLinks(content string) []Wikilink {
	matches := wikilinkRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	links := make([]Wikilink, 0, len(matches))
	for _, m := range matches {
		path := strings.TrimSpace(m[1])
		label := path
		if m[2] != "" {
			label = strings.TrimSpace(m[2])
		}
		links = append(links, Wikilink{Path: path, Label: label})
	}
	return links
}
