# jmapcの開発

jmapc自体の作業は、ほとんどが2つのコマンドで足ります。
1つはテストを走らせ、もう1つはカタログから生成されるものを作り直します。
CIが最初に実行するのもこの2つです。

```
go test ./...        # サーバなしで動くものすべて
go generate ./...    # ランタイムの型と、全言語のサンプルクライアントを再生成する
```

サンプルは言語ごとに3度生成され、`example/client`、`example/rust/src/jmap_client`、`example/ts`に出力されます。
残る2つがコンパイルできるかどうかはGoのテストでは分からないので、CIはRustに`cargo fmt --check`と`cargo test`を、TypeScriptに`tsc --strict`を実行します。
どちらにも生成コードと並ぶ手書きの検査があり、スタブを相手にランタイムを動かします。
ヘッダが送られること、認証がそれに優先すること、セッションがキャッシュされること、そして200を返しながら拒否を含む`/set`がやはりエラーになることを確かめます。

スキーマも同じやり方で、同じ理由から検証します。
バリデータがexampleのリクエストを受け入れ、スキーマが捕まえると主張する間違いを拒むかどうかは、Goのテストには言えません。
`example/schema/check.mjs`が、その時点のカタログから書き出したスキーマに対してバリデータを実行します。

ここでジェネレータをソースから実行しているのは、このリポジトリがジェネレータの居場所だからです。

ここまでのテストは、生成コードをスタブに対して動かします。
スタブは、テストが期待するとおりにサーバとして応答するものです。
`e2e/`は、実際のサーバであるStalwartをコンテナで動かし、それに対して実行します。

```
e2e/run.sh
```

docker、Go、[probe](https://github.com/linyows/probe)がPATHにある必要があります。
コンテナを起動し、Stalwartをbootstrap modeから抜けさせ、アカウントを2つ作ってから、`e2e/workflow.yml`のシナリオを実行します。
各シナリオは、jmapcを通して何かをしたあと、jmapcを通らない経路でそれを確かめます。
jmapcが読んだsessionを直接取得したものと比べ、SMTPで配送したメールを生成クライアントが見つけられるかを確かめます。
jmapcで取り込んだメールのフラグをIMAPで読み、IMAPで付けたフラグをjmapcで読みます。
jmapcでアップロードしたblobを、直接ダウンロードしたものと比べます。
jmapc側を受け持つのは`e2e/driver`で、`e2e/requests`から生成したクライアントを呼び、結果をJSONで出力します。

準備では、サーバの設定とドライバーのビルドを並行して行います。

```mermaid
flowchart LR
    subgraph provision["Provision the server"]
        provision_step0["Wait for bootstrap mode"]
        provision_step1["Complete bootstrap"]
        provision_step2["Restart out of bootstrap mode"]
        provision_step3["Wait for the server"]
        provision_step4["Find the domain bootstrap created"]
        provision_step5["Create alice and bob"]
    end
    subgraph job_1["Build the driver"]
        job_1_step0["go build"]
    end
```

シナリオは互いに独立していて、並行して実行されます。

```mermaid
flowchart LR
    subgraph job_0["The session as jmapc reads it"]
        job_0_step0["Fetch the session directly"]
        job_0_step1["Fetch it through jmapc"]
    end
    subgraph job_1["Mail delivered over SMTP, found through jmapc"]
        job_1_step0["Deliver to alice on port 25"]
        job_1_step1["Find it through the generated client"]
        job_1_step2["Read alice's mail account"]
        job_1_step3["Find the same email directly"]
    end
    subgraph job_2["Mail imported through jmapc, read over IMAP"]
        job_2_step0["Import a flagged message through the generated client"]
        job_2_step1["Find it in the inbox over IMAP, flagged and unread"]
        job_2_step2["Mark it read over IMAP"]
        job_2_step3["See it read through the generated client"]
    end
    subgraph job_3["A blob uploaded through jmapc, downloaded directly"]
        job_3_step0["Upload and download it through jmapc"]
        job_3_step1["Download the whole of it directly"]
        job_3_step2["Ask for the same range directly"]
    end
```

2つ目の図は`probe dag --mermaid e2e/vars.yml,e2e/workflow.yml`で、1つ目は`e2e/setup.yml`について同じコマンドで出力したものです。
ジョブやステップを足したら、出力し直してください。

`E2E_KEEP=1`を指定しない限り、終わるとコンテナは削除されます。
Stalwartのリリースは`e2e/run.sh`で固定しています。
Stalwartはマイナーバージョンの間で設定の形を変えるので、新しいバージョンへの移行は、そこを意図して書き換える変更として行います。

ランタイムの型とサンプルのクライアントはコミットされていて、それらをカタログが今生成する結果と比較するテストがあります。
データモデルを変えたのに再生成し忘れると、見逃されるのではなくビルドが失敗します。
CIでは同じ検証に加えて、gofmt、go vet、govulncheckを実行します。

リリースは、変更がmainに入ったあとにタグをpushして作ります。
リリースノートは[CHANGELOG.md](https://github.com/linyows/jmapc/blob/main/CHANGELOG.md)のそのタグの節で、何が変わったかを、利用側にとっての意味でグループ分けし、破壊的変更を先頭に置いて書きます。
節はタグより先に書いてください。
対応する節がないタグはリリースを失敗させます。空の本文で公開するよりよいからです。

各項目は、どれだけ長くなっても1行で書きます。
GitHubはリリース本文を、渡された改行のまま表示します。
80桁で折り返した段落は、リリースページでも80桁で改行された段落になります。
