# HISUCON 2019

社内ISUCON(HISUCON 2019)を開催したので、使用したコンテンツを公開します。

# ライセンス

MIT ライセンスにて公開しております。
ただ、Web アプリケーションにて利用しております画像につきましては下記に従ってご利用ください。

- [FLAT ICON DESIGN](http://flat-icon-design.com/)
- [ライセンス](http://flat-icon-design.com/?page_id=41)

# 使用手順

## 画像データの展開

### アプリケーション

```
$ cd webapp/ansible/roles/webapp/files/app/static
$ unzip -o icons.zip -d icons 
$ unzip -o icons.zip -d original_icons
```

### ベンチマーカー

```
$ cd bench/ansible/roles/bench/files/bench/data/images
$ unzip -o bench_image.zip
```

## DB初期データの展開

```
$ cd webapp/ansible/roles/webapp/files/Docker/mysql
$ unzip -o data.zip
```

## ローカル環境で動かす

GoとDockerとDocker Composeを予めインストールしておいて下さい。

### アプリケーション

```
$ cd webapp/ansible/roles/webapp/files
$ docker-compose up -d
$ docker-compose ps
      Name                     Command               State                 Ports
----------------------------------------------------------------------------------------------
hisucon2019-app     gunicorn views:app -b 0.0. ...   Up      0.0.0.0:8000->5000/tcp
hisucon2019-db      docker-entrypoint.sh mysqld      Up      0.0.0.0:3306->3306/tcp, 33060/tcp
hisucon2019-nginx   nginx -g daemon off;             Up      0.0.0.0:80->80/tcp
```
コンテナのスタータスが`Up`になっていれば、 http://localhost にアクセスするとアプリケーションが表示されます。

### ベンチマーカー

```
$ cd bench/ansible/roles/bench/files/bench
$ gb vendor restore
$ make
$ ./bin/bench -remotes=localhost
```

## Alibaba Cloudで動かす

Ansible を予めインストールしておいてください。

### インスタンス作成

アプリケーション用とベンチマーカー用で2台作成する

- 価格モデル
  - 従量課金
- リージョン
  - 東京
- インスタンスタイプ
  - `ecs.c5.large`もしくは`ecs.sn1ne.large` (2vCPU 4GiB)
- イメージ
  - CentOS 7.6
- ストレージ
  - Ultraクラウドディスク 30GiB
- ネットワーク課金タイプ
  - パブリックIPの割当に ✔ する
  - 帯域幅を100Mbpsにする
- セキュリティグループ
  - アプリケーション
    - IPv4にてSSH(22), HTTP(80)を許可
  - ベンチマーカー
    - IPv4にてSSH(22), HTTP(80), Custom TCP(3000)を許可
- ログイン認証
  - キーペア

### アプリケーション

```
$ cd webapp/ansible/
$ vim inventry
[target]
インスタンスのグローバルIPを記入

$ ansible-playbook -i inventry site.yml -c ssh -u root --private-key=ダウンロードした秘密鍵
```

正常に終了すれば、 http://IPアドレス/ にアクセスするとアプリケーションが表示されます。

### ベンチマーカー

```
$ cd bench/ansible/
$ vim inventry
[target]
インスタンスのグローバルIPを記入

$ ansible-playbook -i inventry site.yml -c ssh -u root --private-key=ダウンロードした秘密鍵
```

正常に終了すれば、下記のGrafana、ポータルサイトが表示されます。

- 起動
  ```
  systemctl start hisucon2019-bench.service
  ```
- 停止
  ```
  systemctl stop hisucon2019-bench.service
  ```

### Grafana

スコア遷移のグラフを表示します。

- http://IPアドレス:3000/ にアクセス
  - User: admin
  - Password: admin
- データソースはMySQLを選択
  - Host: localhost:3306
  - Database: hisucon2019_portal
  - User: root
  - Password: DQCjL6Hl9HOY1Jnf#
- Queryは下記を登録
  ```
  SELECT
  UNIX_TIMESTAMP(created_at) as time_sec,
  CAST(result->"$.score" as SIGNED) as value,
  team as metric
  FROM bench 
  WHERE $__timeFilter(created_at)
  ORDER BY created_at ASC
  ```
- 起動
  ```
  systemctl start grafana-server.service
  ```
- 停止
  ```
  systemctl stop grafana-server.service
  ```

### ポータルサイト

- http://IPアドレス/top/[team]/[private-ipaddress] にアクセス
  - [team]
    - チーム名を入力
  - [private-ipaddress]
    - アプリケーション用インスタンスのプライベートIPアドレスを入力
    - `/srv/webapp/main.go`の L40 にてプライベートIP制限をかけてますので、こちらの修正もあわせてお願いします。
- 起動
  ```
  systemctl start hisucon2019-portal.service
  ```
- 停止
  ```
  systemctl stop hisucon2019-portal.service
  ```
