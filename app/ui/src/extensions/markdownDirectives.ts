// Shared markdown-it plugin backing SecretBlock, CardBlock and Wikilink's
// markdown parse/serialize (see those files' addStorage().markdown.parse.setup).
//
// markdown-it's own plugin-authoring types are awkward for this kind of
// low-level rule (StateBlock/StateInline/Token aren't cleanly exported for
// deep-import in every resolution mode), so this file works in `any` for
// markdown-it internals only — everything else in the extension files that
// import from here stays normally typed.

const SECRET_OPEN_RE = /^:::secret\s*$/
const CARD_OPEN_RE = /^:::card\s+(\S+)\s*$/
const FENCE_CLOSE_RE = /^:::\s*$/

function lineText(state: any, line: number): string {
  return state.src.slice(state.bMarks[line] + state.tShift[line], state.eMarks[line])
}

/**
 * Mirrors pkg/contentengine/parse.parseBlocks' line-based recursive grammar
 * exactly: the same three regexes, and "the first unmatched bare `:::`
 * closes the innermost still-open container" — so markdown produced and
 * consumed here round-trips through the Go CLI's parser identically.
 *
 * Deliberately NOT markdown-it-container's CommonMark-style fence-length
 * nesting (`::::` wrapping `:::`) — our directive blocks nest via pure
 * recursion with the *same* 3-colon fence throughout, which container
 * plugins don't model and the Go parser doesn't require.
 */
function directiveBlockRule(state: any, startLine: number, endLine: number, silent: boolean): boolean {
  const text = lineText(state, startLine)
  const isSecret = SECRET_OPEN_RE.test(text)
  const cardMatch = CARD_OPEN_RE.exec(text)
  if (!isSecret && !cardMatch) return false

  // Find the matching closing fence, honoring nested opens of either kind
  // (a nested :::secret or :::card increments depth; only a bare closing
  // ::: that brings depth back to 0 counts as *this* block's close).
  let depth = 1
  let closeLine = -1
  for (let line = startLine + 1; line < endLine; line++) {
    const t = lineText(state, line)
    if (SECRET_OPEN_RE.test(t) || CARD_OPEN_RE.test(t)) {
      depth++
    } else if (FENCE_CLOSE_RE.test(t)) {
      depth--
      if (depth === 0) {
        closeLine = line
        break
      }
    }
  }
  // Unclosed block: don't claim the line — let it fall through to a plain
  // paragraph rather than replicating the Go parser's hard MalformedDirective
  // failure mid-edit (that validation still happens server-side on save).
  if (closeLine === -1) return false

  if (silent) return true

  const facet = cardMatch ? cardMatch[1] : undefined
  const openType = cardMatch ? 'card_block_open' : 'secret_block_open'
  const closeType = cardMatch ? 'card_block_close' : 'secret_block_close'

  const openToken = state.push(openType, 'div', 1)
  openToken.map = [startLine, closeLine + 1]
  openToken.block = true
  if (facet) openToken.attrSet('data-facet', facet)

  state.md.block.tokenize(state, startLine + 1, closeLine)

  state.push(closeType, 'div', -1).block = true

  state.line = closeLine + 1
  return true
}

const WIKILINK_RE = /^\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/

function wikilinkInlineRule(state: any, silent: boolean): boolean {
  const src = state.src
  const pos = state.pos
  if (src.charCodeAt(pos) !== 0x5b /* [ */ || src.charCodeAt(pos + 1) !== 0x5b) return false
  const match = WIKILINK_RE.exec(src.slice(pos))
  if (!match) return false

  if (!silent) {
    const token = state.push('wikilink', '', 0)
    token.attrSet('data-path', match[1].trim())
    if (match[2] != null) token.attrSet('data-label', match[2].trim())
    token.markup = match[0]
  }
  state.pos += match[0].length
  return true
}

const registered = new WeakSet<object>()

/**
 * Idempotent: SecretBlock, CardBlock and Wikilink each call this from their
 * own `markdown.parse.setup(markdownit)` — only the first call for a given
 * markdown-it instance actually registers the rules.
 */
export function registerDirectiveRules(md: any): void {
  if (registered.has(md)) return
  registered.add(md)

  md.block.ruler.before('fence', 'directive_block', directiveBlockRule, {
    alt: ['paragraph', 'reference', 'blockquote', 'list'],
  })
  md.renderer.rules.secret_block_open = () => '<div data-type="secret-block">\n'
  md.renderer.rules.secret_block_close = () => '</div>\n'
  md.renderer.rules.card_block_open = (tokens: any[], idx: number) => {
    const facet = tokens[idx].attrGet('data-facet') || ''
    return `<div data-type="card-block" data-facet="${md.utils.escapeHtml(facet)}">\n`
  }
  md.renderer.rules.card_block_close = () => '</div>\n'

  md.inline.ruler.before('link', 'wikilink', wikilinkInlineRule)
  md.renderer.rules.wikilink = (tokens: any[], idx: number) => {
    const t = tokens[idx]
    const path = t.attrGet('data-path') || ''
    const label = t.attrGet('data-label')
    let attrs = `data-type="wikilink" data-path="${md.utils.escapeHtml(path)}"`
    if (label) attrs += ` data-label="${md.utils.escapeHtml(label)}"`
    return `<span ${attrs}></span>`
  }
}
