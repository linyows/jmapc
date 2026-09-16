import { IBM_Plex_Mono, IBM_Plex_Sans, Noto_Sans_JP } from 'next/font/google'
import { Footer, Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import { Wordmark } from '../_components/wordmark.jsx'
import { LocaleSwitch } from '../_components/locale-switch.jsx'
import { localeOf, unprefix } from '../_locale.js'
import 'nextra-theme-docs/style.css'
import './styles.css'

const SITE = 'https://jmapc.linyo.ws'

// IBM Plex: drawn for a company whose business was compilers, and a family
// whose monospace and proportional faces are the same voice. It carries no
// Japanese, so Noto Sans JP stands behind it in the stack: Latin is set in
// Plex either way, and the kana and kanji fall through.
const sans = IBM_Plex_Sans({
  subsets: ['latin'],
  // 600 is what the theme sets its headings in; 700 is what <strong> asks for,
  // and without it the browser draws a bold that was never drawn.
  weight: ['400', '500', '600', '700'],
  variable: '--font-sans',
  display: 'swap'
})

const mono = IBM_Plex_Mono({
  subsets: ['latin'],
  weight: ['400', '500', '700'],
  variable: '--font-mono',
  display: 'swap'
})

const japanese = Noto_Sans_JP({
  subsets: ['latin'],
  weight: ['400', '500', '700'],
  variable: '--font-jp',
  display: 'swap'
})

const DICTIONARY = {
  en: {
    description:
      'jmapc is a JMAP compiler: you write the request, it writes the client.',
    editPage: 'Edit this page on GitHub',
    lastUpdated: 'Last updated on',
    backToTop: 'Scroll to top',
    light: 'Light',
    dark: 'Dark',
    system: 'System'
  },
  ja: {
    description:
      'jmapc は JMAP のコンパイラです。リクエストを書くと、クライアントが書かれます。',
    editPage: 'GitHub でこのページを編集',
    lastUpdated: '最終更新',
    backToTop: '先頭に戻る',
    light: 'ライト',
    dark: 'ダーク',
    system: 'システム'
  }
}

export async function generateMetadata({ params }) {
  const lang = localeOf((await params).mdxPath)
  return {
    title: { absolute: '', template: '%s | jmapc' },
    description: DICTIONARY[lang].description,
    metadataBase: new URL(SITE),
    icons: { icon: '/logo.svg' },
    openGraph: { siteName: 'jmapc', type: 'website' }
  }
}

const Logo = () => <Wordmark className="jmapc-logo" />

// The year the first commit landed, and the year this build was made. The site
// is static, so "now" is the last time it was published.
const SINCE = 2026

const Copyright = () => {
  const now = new Date().getFullYear()
  return (
    <span>
      © {now > SINCE ? `${SINCE}-${now}` : SINCE}{' '}
      <a
        href="https://github.com/linyows"
        target="_blank"
        rel="noreferrer"
      >
        linyows
      </a>
    </span>
  )
}

export default async function RootLayout({ children, params }) {
  const lang = localeOf((await params).mdxPath)
  const dictionary = DICTIONARY[lang]
  const pageMap = await getPageMap(`/${lang}`)

  return (
    <html
      lang={lang}
      dir="ltr"
      className={`${sans.variable} ${mono.variable} ${japanese.variable}`}
      suppressHydrationWarning
    >
      {/* Cream and oxblood, one the ground of the other. The primary is the
          palette's vermilion, taken down in the light and up in the dark so
          that a link on either ground is still a link anyone can read. */}
      <Head
        backgroundColor={{ light: '#f8e9e0', dark: '#5d181f' }}
        color={{
          hue: 8,
          saturation: { light: 75, dark: 82 },
          lightness: { light: 42, dark: 68 }
        }}
      />
      <body>
        <Layout
          navbar={
            <Navbar
              logo={<Logo />}
              logoLink={lang === 'ja' ? '/ja' : '/'}
              projectLink="https://github.com/linyows/jmapc"
            >
              <LocaleSwitch labels={{ en: 'English', ja: '日本語' }} />
            </Navbar>
          }
          footer={
            <Footer>
              <Copyright />
            </Footer>
          }
          docsRepositoryBase="https://github.com/linyows/jmapc/blob/main/docs"
          sidebar={{ defaultMenuCollapseLevel: 1 }}
          toc={{ backToTop: dictionary.backToTop }}
          editLink={dictionary.editPage}
          themeSwitch={{
            light: dictionary.light,
            dark: dictionary.dark,
            system: dictionary.system
          }}
          pageMap={lang === 'ja' ? pageMap : unprefix(pageMap)}
        >
          {children}
        </Layout>
      </body>
    </html>
  )
}
