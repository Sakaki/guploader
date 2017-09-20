# Google Photos uploader written in Go

組み込み向けに書いたGoogle Photosアップローダーです。

というかPQI Air Cardで動かすために作りました。

撮影した画像を直接Google Photosにアップロードしてくれます。

## 使い方

以下の作業が必要ですが、ドキュメント整備中です。

 * settings_dummy.jsonをsettings.jsonにコピーして設定を記述
 * go getでライブラリを取得
   * golang.org/x/oauth2
   * golang.org/x/oauth2/google
 * ホストPCで一度実行して、トークン(token_cached.json)を取得
 * 組み込み機器向けにコンパイル
 * バイナリ、token_cached.json、settings.jsonを組み込み機器にコピー

実行するとGoogle Photosから画像一覧を取得し、対象のディレクトリからそれに含まれない画像をアップロードします。

アップロードが終了すると1秒後にリトライするので、バイナリを何度も実行する必要はありません。

また、画像一覧の取得は初回のみなので以降のアップロードはスムーズに行えます。

さらに、実行する組み込み機器上でのファイルの読み書きがありませんので、電源断が予想される状況でも利用できます。

PQI Air Card上で利用するには、何らかの方法で時刻を合わせる必要があります。
