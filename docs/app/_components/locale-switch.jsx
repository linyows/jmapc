'use client'

import { usePathname } from 'next/navigation'

// The theme's own switch swaps the first segment of the path for the locale,
// which only works when every locale has one. English has none here, so the
// switch maps the same page between its two paths itself, and loads the other
// one whole so that the root layout is drawn again in that language.
export const LocaleSwitch = ({ labels }) => {
  const pathname = usePathname()
  const japanese = pathname === '/ja' || pathname.startsWith('/ja/')
  const paths = {
    en: japanese ? pathname.replace(/^\/ja(?=\/|$)/, '') || '/' : pathname,
    ja: japanese ? pathname : `/ja${pathname === '/' ? '' : pathname}`
  }

  return (
    <select
      className="jmapc-locale"
      aria-label="Language"
      value={japanese ? 'ja' : 'en'}
      onChange={event => window.location.assign(paths[event.target.value])}
    >
      {Object.entries(labels).map(([lang, label]) => (
        <option key={lang} value={lang} lang={lang}>
          {label}
        </option>
      ))}
    </select>
  )
}
