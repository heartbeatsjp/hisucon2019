# 【HISUCON2019】競技マニュアル

## ベンチマークについて

- ベンチマークの多重実行は不可です
- ベンチマーク実行ボタン押下後はキューに入ります。想定では 1 分 30 秒程度で結果反映されますが、同時実行数が多い場合には結果が反映されるまでに時間がかかります
- 結果反映が分かるように、ポータル画面は 10 秒ごとに画面を reload してます
- 初期スコアは平均 200 程度となってます

## 構成について

- docker-compose でWeb(Nginx)・アプリケーション(Gunicorn/Flask)・DB(MySQL)のコンテナを管理しています
- Nginxの設定ファイル・アプリのPythonファイル・MySQLの設定ファイルはホスト側で編集できるようコンテナ側とマウントさせています
- 詳細は /home/hisucon/docker-compose.yml をご確認ください

## アプリケーションについて

- サーバ内の作業ユーザは hisucon となります
- アプリケーションは /home/hisucon/app となります。
- 注意事項
    - 以下ディレクトリ配下、及びファイルは初期化用です。絶対に編集しないでください。
        - /home/hisucon/app/static/original_icons/
        - /home/hisucon/Docker/mysql/sqls/initialize.sql
        - /home/hisucon/Docker/mysql/accesslog.txt
        - /home/hisucon/Docker/mysql/bulletins.txt
        - /home/hisucon/Docker/mysql/bulletins_star.txt
        - /home/hisucon/Docker/mysql/comments.txt
        - /home/hisucon/Docker/mysql/comments_star.txt
        - /home/hisucon/Docker/mysql/users.txx
- Dockerfileを編集したときのdocker-composeの操作
    - コンテナを停止し、 up で作成したコンテナ・ネットワーク・ボリューム・イメージを削除

        ```
        $ sudo docker-compose down --rmi all
        ```
    - コンテナをバックグラウンドで起動し、実行

        ```
        $ sudo docker-compose up -d
        ```
- アプリ再起動
    ```
    $ sudo docker-compose restart app
    ```
- Nginx再起動
    ```
    $ sudo docker-compose restart web
    ```
- MySQL再起動
    ```
    $ sudo docker-compose restart db
    ```

### MySQL

- ユーザ
    - ユーザ名
        - root
    - パスワード
        - secret
- ログイン

    ```
    $ sudo docker exec -it <コンテナ名> bash
    # mysql -u root -p
    ```
