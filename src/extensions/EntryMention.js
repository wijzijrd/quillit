import Mention from '@tiptap/extension-mention'

export function createEntryMention(getEntries) {
  return Mention.configure({
    HTMLAttributes: { class: 'entry-mention' },

    renderLabel({ node }) {
      return `[[${node.attrs.label ?? node.attrs.id}]]`
    },

    suggestion: {
      char: '@',

      items({ query }) {
        const q = query.toLowerCase()
        return getEntries()
          .filter(e => !q || e.title.toLowerCase().includes(q))
          .slice(0, 8)
      },

      render() {
        let el = null
        let selectedIndex = 0
        let currentProps = null

        function build() {
          el = document.createElement('div')
          el.className = 'entry-suggestion-popup'
          document.body.appendChild(el)
        }

        function paint(props) {
          if (!el) return
          const rect = props.clientRect?.()
          if (rect) {
            el.style.cssText = [
              'position:fixed',
              `top:${rect.bottom + 4}px`,
              `left:${rect.left}px`,
              'z-index:1000',
              'display:block',
            ].join(';')
          }
          if (!props.items.length) {
            el.style.display = 'none'
            return
          }
          // Build list with safe DOM methods — no innerHTML with user content
          el.textContent = ''
          props.items.forEach((item, i) => {
            const row = document.createElement('div')
            row.className = `esug-item${i === selectedIndex ? ' esug-item--focused' : ''}`
            row.dataset.idx = String(i)

            const cat = document.createElement('span')
            cat.className = 'esug-cat'
            cat.textContent = item.category

            const title = document.createElement('span')
            title.className = 'esug-title'
            title.textContent = item.title

            row.appendChild(cat)
            row.appendChild(title)
            row.addEventListener('mousedown', e => {
              e.preventDefault()
              props.command({ id: item.id, label: item.title })
            })
            el.appendChild(row)
          })
        }

        return {
          onStart(props) {
            selectedIndex = 0
            currentProps = props
            build()
            paint(props)
          },
          onUpdate(props) {
            selectedIndex = 0
            currentProps = props
            paint(props)
          },
          onKeyDown({ event }) {
            if (!currentProps?.items.length) return false
            const len = currentProps.items.length
            if (event.key === 'ArrowDown') {
              selectedIndex = (selectedIndex + 1) % len
              paint(currentProps)
              return true
            }
            if (event.key === 'ArrowUp') {
              selectedIndex = (selectedIndex - 1 + len) % len
              paint(currentProps)
              return true
            }
            if (event.key === 'Enter') {
              const item = currentProps.items[selectedIndex]
              currentProps.command({ id: item.id, label: item.title })
              return true
            }
            return false
          },
          onExit() {
            el?.remove()
            el = null
            currentProps = null
          },
        }
      },
    },
  })
}
