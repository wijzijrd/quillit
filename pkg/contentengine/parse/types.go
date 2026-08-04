// Package parse turns a single entry's raw .md bytes into a structured
// representation, per docs/cli-spec.md §4 and docs/web-refactor-spec.md
// §7.1. It has no filesystem, environment, or CLI coupling — callers pass
// bytes in and get a structured Entry out — so it can be imported by both
// the CLI (this repo's cli/ module, via the go.work workspace) and, later,
// the quillit/content service, without either depending on the other.
package parse

import "encoding/json"

// Entry is the parsed representation of one entry's .md file.
type Entry struct {
	Frontmatter Frontmatter
	Body        []Block
}

// Frontmatter is the YAML frontmatter block at the top of an entry's .md
// file, per CLI spec §4's example. Absent or malformed frontmatter parses
// as a zero-value Frontmatter rather than an error — the strict-parsing
// requirements this package must enforce (CLI spec golden rule 6) are
// about directive blocks, not the frontmatter's own YAML validity.
type Frontmatter struct {
	Name string   `yaml:"name"`
	Tags []string `yaml:"tags"`
}

// BlockKind distinguishes the three kinds of top-level (or nested) content
// a parsed entry is made of.
type BlockKind int

const (
	// BlockProse is ordinary markdown prose, not inside any directive.
	BlockProse BlockKind = iota
	// BlockSecret is a :::secret ... ::: block (CLI spec §4): DM-only,
	// stripped entirely from the player view.
	BlockSecret
	// BlockCard is a :::card <facet> ... ::: block (CLI spec §4).
	BlockCard
)

// String names a BlockKind ("prose", "secret", "card") — used by
// MarshalJSON so serialized entries (e.g. golden test fixtures) are
// readable without memorizing the iota values.
func (k BlockKind) String() string {
	switch k {
	case BlockSecret:
		return "secret"
	case BlockCard:
		return "card"
	default:
		return "prose"
	}
}

func (k BlockKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// Block is one piece of entry content, in source order. Blocks nest one
// level deep: a Secret block's Blocks holds any Card blocks found inside
// it (CLI spec §4: "Card blocks may sit inside secret blocks; such cards
// DM-only, still render in card view").
type Block struct {
	Kind BlockKind

	// Facet is set only on BlockCard blocks: the facet name from
	// `:::card <facet>`.
	Facet string

	// Content is this block's own raw markdown prose — for a container
	// block (Secret/Card), this is only the prose directly inside it,
	// with any nested directive blocks factored out into Blocks rather
	// than duplicated here.
	Content string

	// Blocks holds nested directive blocks found inside this block (see
	// BlockKind doc). Always empty for BlockProse blocks.
	Blocks []Block

	// Links are the wikilinks found in this block's own Content (not in
	// nested Blocks — those carry their own Links). This is the
	// "containing-block context" docs/cli-spec.md §3 "Link index" and
	// §4's card-expansion rules need: for a link inside a card block,
	// Facet on this Block tells the caller which facet it belongs to.
	Links []Wikilink
}

// Wikilink is one [[path]] or [[path|Label]] reference, per CLI spec §4
// "Links".
type Wikilink struct {
	// Path is the linked entry's project-root-relative path, e.g.
	// "characters/npcs/mary".
	Path string

	// Label is the display label. For the bare [[path]] form (no "|"),
	// Label equals Path.
	Label string
}
