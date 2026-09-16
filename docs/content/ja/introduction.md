# はじめに

jmapc は JMAP のクライアントを生成するツールです。
そのため、このドキュメントはプロトコルの説明から始めます。
JMAP とは何か、そしてそのどの部分が、以降のページで扱うリクエストと生成コードの形を決めているかです。

## JMAP

[JMAP](https://jmap.io)（JSON Meta Application Protocol）は、ユーザのメール、連絡先、カレンダーを扱うための、IETF のオープンな標準です。
IMAP、CardDAV、CalDAV を1つのプロトコルで置き換えることを目指していて、HTTP の上で JSON をやりとりします。
中核は [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) が定め、ほかの仕様がそれぞれのデータモデルを加えます。
メールは [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621)、連絡先は [RFC 9610](https://www.rfc-editor.org/rfc/rfc9610) などです。
jmapc が対応している仕様は[対応範囲](coverage.md)にあります。

クライアントは、まず**セッション**を取得します。
セッションはサーバが公開する JSON の文書で、通常は `/.well-known/jmap` にあります。
サーバが対応するケイパビリティ、ユーザが到達できるアカウント、1つのリクエストが守るべき上限、そしてリクエストの送り先、blob のアップロードとダウンロード、プッシュの受信に使う URL が載っています。

**リクエスト**は、API の URL に送る JSON オブジェクトです。
メソッド呼び出しを並べたもので、サーバはそれを順に実行し、すべての答えを1つのレスポンスで返します。

```json
{
  "using": ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],
  "methodCalls": [
    ["Email/query", {"accountId": "a1", "filter": {"inMailbox": "mbx1"}, "limit": 10}, "search"],
    ["Email/get", {"accountId": "a1",
                   "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
                   "properties": ["subject", "from"]}, "fetch"]
  ]
}
```

呼び出しの名前は、`Email/query` のようにデータ型とメソッドの組です。
RFC 8620 は6つの標準メソッド `/get`、`/set`、`/changes`、`/query`、`/queryChanges`、`/copy` を定めていて、データ型がそれを提供する場合は、どの型でも同じ形になります。
`#` で始まる引数は**結果参照**です。
サーバは `Email/get` の id を、先に実行した `Email/query` の結果から取ります。
そのため2つの呼び出しは1往復で済み、途中の id はクライアントに戻ってきません。

データ型はそれぞれ**状態**（state）も報告します。
状態はレコードが変わるたびに変わる文字列です。
前回読んだときの状態を保持しているクライアントは、すべてを読み直すのではなく、`/changes` でそれ以降の変更を問い合わせます。
サーバは状態が変わったことをプッシュで知らせることができるので、クライアントは問い合わせるべき時点を知ることができます。

## jmapc の役割

jmapc の入力は、上のリクエストオブジェクトそのものです。
プログラムが送るリクエストを JMAP のままファイルに書くと、jmapc はそれぞれを仕様に照らして検証し、それを送信する関数を生成します。
関数の戻り値は、リクエストが要求したプロパティに合わせて型付けされたレスポンスです。
セッション、アカウント id、JMAP が報告するエラー、プッシュとページングのループは、生成されたコードと、それが呼ぶランタイムが扱います。

jmapc がリクエストを組み立てるライブラリではなくコンパイラである理由は[なぜ jmapc か](why.md)に、空のモジュールから呼び出しが動くまでの手順は[はじめてのクライアント](getting-started.md)にあります。

## JMAP についての資料

[jmap.io](https://jmap.io) は JMAP の公式サイトです。
IMAP との比較である [Why JMAP?](https://jmap.io/why-jmap/)、[仕様](https://jmap.io/spec/)、クライアントとサーバの開発者向けの[ガイド](https://jmap.io/guides/)、そして JMAP を実装した[サーバとクライアントの一覧](https://jmap.io/software/)があります。
