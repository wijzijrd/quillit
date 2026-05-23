/**
 * Import Sol'Yn Obsidian vault into Quillit.
 *
 * Prerequisites:
 *   - quillit-svc backend running on http://localhost:3000
 *   - Logged-in session cookie (run once in browser first, or set QUILLIT_COOKIE below)
 *   - 7 categories already created in Admin: Characters, Deities, Location, Realm, Faction, Lore, Plot
 *
 * Usage:
 *   node scripts/import-solyn.mjs
 */

import { readdir, readFile, stat } from 'fs/promises'
import { join, relative, basename, extname, dirname } from 'path'
import matter from 'gray-matter'
import { marked } from 'marked'

// ─── Config ──────────────────────────────────────────────────────────────────

const VAULT_ROOT = "/Users/tauriqbehardien/Documents/repositories/solyn/Sol'Yn"
const API_BASE   = 'http://localhost:3000/api'

// Paste your session cookie here if needed (copy from browser DevTools → Application → Cookies)
const QUILLIT_COOKIE = process.env.QUILLIT_COOKIE ?? ''

// ─── Category mapping ────────────────────────────────────────────────────────

const DIR_TO_CATEGORY = {
  "Game Master Only/Sol'Yn/Characters":         'Characters',
  "Game Master Only/General Lore/Pantheon of Gods/The Primes": 'Deities',
  "Game Master Only/General Lore/Pantheon of Gods": 'Deities',
  "Game Master Only/General Lore":              'Lore',
  "Game Master Only/Sol'Yn/Cities":             'Location',
  "Game Master Only/Sol'Yn/Isles":              'Location',
  "Game Master Only/Realms":                    'Location',
  "Game Master Only/Qadahn'ihā":                'Location',
  "Loose plots":                                'Plot',
}
const TAG_TO_CATEGORY = { god: 'Deities', realm: 'Location', city: 'Location' }

// Files to skip entirely
const SKIP_FILES = new Set([
  "Untitled.md",                                // app spec doc, not a note
  "Game Master Only/General Lore/Untitled.md",  // empty
  "Game Master Only/Sol'Yn/Sol'Yn.md",          // empty
  "King Hrunn XLII.md",                         // empty stub
])

// ─── Crew members extracted from Captain Crockett ────────────────────────────

const CREW = [
  {
    name: '"Lucky" Rennick Bale',
    race: 'Human', cls: 'Rogue', age: 34,
    body: `<p><strong>Race:</strong> Human | <strong>Class:</strong> Rogue | <strong>Age:</strong> 34</p>
<p>A cardsharp and gambler who joined Crockett's third expedition. Returned from that voyage with an inexplicable terror of doorways — he will only enter rooms sideways, never facing forward.</p>
<blockquote><p>"It ain't the rooms what move… it's the hallways."</p></blockquote>`,
  },
  {
    name: 'Sister Ysolde Mire',
    race: 'Half-Elf', cls: 'Cleric', age: 52,
    body: `<p><strong>Race:</strong> Half-Elf | <strong>Class:</strong> Cleric | <strong>Age:</strong> 52</p>
<p>A former priestess who joined the fifth expedition. Returned with three fingers missing and an obsessive compulsion to sketch the same tower from slightly different angles. The drawings cover the walls of her room at the Anchor &amp; Wren.</p>
<blockquote><p>"Something in there knew my prayers before I spoke them."</p></blockquote>`,
  },
  {
    name: 'Brakka Stonejaw',
    race: 'Dwarf', cls: 'Fighter', age: 78,
    body: `<p><strong>Race:</strong> Dwarf | <strong>Class:</strong> Fighter | <strong>Age:</strong> 78</p>
<p>A mercenary who complained constantly that the castle had "bad stone." Took to licking the walls. Disappeared entirely during the fourth expedition; his axe was found embedded in a ceiling beam thirty feet overhead, with no explanation of how it got there.</p>`,
  },
  {
    name: 'Pellwick "Pelly" Tumb',
    race: 'Halfling', cls: 'Bard', age: 26,
    body: `<p><strong>Race:</strong> Halfling | <strong>Class:</strong> Bard | <strong>Age:</strong> 26</p>
<p>A tavern musician who wrote "The Ballad of James the Bold" before the expedition. After returning he lost the ability to hear music properly — every melody sounds a half-tone wrong, chord resolutions feel unfinished. He still follows Crockett, hoping one more voyage will let him hear the ending he wrote.</p>`,
  },
  {
    name: 'Vaelis Thorne',
    race: 'Elf', cls: 'Ranger', age: 143,
    body: `<p><strong>Race:</strong> Elf | <strong>Class:</strong> Ranger | <strong>Age:</strong> 143</p>
<p>A northern forest hunter who claimed to have dreamed of the castle before Crockett ever hired him. Returned from the second expedition with a detailed sketchbook of impossible architecture — staircases with no floors, rooms nested inside other rooms at wrong scales.</p>
<p>Six weeks later he took his own life. On the final page of the sketchbook he wrote:</p>
<blockquote><p>"The map is incomplete because the castle is alive."</p></blockquote>`,
  },
  {
    name: 'Mother Garru',
    race: 'Orc', cls: 'Druid', age: 61,
    body: `<p><strong>Race:</strong> Orc | <strong>Class:</strong> Druid | <strong>Age:</strong> 61</p>
<p>A herbalist and swamp mystic who claimed the sea had been screaming Crockett's name for years before she found him. Vanished on the sixth expedition. Three days later, the crew heard her voice singing below deck — calm, tuneless — with no body to find.</p>`,
  },
  {
    name: 'Edwin Pike',
    race: 'Human', cls: 'Wizard', age: 22,
    body: `<p><strong>Race:</strong> Human | <strong>Class:</strong> Wizard | <strong>Age:</strong> 22</p>
<p>An apprentice sent by Lucien Vale to document the castle's contents. Returned unable to remember his own face. Looking in a mirror caused him to panic. The official cause of death listed "brain fever." He jumped from a bell tower.</p>`,
  },
  {
    name: 'Korga Redwake',
    race: 'Dragonborn', cls: 'Barbarian', age: 41,
    body: `<p><strong>Race:</strong> Dragonborn | <strong>Class:</strong> Barbarian | <strong>Age:</strong> 41</p>
<p>A former pit fighter who by his own account had never felt fear. Entered the castle's throne room alone and emerged crying. He never explained what he saw. Some weeks later he climbed over the ship's railing into freezing water. His body was never recovered.</p>`,
  },
  {
    name: 'Nim of the Reeds',
    race: 'Goblin', cls: 'Monk', age: null,
    body: `<p><strong>Race:</strong> Goblin | <strong>Class:</strong> Monk | <strong>Age:</strong> Unknown</p>
<p>Nobody remembered hiring Nim — he simply appeared on the ship's manifest after the second voyage. He could navigate portions of the castle instinctively, as if he had been there before. Disappeared on the third expedition. For weeks afterward, tiny wet footprints appeared in Crockett's locked cabin each morning.</p>`,
  },
  {
    name: 'Helene Voss',
    race: 'Tiefling', cls: 'Warlock', age: 39,
    body: `<p><strong>Race:</strong> Tiefling | <strong>Class:</strong> Warlock | <strong>Age:</strong> 39</p>
<p>A scholar of forbidden history, obsessed with Aramand. She recognised symbols no one had shown her and finished sentences before the speaker had started them. Vanished on the fifth expedition.</p>
<blockquote><p>"The castle remembers every visitor. Even the ones it lets leave."</p></blockquote>
<p>Crockett refuses to speak her name.</p>`,
  },
]

// ─── Helpers ─────────────────────────────────────────────────────────────────

function slugify(name) {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

async function apiFetch(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers ?? {}) }
  if (QUILLIT_COOKIE) headers['Cookie'] = QUILLIT_COOKIE
  const res = await fetch(API_BASE + path, {
    ...options,
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined,
    credentials: 'include',
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`${options.method ?? 'GET'} ${path} → ${res.status}: ${text}`)
  }
  return res.json()
}

function determineCategory(relPath, frontmatterTags) {
  const dir = dirname(relPath).replace(/\\/g, '/')
  // Longest matching prefix wins
  const match = Object.keys(DIR_TO_CATEGORY)
    .filter(prefix => dir === prefix || dir.startsWith(prefix + '/'))
    .sort((a, b) => b.length - a.length)[0]
  if (match) return DIR_TO_CATEGORY[match]
  // Frontmatter tag override
  for (const tag of (frontmatterTags ?? [])) {
    if (TAG_TO_CATEGORY[tag]) return TAG_TO_CATEGORY[tag]
  }
  // Root level
  return 'Lore'
}

function convertBody(markdown, idMap) {
  // Wikilinks: [[target|alias]] and [[target]]
  const withLinks = markdown
    .replace(/\[\[([^\]|]+)\|([^\]]+)\]\]/g, (_, target, alias) => {
      const id = idMap[slugify(target.trim())]
      return id
        ? `<span class="entry-mention" data-entry-id="${id}">${alias.trim()}</span>`
        : `<em>${alias.trim()}</em>`
    })
    .replace(/\[\[([^\]]+)\]\]/g, (_, target) => {
      const id = idMap[slugify(target.trim())]
      return id
        ? `<span class="entry-mention" data-entry-id="${id}">${target.trim()}</span>`
        : `<em>${target.trim()}</em>`
    })
  return marked.parse(withLinks)
}

async function collectFiles(dir) {
  const results = []
  const items = await readdir(dir)
  for (const item of items) {
    if (item.startsWith('.')) continue
    const full = join(dir, item)
    const s = await stat(full)
    if (s.isDirectory()) {
      results.push(...(await collectFiles(full)))
    } else if (extname(item) === '.md') {
      results.push(full)
    }
  }
  return results
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  console.log('Collecting files…')
  const allFiles = await collectFiles(VAULT_ROOT)

  // Resolve relative paths and filter skips
  const files = allFiles
    .map(f => ({ full: f, rel: relative(VAULT_ROOT, f) }))
    .filter(({ rel }) => !SKIP_FILES.has(rel))

  console.log(`Found ${files.length} files to import (after skipping ${allFiles.length - files.length})`)

  // ── Parse frontmatter ──
  const parsed = []
  for (const { full, rel } of files) {
    const raw = await readFile(full, 'utf-8')
    const { data: fm, content: body } = matter(raw)
    // Skip truly empty notes
    if (!body.trim() && !fm.aliases?.length) continue
    const title = (fm.aliases?.[0] ?? basename(rel, '.md')).trim()
    const tags = Array.isArray(fm.tags) ? fm.tags : (fm.tags ? [fm.tags] : [])
    const category = determineCategory(rel, tags)
    parsed.push({ full, rel, title, tags, category, body })
  }

  // ── Pass 1: create entries, record ID map ──
  console.log('\nPass 1: creating entries…')
  const idMap = {}  // slugified title → entry ID

  // Create crew first so they can be linked in Crockett body
  for (const crew of CREW) {
    const tags = ['NPC', 'Former Crew', crew.race, crew.cls]
    const entry = await apiFetch('/entries', {
      method: 'POST',
      body: { title: crew.name, category: 'Characters', tags },
    })
    idMap[slugify(crew.name)] = entry.id
    console.log(`  [crew] ${crew.name} → ${entry.id}`)
  }

  for (const file of parsed) {
    // Skip if a crew member by same name was already created
    if (idMap[slugify(file.title)]) continue
    const entry = await apiFetch('/entries', {
      method: 'POST',
      body: { title: file.title, category: file.category, tags: file.tags },
    })
    idMap[slugify(file.title)] = entry.id
    console.log(`  ${file.title} (${file.category}) → ${entry.id}`)
  }

  // ── Pass 2: update bodies with resolved wikilinks ──
  console.log('\nPass 2: updating bodies…')

  // Crew bodies
  for (const crew of CREW) {
    const id = idMap[slugify(crew.name)]
    await apiFetch(`/entries/${id}`, { method: 'PATCH', body: { body: crew.body } })
  }

  // File bodies
  for (const file of parsed) {
    const id = idMap[slugify(file.title)]
    if (!id) continue
    const html = convertBody(file.body, idMap)
    await apiFetch(`/entries/${id}`, { method: 'PATCH', body: { body: html } })
  }

  // ── Pass 3: link crew to Crockett via relations ──
  console.log('\nPass 3: creating crew relations…')
  const crockettId = idMap[slugify('Captain James Crockett')]
  if (crockettId) {
    for (const crew of CREW) {
      const crewId = idMap[slugify(crew.name)]
      if (!crewId) continue
      try {
        await apiFetch(`/entries/${crewId}/relations`, {
          method: 'POST',
          body: { toId: crockettId, label: 'former crew of' },
        })
        console.log(`  linked ${crew.name} → Crockett`)
      } catch (e) {
        console.warn(`  Could not link ${crew.name}: ${e.message}`)
      }
    }
  }

  // ── Summary ──
  console.log(`\n✓ Import complete.`)
  console.log(`  Notes created: ${Object.keys(idMap).length}`)
  console.log(`  Crew members:  ${CREW.length}`)
  console.log(`  Files:         ${parsed.length}`)
}

main().catch(err => { console.error(err); process.exit(1) })
