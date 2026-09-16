# The documentation and its site

`content/{en,ja}/*.md` is the documentation. It is also, unchanged, what Nextra
reads: the rest of this directory is the site that publishes it at
<https://jmapc.linyo.ws>. Nothing is generated from the Markdown, so a page is
edited in one place and appears in both.

## Running it

```sh
npm install
npm run dev    # http://localhost:3000
npm run build  # the whole site, written to out/
```

## The two languages

English is the site's default and is served without a prefix: `/requests` is
English and `/ja/requests` is Japanese. Nextra expects every route to carry its
locale, and offers a middleware to put it there, which a static export has no
room for. So `app/_locale.js` tells the two apart from the first segment of the
route instead, and strips the locale back out of the page map for English;
`app/_components/locale-switch.jsx` is the link between the two, in place of
the theme's own switch, which assumes every locale has a segment.

## The landing page

`content/{en,ja}/index.mdx` is the one page that is not documentation. It is
written with the components in `app/_components/`, and it asks for the theme's
full-width layout with no sidebar and no table of contents in `_meta.js`. Its
code blocks are ordinary fences, so the request and the call it generates are
highlighted the way they are everywhere else on the site.

The generated half of the hero holds one panel per language, stacked, and
`generated.jsx` compiles the request into the next one every few seconds: the
new code wipes in from the side the seam is on, over the code it replaces. It
stops as soon as a reader hovers the panel or picks a language, and it does not
start at all under `prefers-reduced-motion`.

## Writing

`content/en/requests.md` and `content/ja/requests.md` are the two translations
of one page; the navbar switches between them, and `_meta.js` in each locale
gives the order of the sidebar and the title of each entry. A new page is a
file in both locales and a line in both `_meta.js`.

A link to another document is written the way the file system has it —
`[Push](push.md)` — so that it works when the file is read on GitHub. Nextra
drops the extension and leaves the link relative, and the routes carry no
trailing slash, so it resolves against the sibling it names. A link out of the
documentation, to the changelog or to the examples, is written as a URL: there
is no relative path that is right in both places.

## The patch

`scripts/patch-nextra-theme.mjs` runs on install. nextra-theme-docs takes
`children` out of its props before validating them against a schema that still
requires `children`, which fails every page. The fix is merged upstream and not
yet released: https://github.com/shuding/nextra/pull/4990. The patch does
nothing once a release carries it.
