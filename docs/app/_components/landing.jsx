import { Children, isValidElement } from 'react'
import './landing.css'

export { Generated } from './generated.jsx'

import { LanguageIcon } from './language-icon.jsx'

export const Landing = ({ children }) => <div className="lp">{children}</div>

export const Hero = ({ title, children }) => (
  <header className="lp-hero">
    <h1 className="lp-title">
      {title.map(line => (
        <span key={line}>{line}</span>
      ))}
    </h1>
    {children}
  </header>
)

// MDX wraps the lede's text in a paragraph of its own, so the wrapper cannot be
// one too: a <p> inside a <p> is invalid HTML, and fails hydration.
export const Lede = ({ children }) => <div className="lp-lede">{children}</div>

export const Actions = ({ children }) => <p className="lp-actions">{children}</p>

// `primary` is the thing to do on the page. `more` is the way on to the page
// that says the rest of what the section says, which is not a step in the
// sequence and is not drawn as one.
export const Action = ({ href, primary, more, children }) => {
  const variant = primary ? ' lp-action-primary' : more ? ' lp-action-more' : ''
  return (
    <a className={`lp-action${variant}`} href={href}>
      {children}
    </a>
  )
}

// The input and the output of the compiler, with the command that turns one
// into the other sitting on the seam between them.
export const Compile = ({ command, children }) => {
  const [input, output] = Children.toArray(children).filter(isValidElement)
  return (
    <div className="lp-compile">
      <div className="lp-compile-in">{input}</div>
      <div className="lp-seam">
        <code>{command}</code>
      </div>
      {output}
    </div>
  )
}

// `half` marks a section set to half the page's measure rather than its whole
// width, though it still stands in the one column the page is.
export const Section = ({ kind, title, lede, invert, half, children }) => (
  <section
    className="lp-section"
    data-invert={invert ? '' : undefined}
    data-half={half ? '' : undefined}
  >
    {kind && <p className="lp-section-kind">{kind}</p>}
    <h2 className="lp-section-title">{title}</h2>
    <p className="lp-section-lede">{lede}</p>
    {children}
  </section>
)

export const Traits = ({ children }) => <dl className="lp-traits">{children}</dl>

// `lang` marks a row that is about one of the three languages: it takes the
// language's mark and its colour, the way the generated window does.
export const Trait = ({ title, lang, children }) => (
  <div className="lp-trait" data-lang={lang}>
    <dt>
      <LanguageIcon lang={lang} />
      {title}
    </dt>
    <dd>{children}</dd>
  </div>
)
