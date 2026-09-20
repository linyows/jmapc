//読み手がしていることで分ける。jmapcを知る、リクエストを書く、
//そこから生成されたコードを呼ぶ。
export default {
  index: {
    title: 'ホーム',
    display: 'hidden',
    theme: {
      layout: 'full',
      sidebar: false,
      toc: false,
      breadcrumb: false,
      pagination: false,
      timestamp: false,
      copyPage: false
    }
  },
  '-- overview': { type: 'separator', title: '概要' },
  introduction: 'はじめに',
  why: 'なぜjmapcか',
  'getting-started': 'はじめてのクライアント',
  '-- requests': { type: 'separator', title: 'リクエスト' },
  requests: 'リクエストの書き方',
  properties: 'プロパティ集合',
  verification: '検証',
  run: 'リクエストを送る',
  '-- generated': { type: 'separator', title: '生成されたコード' },
  'generated-code': '生成される名前',
  errors: 'エラー',
  push: 'プッシュ',
  paging: 'ページング',
  blobs: 'Blob',
  client: 'クライアントの設定',
  testing: 'テスト',
  languages: 'RustとTypeScript',
  '-- reference': { type: 'separator', title: 'リファレンス' },
  cli: 'jmapcコマンド',
  extensions: 'ベンダ拡張',
  coverage: '対応範囲',
  contributing: 'jmapcの開発'
}
