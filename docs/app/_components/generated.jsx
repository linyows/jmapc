'use client'

import { Children, Fragment, isValidElement, useEffect, useState } from 'react'

// Long enough to read the call through and look back at the request it came
// from, short enough that a reader who stays sees all three.
const DWELL = 7600

const reducedMotion = () =>
  typeof matchMedia === 'function' &&
  matchMedia('(prefers-reduced-motion: reduce)').matches

// The same request, compiled again into another language. The panels are
// stacked, so the box is as tall as the tallest of them and nothing below it
// moves when one replaces another.
export const Generated = ({ label, languages, children }) => {
  const panels = Children.toArray(children).filter(isValidElement)
  const [shown, setShown] = useState(0)
  const [leaving, setLeaving] = useState(null)
  // A reader who picks a language has said which one they want to read.
  const [picked, setPicked] = useState(false)
  const [reading, setReading] = useState(false)

  useEffect(() => {
    if (picked || reading || reducedMotion()) return
    const id = setTimeout(() => {
      setLeaving(shown)
      setShown((shown + 1) % panels.length)
    }, DWELL)
    return () => clearTimeout(id)
  }, [shown, picked, reading, panels.length])

  const pick = next => {
    setPicked(true)
    if (next === shown) return
    setLeaving(shown)
    setShown(next)
  }

  // Two grid items rather than one box, so that the panel lines up with the
  // request it was compiled from and the languages sit below both.
  return (
    <Fragment>
      <div
        className="lp-generated-stack"
        onMouseEnter={() => setReading(true)}
        onMouseLeave={() => setReading(false)}
        onFocus={() => setReading(true)}
        onBlur={() => setReading(false)}
      >
        {panels.map((panel, i) => (
          <div
            key={languages[i]}
            className="lp-generated-panel"
            data-lang={languages[i].toLowerCase()}
            data-state={
              i === shown ? 'shown' : i === leaving ? 'leaving' : 'hidden'
            }
            aria-hidden={i !== shown}
          >
            {panel}
          </div>
        ))}
      </div>
      <div className="lp-generated-pick" role="group" aria-label={label}>
        {languages.map((name, i) => (
          <button
            key={name}
            type="button"
            className="lp-generated-lang"
            data-lang={name.toLowerCase()}
            aria-pressed={i === shown}
            onClick={() => pick(i)}
          >
            {name}
          </button>
        ))}
      </div>
    </Fragment>
  )
}
