# はじめに

jmapcはJMAPのクライアントを生成するツールです。
そのため、このドキュメントはプロトコルの説明から始めます。
JMAPとは何か、そしてそのどの部分が、以降のページで扱うリクエストと生成コードの形を決めているかです。

## JMAP

[JMAP](https://jmap.io)（JSON Meta Application Protocol）は、ユーザのメール、連絡先、カレンダーを扱うための、IETFのオープンな標準です。
IMAP、CardDAV、CalDAVを1つのプロトコルで置き換えることを目指していて、HTTPの上でJSONをやりとりします。
中核は[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620)が定め、ほかの仕様がそれぞれのデータモデルを加えます。
メールは[RFC 8621](https://www.rfc-editor.org/rfc/rfc8621)、連絡先は[RFC 9610](https://www.rfc-editor.org/rfc/rfc9610)などです。
jmapcが対応している仕様は[対応範囲](coverage.md)にあります。

クライアントは、まず**セッション**を取得します。
セッションはサーバが公開するJSONの文書で、通常は`/.well-known/jmap`にあります。
サーバが対応するケイパビリティ、ユーザが到達できるアカウント、1つのリクエストが守るべき上限、そしてリクエストの送り先、blobのアップロードとダウンロード、プッシュの受信に使うURLが載っています。

**リクエスト**は、APIのURLに送るJSONオブジェクトです。
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

呼び出しの名前は、`Email/query`のようにデータ型とメソッドの組です。
RFC 8620は6つの標準メソッド`/get`、`/set`、`/changes`、`/query`、`/queryChanges`、`/copy`を定めていて、データ型がそれを提供する場合は、どの型でも同じ形になります。
`#`で始まる引数は**結果参照**です。
サーバは`Email/get`のidを、先に実行した`Email/query`の結果から取ります。
そのため2つの呼び出しは1往復で済み、途中のidはクライアントに戻ってきません。

データ型はそれぞれ**状態**（state）も報告します。
状態はレコードが変わるたびに変わる文字列です。
前回読んだときの状態を保持しているクライアントは、すべてを読み直すのではなく、`/changes`でそれ以降の変更を問い合わせます。
サーバは状態が変わったことをプッシュで知らせることができるので、クライアントは問い合わせるべき時点を知ることができます。

## jmapcの役割

jmapcの入力は、上のリクエストオブジェクトそのものです。
プログラムが送るリクエストをJMAPのままファイルに書くと、jmapcはそれぞれを仕様に照らして検証し、それを送信する関数を生成します。
関数の戻り値は、リクエストが要求したプロパティに合わせて型付けされたレスポンスです。
セッション、アカウントid、JMAPが報告するエラー、プッシュとページングのループは、生成されたコードと、それが呼ぶランタイムが扱います。

jmapcがリクエストを組み立てるライブラリではなくコンパイラである理由は[なぜjmapcか](why.md)に、空のモジュールから呼び出しが動くまでの手順は[はじめてのクライアント](getting-started.md)にあります。

## JMAPについての資料

[jmap.io](https://jmap.io)はJMAPの公式サイトです。
IMAPとの比較である[Why JMAP?](https://jmap.io/why-jmap/)、[仕様](https://jmap.io/spec/)、クライアントとサーバの開発者向けの[ガイド](https://jmap.io/guides/)、そしてJMAPを実装した[サーバとクライアントの一覧](https://jmap.io/software/)があります。
