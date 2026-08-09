package handler

import (
	"net/http"

	"github.com/quillit/contentengine/parse"
)

// tomExample is the worked example from docs/cli-spec.md §4 — used here
// purely to prove pkg/contentengine imports and runs correctly from
// inside this service, before any real content endpoints exist (#37+).
const tomExample = `---
name: Tom the Innkeeper
tags: [npc, waterdeep]
---

# Tom the Innkeeper

Tom runs the Gilded Goose inn. He rarely speaks of [[characters/npcs/mary|Mary]].

:::secret
Tom is secretly a spy for the Crimson Hand.
:::

:::card motivation
Wants to buy back his family farm.
:::

:::card description
Round-faced, ale-stained apron, booming laugh.
Spouse: [[characters/npcs/mary|Mary]]
:::
`

// Smoke proves the content-engine parser works from inside this service.
type Smoke struct{}

func NewSmoke() *Smoke {
	return &Smoke{}
}

// Parse runs the content-engine parser against a hardcoded example entry
// and returns the resulting structure.
func (s *Smoke) Parse(w http.ResponseWriter, r *http.Request) {
	entry, err := parse.Parse([]byte(tomExample))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entry)
}
