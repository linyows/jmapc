import { Children, isValidElement } from 'react'
import './landing.css'

export { Generated } from './generated.jsx'

import { LanguageIcon } from './language-icon.jsx'
import { Wordmark } from './wordmark.jsx'

export const Landing = ({ children }) => <div className="lp">{children}</div>

export const Hero = ({ title, children }) => (
  <header className="lp-hero">
    <Wordmark className="lp-mark" />
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

export const Action = ({ href, primary, children }) => (
  <a className={primary ? 'lp-action lp-action-primary' : 'lp-action'} href={href}>
    {children}
  </a>
)

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

export const Section = ({ title, lede, invert, children }) => (
  <section className="lp-section" data-invert={invert ? '' : undefined}>
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
