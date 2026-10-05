# jmapcコマンド

`jmapc`には6つのサブコマンドがあります。

| サブコマンド | |
|---|---|
| `jmapc generate` | リクエストを検証し、クライアントを生成します。 |
| `jmapc validate` | リクエストを検証するだけで、何も書き出しません。`-session`を付けると稼働中のサーバに対しても検証します。[検証](verification.md)を参照してください。 |
| `jmapc run <request>` | リクエストを1つサーバに送り、レスポンスを表示します。[リクエストを送る](run.md)を参照してください。 |
| `jmapc schema` | リクエストファイルを記述するJSON Schemaを、エディタのために書き出します。[エディタ対応](verification.md#エディタ対応)を参照してください。 |
| `jmapc guide` | リクエストの書き方を、コーディングエージェントのために表示します。[コーディングエージェント](#コーディングエージェント)を参照してください。 |
| `jmapc version` | バージョンを表示します。 |

`generate`と`validate`が取るフラグは`jmapc -h`で、`run`、`schema`、`guide`が取るフラグは`jmapc run -h`、`jmapc schema -h`、`jmapc guide -h`で表示できます。

## コーディングエージェント

`jmapc guide`は、コーディングエージェントがリクエストを追加・変更する前に読むためのガイドを表示します。
内容は、ファイルの形式、jmapcが読むメンバー、パラメータ、リクエストを書いてからクライアントを生成するまでの手順、`validate`が拒否する間違いとその直し方です。
ガイドはコマンドに組み込まれているので、表示したjmapcの版のとおりの内容になります。
`jmapc guide mistakes`のように指定すると1つの節だけを表示し、節の一覧は`jmapc guide -h`で表示できます。

`jmapc guide -install`は、リクエストを書く前にガイドを読み、書いた後に`jmapc validate`を実行するようエージェントに指示するskillを書き出します。
書き出し先は、Claude Code向けが`.claude/skills/jmapc/SKILL.md`、Codex向けが`.agents/skills/jmapc/SKILL.md`です。
`-for claude`または`-for codex`を付けると片方だけを書き出します。
skillはガイドを指し示すだけなので、jmapcを上げても書き直す必要はありません。
また、`-install`は自分が書いたのではない`SKILL.md`を上書きしません。

## 生成物が最新かを確かめる

生成されたクライアントはリポジトリにコミットするものなので、生成元のリクエストから遅れることがあります。
リクエストを直して生成し直さなかった場合や、jmapcを上げて生成し直さなかった場合です。
`generate -check`は、ディスクにあるものと、いま生成したら書かれるものとを比べます。
何も書かず、両者が違えば失敗します。

```
jmapc generate -check
client/listinboxemails_gen.go: out of date
client/oldreport_gen.go: generated from a request that is no longer there
jmapc: 2 files are out of date; run jmapc generate
```

報告するのは3つです。

- リクエストがいま生成するものと違うファイル
- まだ生成されていないファイル
- jmapcが以前書いたが、どのリクエストももう生成しないファイル（リクエストファイルを消したときに残るもの）

`jmapc generate`を実行すれば3つとも直ります。前の2つは書き直し、3つ目は削除します。

生成されたことを示す先頭行を持たないファイルは手で書かれたものとみなし、`-check`は報告せず、生成し直しても削除しません。

検査する生成と同じ引数を渡してください。
リクエストの置かれていたパスは、そこから生成されたファイルに書き込まれるので、`-requests`に別のパスを渡して検査すると、すべてのファイルが古いと報告されます。

ワークフローでは次のように書きます。

```yaml
- run: go tool jmapc generate -check
```

## 設定

どのサブコマンドも、実行したディレクトリに`jmapc.json`があればそこから設定を読みます。
別のファイルを読ませるには`-config`で指定します。
フラグは同じ名前の設定より優先されます。

```json
{
  "requests": "requests",
  "out": "internal/client",
  "package": "client",
  "schemas": ["schema/notes.json"]
}
```

| 設定 | フラグ | 既定値 | |
|---|---|---|---|
| `requests` | `-requests` | `requests` | リクエストファイルを置くディレクトリ |
| `out` | `-out` | `client` | 生成したクライアントを書き出すディレクトリ |
| `lang` | `-lang` | `go` | 生成する言語。`go`、`rust`、`typescript`のいずれか |
| `package` | `-package` | `out`の最後の要素 | 生成するGoパッケージの名前 |
| `schemas` | `-schema` | | ベンダ拡張を記述したスキーマファイル。[ベンダ拡張](extensions.md)を参照してください。フラグは繰り返し指定でき、設定に列挙したファイルに追加されます |

`requests`ディレクトリにはリクエストを1ファイルに1つずつ置きます。
リクエストが指定するプロパティ集合を宣言した`properties.json`を置くこともできます。
[プロパティ集合](properties.md)を参照してください。
