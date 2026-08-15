export function stripGmMarks(html: string): string {
  if (!html) return ''
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('.annotation-mark--gm').forEach(el => {
    el.replaceWith(...el.childNodes)
  })
  return doc.body.innerHTML
}
