import { cloneElement, isValidElement } from 'react'
import { useMDXComponents as getDocsMDXComponents } from 'nextra-theme-docs'

const docsComponents = getDocsMDXComponents()

// The Japanese pages are written one sentence to a line. A line break inside a
// paragraph is a space once the page is rendered, and Japanese sets no space
// beside anything -- not between two of its own characters, and not beside
// Latin or code either -- so a break with Japanese on either side of it is
// closed up here. A break between two Latin words is the space English wants,
// and the English pages, which wrap at a column rather than at a sentence, are
// left as they are by the same rule.
//
// This is done to the rendered children rather than in a remark plugin because
// the compiler's options have to be serialisable and a plugin is a function,
// which Turbopack refuses to pass to the loader.
const CJK =
  '\\u3000-\\u303f\\u3040-\\u30ff\\u3400-\\u4dbf\\u4e00-\\u9fff\\uff00-\\uff60\\uffe0-\\uffe6'

// Either the character before the break is Japanese, and it is kept while the
// break goes, or the one after it is, and the break goes on its own. Looking
// at the character after rather than consuming it lets a run of one-sentence
// lines close up in a single pass.
const BREAK = new RegExp(`([${CJK}])\\n|\\n(?=[${CJK}])`, 'g')
const closeUp = (match, japanese) => japanese || ''

const join = node => {
  if (typeof node === 'string') return node.replace(BREAK, closeUp)
  if (Array.isArray(node)) return node.map(join)
  if (!isValidElement(node)) return node
  // Code is quoted, not set: its line breaks are part of what is quoted.
  if (node.type === docsComponents.code || node.type === docsComponents.pre) {
    return node
  }
  const { children } = node.props
  return children === undefined ? node : cloneElement(node, undefined, join(children))
}

// Every element that carries a paragraph's worth of text. The inline elements
// inside them are reached through their children.
const PROSE = ['p', 'li', 'td', 'th', 'blockquote', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6']

const joinedComponents = Object.fromEntries(
  PROSE.map(tag => {
    const Component = docsComponents[tag] ?? tag
    const Joined = ({ children, ...props }) => (
      <Component {...props}>{join(children)}</Component>
    )
    return [tag, Joined]
  })
)

export const useMDXComponents = components => ({
  ...docsComponents,
  ...joinedComponents,
  ...components
})
