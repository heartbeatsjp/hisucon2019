import os
import hashlib
import shutil
from PIL import Image
from flask import Flask, render_template, request, redirect, url_for, session, abort, flash, jsonify
from flask_wtf.csrf import CSRFProtect
from flask_paginate import Pagination, get_page_parameter
from flask_sqlalchemy import SQLAlchemy

from models import Bulletins, Users, Comments, Accesslog, Bulletins_Star, Comments_Star


app = Flask(__name__)
csrf = CSRFProtect(app)
app.secret_key = 'tJMrMFgzMH7CPjZkta'
url = 'mysql+mysqldb://root:secret@hisucon2019-db/bb_app?charset=utf8mb4'
app.config['SQLALCHEMY_DATABASE_URI'] = url
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = True
app.config['DEBUG'] = True
db = SQLAlchemy(app)


def get_session_name():
    if 'user_id' in session:
        name = session['username']
    else:
        name = ''
    return name


def check_user(user_id):
    if 'user_id' in session:
        if session['user_id'] != user_id:
            abort(403)
    else:
        abort(403)


class Validate():
    def __init__(self):
        self.username = 'ユーザ名'
        self.nickname = 'ニックネーム'

    def check_empty(self, *args):
        for value in args:
            if not value:
                return '空白の項目があります必ず記入して下さい'
            return None

    def check_length(self, **kwargs):
        error = []
        for key, value in kwargs.items():
            if key == 'password':
                if len(value) < 8:
                    error.append('パスワードは8文字以上にして下さい')
            else:
                if key == 'username':
                    name = self.username
                    num = 16
                if key == 'nickname':
                    name = self.nickname
                    num = 32

                if len(value) > num:
                    error.append('%sは%s文字以内にして下さい' % (name, num))
        return error

    def check_exist(self, **kwargs):
        error = []
        for key, value in kwargs.items():
            if key == 'username':
                string = self.username
            if key == 'nickname':
                string = self.nickname

            res = db.session.execute("SELECT * FROM users WHERE %s = '%s'" % (key, value))
            row = res.fetchone()
            if row:
                error.append('既に存在している%sです' % string)
        return error

    def check_diff(self, passwd, confirm):
        if passwd != confirm:
            return '「パスワード」と「パスワード確認用」に差異があります'
        return None


validate = Validate()


@app.route('/reset', methods=['GET'])
def reset():
    db.session.query(Comments).delete()
    db.session.query(Users).delete()
    db.session.query(Bulletins).delete()
    db.session.query(Accesslog).delete()
    db.session.query(Bulletins_Star).delete()
    db.session.query(Comments_Star).delete()
    db.session.execute("ALTER TABLE `comments` auto_increment = 1;")
    db.session.execute("ALTER TABLE `users` auto_increment = 1;")
    db.session.execute("ALTER TABLE `bulletins` auto_increment = 1;")
    db.session.execute("ALTER TABLE `accesslog` auto_increment = 1;")
    db.session.execute("ALTER TABLE `bulletins_star` auto_increment = 1;")
    db.session.execute("ALTER TABLE `comments_star` auto_increment = 1;")
    db.session.execute("LOAD DATA LOCAL INFILE '/tmp/accesslog.txt' INTO TABLE accesslog;")
    db.session.execute("LOAD DATA LOCAL INFILE '/tmp/bulletins.txt' INTO TABLE bulletins;")
    db.session.execute("LOAD DATA LOCAL INFILE '/tmp/comments.txt' INTO TABLE comments;")
    db.session.execute("LOAD DATA LOCAL INFILE '/tmp/users.txt' INTO TABLE users;")
    db.session.execute("LOAD DATA LOCAL INFILE '/tmp/bulletins_star.txt' INTO TABLE bulletins_star;")
    db.session.execute("LOAD DATA LOCAL INFILE '/tmp/comments_star.txt' INTO TABLE comments_star")
    db.session.commit()

    icons_path = "./static/icons"
    original_icons_path = "./static/original_icons"
    if os.path.isdir(icons_path):
        shutil.rmtree(icons_path)

    if os.path.isdir(original_icons_path):
        shutil.copytree(original_icons_path, icons_path)

    return ('', 204)


@app.route('/login', methods=['GET', 'POST'])
def login():
    if request.method == 'GET':
        return render_template('login.html')

    name = request.form['name']
    res = db.session.execute("SELECT * FROM users WHERE username = :name", {"name": name})
    row = res.fetchone()
    password = hashlib.sha256(request.form['password'].encode('utf-8')).hexdigest()

    if not row or password != row['password']:
        flash('ログインに失敗しました', 'error_msg')
        return render_template('login.html'), 403

    session['username'] = name
    session['user_id'] = row['id']
    return redirect(url_for('index'))


@app.route('/logout')
def logout():
    session.clear()
    return redirect(url_for('index'))


@app.route('/')
def root():
    return redirect(url_for('index'))


@app.route('/bulletins', methods=["GET"])
def index():
    contents = []
    bulletins = db.session.execute("SELECT * FROM bulletins ORDER BY modified DESC")
    for bulletin in bulletins:
        res = db.session.execute("SELECT nickname FROM users WHERE id = (SELECT user_id FROM bulletins WHERE id = :b_id)", {"b_id": bulletin.id})
        row = res.fetchone()
        content = {'nickname': row['nickname'], 'title': bulletin.title, 'id': bulletin.id, 'modified': bulletin.modified}
        contents.append(content)

    res = db.session.execute("SELECT COUNT(*) FROM bulletins")
    total = res.fetchone()[0]
    page = request.args.get(get_page_parameter(), type=int, default=1)
    select_contents = contents[(page - 1) * 10: page * 10]
    pagination = Pagination(page=page, per_page=10, total=total, search=False, record_name='entries')

    rank_dict = {}
    for num in range(1, (total + 1)):
        res = db.session.execute("SELECT COUNT(*) FROM accesslog WHERE bulletin_id = :num", {"num": num})
        row = res.fetchone()[0]
        rank_dict[num] = row
    rank_sort_list = sorted(rank_dict.items(), reverse=True, key=lambda x: x[1])
    ranking_contents = []
    for bulletin_id, count in rank_sort_list[0:10]:
        res = db.session.execute("SELECT title FROM bulletins WHERE id = :b_id", {"b_id": bulletin_id})
        title = res.fetchone()[0]
        ranking_content = {'bulletin_id': bulletin_id, 'title': title, 'count': count}
        ranking_contents.append(ranking_content)

    return render_template('index.html', contents=select_contents, pagination=pagination, ranking=ranking_contents, name=get_session_name())


@app.route('/bulletins/add', methods=["GET", "POST"])
def add():
    if 'user_id' not in session:
        return redirect(url_for('login'))

    if request.method == "GET":
        bulletin = {}
        return render_template('add.html', bulletin=bulletin, name=get_session_name())

    empty_error = validate.check_empty(request.form['title'], request.form['body'])

    if empty_error:
        flash(empty_error, 'error_msg')
        bulletin = request.form.to_dict()
        return render_template('add.html', bulletin=bulletin, name=get_session_name())

    db.session.execute("INSERT INTO bulletins (user_id, title, body, created, modified) VALUES (:s_u_id, :b_title, :b_body, NOW(), NOW())", {"s_u_id": session['user_id'], "b_title": request.form['title'], "b_body": request.form['body']})
    db.session.commit()
    res = db.session.execute("SELECT * FROM bulletins WHERE user_id = :s_u_id AND title = :b_title AND body = :b_body ORDER BY id DESC LIMIT 1", {"s_u_id": session['user_id'], "b_title": request.form['title'], "b_body": request.form['body']})
    bulletin_id = res.fetchone()[0]

    return redirect(url_for('view', bulletin_id=bulletin_id))


@app.route('/bulletins/delete/<int:bulletin_id>', methods=["GET", "POST"])
def delete(bulletin_id):
    if request.method == "GET":
        abort(404)

    res = db.session.execute("SELECT user_id FROM bulletins WHERE id = :b_id", {"b_id": bulletin_id})
    user_id = res.fetchone()[0]

    check_user(user_id)

    db.session.execute("DELETE FROM bulletins where id = :b_id", {"b_id": bulletin_id})
    db.session.execute("DELETE FROM accesslog where bulletin_id = :b_id", {"b_id": bulletin_id})
    db.session.commit()

    return redirect(url_for('index'))


@app.route('/bulletins/edit/<int:bulletin_id>', methods=["GET", "POST"])
def edit(bulletin_id):
    res = db.session.execute("SELECT * FROM bulletins WHERE id = :b_id", {"b_id": bulletin_id})
    row = res.fetchone()
    user_id = row['user_id']

    check_user(user_id)

    if request.method == "GET":
        bulletin = row
        return render_template('edit.html', bulletin=bulletin, name=get_session_name())

    empty_error = validate.check_empty(request.form['title'], request.form['body'])

    if empty_error:
        flash(empty_error, 'error_msg')
        bulletin = request.form.to_dict()
        return render_template('edit.html', bulletin=bulletin, name=get_session_name())

    db.session.execute("UPDATE bulletins SET title=:b_title, body=:b_body, modified=NOW() WHERE id=:b_id",
                       {"b_title": request.form['title'], "b_body": request.form['body'], "b_id": bulletin_id})
    db.session.commit()
    return redirect(url_for('view', bulletin_id=bulletin_id))


@app.route('/bulletins/view/<int:bulletin_id>')
def view(bulletin_id):
    comments = []
    bulletins = db.session.execute("SELECT * FROM bulletins WHERE id = :b_id", {"b_id": bulletin_id})
    bulletin = bulletins.fetchone()

    if not bulletin:
        abort(404)

    res = db.session.execute("SELECT * FROM users WHERE id = (SELECT user_id FROM bulletins WHERE id = :b_id)", {"b_id": bulletin_id})
    nickname = res.fetchone()
    _res = db.session.execute("SELECT * FROM comments WHERE bulletin_id = :b_id ORDER BY created", {"b_id": bulletin_id})
    if _res:
        for comment in _res.fetchall():
            row = db.session.execute("SELECT * FROM users WHERE id = :u_id", {"u_id": comment['user_id']})
            com_data = row.fetchone()
            res = db.session.execute("SELECT COUNT(*) FROM comments_star WHERE comment_id = :c_id", {"c_id": comment['id']})
            star = res.fetchone()[0]
            data = {'id': comment['id'], 'icon': com_data['icon'], 'user_id': com_data['id'], 'nickname': com_data['nickname'], 'comment': comment['body'], 'created': comment['created'], 'star_count': star}
            comments.append(data)

    res = db.session.execute("SELECT COUNT(*) FROM bulletins_star WHERE bulletin_id = :b_id", {"b_id": bulletin_id})
    star_count = res.fetchone()[0]

    db.session.execute("INSERT INTO accesslog (bulletin_id, access) VALUES (:b_id, NOW())", {"b_id": bulletin_id})
    db.session.commit()

    if 'user_id' in session:
        user_id = session['user_id']
    else:
        user_id = ""

    return render_template('view.html', bulletin=bulletin, name=get_session_name(), nickname=nickname, comments=comments, user_id=user_id, star_count=star_count)


@app.route('/star', methods=['POST'])
def add_star():
    if 'bulletin_id' in request.form:
        db.session.execute("INSERT INTO bulletins_star (bulletin_id, access) VALUES (:b_id, NOW())", {"b_id": request.form['bulletin_id']})
        db.session.commit()

        res = db.session.execute("SELECT COUNT(*) FROM bulletins_star WHERE bulletin_id = :b_id", {"b_id": request.form['bulletin_id']})
        star_count = res.fetchone()[0]
    elif 'comment_id' in request.form:
        db.session.execute("INSERT INTO comments_star (comment_id, access) VALUES (:c_id, NOW())", {"c_id": request.form['comment_id']})
        db.session.commit()

        res = db.session.execute("SELECT COUNT(*) FROM comments_star WHERE comment_id = :c_id", {"c_id": request.form['comment_id']})
        star_count = res.fetchone()[0]
    else:
        abort(403)

    return jsonify({'output': star_count})


@app.route('/bulletins/add_comment', methods=['GET', 'POST'])
def add_comment():
    if request.method == "GET":
        abort(404)

    if 'user_id' not in session:
        abort(403)

    empty_error = validate.check_empty(request.form['comment'])

    if empty_error:
        flash(empty_error, 'error_msg')
        return redirect(url_for('view', bulletin_id=request.form['bulletin_id']))

    db.session.execute("INSERT INTO comments (bulletin_id, user_id, body, created, modified) VALUES (:b_id, :u_id, :c_id, NOW(), NOW())",
                       {"b_id": request.form['bulletin_id'], "u_id": session['user_id'], "c_id": request.form['comment']})
    db.session.commit()

    return redirect(url_for('view', bulletin_id=request.form['bulletin_id']))


@app.route('/bulletins/search', methods=["GET"])
def search(contents=[]):
    bulletins = db.session.execute("SELECT * FROM bulletins ORDER BY modified DESC")
    word_included_title = ''
    user_id = None

    if request.args.get("title") is not None:
        word_included_title = request.args.get("title")
    elif ('username' in session) and (request.args.get("my_bulletins") == session['username']):
        res = db.session.execute("SELECT id FROM users WHERE username = :m_bulletins", {"m_bulletins": request.args.get("my_bulletins")})
        user_id = res.fetchone()[0]

    target_cnt = 0
    contents = []
    for bulletin in bulletins:
        if (request.args.get("title") is not None) and (word_included_title in bulletin.title):
            res = db.session.execute("SELECT nickname FROM users WHERE id = (SELECT user_id FROM bulletins WHERE id = :b_id)", {"b_id": bulletin.id})
            row = res.fetchone()
            content = {'nickname': row['nickname'], 'title': bulletin.title, 'id': bulletin.id, 'modified': bulletin.modified}
            contents.append(content)
            target_cnt += 1
        elif (request.args.get("my_bulletins") is not None) and (user_id == bulletin.user_id):
            res = db.session.execute("SELECT nickname FROM users WHERE id = (SELECT user_id FROM bulletins WHERE id = :b_id)", {"b_id": bulletin.id})
            row = res.fetchone()
            content = {'nickname': row['nickname'], 'title': bulletin.title, 'id': bulletin.id, 'modified': bulletin.modified}
            contents.append(content)
            target_cnt += 1

    res = db.session.execute("SELECT COUNT(*) FROM bulletins")
    total_bulletins = target_cnt
    page = request.args.get(get_page_parameter(), type=int, default=1)
    select_contents = contents[(page - 1) * 10: page * 10]
    pagination = Pagination(page=page, per_page=10, total=total_bulletins, search=False, record_name='entries')

    return render_template('search.html', contents=select_contents, pagination=pagination, name=get_session_name())


@app.route('/comment/edit/<int:comment_id>', methods=["GET", "POST"])
def comment_edit(comment_id):
    res = db.session.execute("SELECT * FROM comments WHERE id = :c_id", {"c_id": comment_id})
    row = res.fetchone()
    user_id = row['user_id']

    check_user(user_id)

    if request.method == "GET":
        comment = row
        session['referrer'] = request.headers.get('Referer')
        return render_template('comment_edit.html', comment=comment, name=get_session_name())

    empty_error = validate.check_empty(request.form['body'])

    if empty_error:
        flash(empty_error, 'error_msg')
        return redirect(url_for('comment_edit', comment_id=comment_id))

    db.session.execute("UPDATE comments SET body=:b_body, modified=NOW() WHERE id=:c_id", {"b_body": request.form['body'], "c_id": comment_id})
    db.session.commit()

    return redirect(session['referrer'])


@app.route('/comment/delete/<int:comment_id>', methods=["GET", "POST"])
def comment_delete(comment_id):
    res = db.session.execute("SELECT user_id FROM comments WHERE id = :c_id", {"c_id": comment_id})
    user_id = res.fetchone()[0]

    check_user(user_id)

    if request.method == "GET":
        abort(404)

    db.session.execute("DELETE FROM comments WHERE id = :c_id", {"c_id": comment_id})
    db.session.commit()

    return redirect(session['referrer'])


@app.route('/users/add', methods=['GET', 'POST'])
def users_add():
    if request.method == "GET":
        return render_template('users_add.html')

    form_username = request.form['username']
    form_nickname = request.form['nickname']
    form_password = request.form['password']
    form_password_confirm = request.form['password_confirm']

    empty_error = validate.check_empty(form_username, form_nickname, form_password, form_password_confirm)
    length_error = validate.check_length(username=form_username, nickname=form_nickname, password=form_password)
    exist_error = validate.check_exist(username=form_username, nickname=form_nickname)
    diff_error = validate.check_diff(form_password, form_password_confirm)

    if empty_error or length_error or exist_error or diff_error:
        if empty_error:
            flash(empty_error, 'error_msg')

        if length_error:
            for error in length_error:
                flash(error, 'error_msg')

        if exist_error:
            for error in exist_error:
                flash(error, 'error_msg')

        if diff_error:
            flash(diff_error, 'error_msg')

        return render_template('users_add.html'), 409

    if request.files['icon']:
        img = request.files['icon']
        img_name = img.filename
        img.save(os.path.join('static', 'icons', img_name))
        icon_path = img_name
    else:
        icon_path = 'default-icon.png'

    password = hashlib.sha256(request.form['password'].encode('utf-8')).hexdigest()

    db.session.execute("INSERT INTO users (username, password, nickname, icon, created, modified) VALUES (:u_name, :u_pass, :u_nick, :u_icon, NOW(), NOW())",
                       {"u_name": request.form['username'], "u_pass": password, "u_nick": request.form['nickname'], "u_icon": icon_path})
    db.session.commit()
    return redirect(url_for('login'))


@app.route('/users/edit', methods=['GET', 'POST'])
def users_edit():
    if 'user_id' not in session:
        abort(403)

    res = db.session.execute("SELECT * FROM users WHERE id = :u_id", {"u_id": session['user_id']})
    user_data = res.fetchone()

    if request.method == "GET":
        return render_template('users_edit.html', user_data=user_data, name=get_session_name())

    form_username = request.form['username']
    form_nickname = request.form['nickname']

    if form_username or form_nickname:
        length_error = validate.check_length(username=form_username, nickname=form_nickname)
        exist_error = validate.check_exist(username=form_username, nickname=form_nickname)

        if length_error or exist_error:
            if length_error:
                for error in length_error:
                    flash(error, 'error_msg')

            if exist_error:
                for error in exist_error:
                    flash(error, 'error_msg')

            return redirect(url_for('users_edit'))

        if form_username:
            db.session.execute("UPDATE users SET username=:u_name WHERE id=:u_id", {"u_name": form_username, "u_id": session['user_id']})
            session['username'] = form_username
            flash("ユーザ名を更新しました", 'success_msg')

        if form_nickname:
            db.session.execute("UPDATE users SET nickname=:u_nick WHERE id=:u_id", {"u_nick": form_nickname, "u_id": session['user_id']})
            flash("ニックネームを更新しました", 'success_msg')

    if request.files['icon']:
        img = request.files['icon']
        img_name = img.filename
        img.save(os.path.join('static', 'icons', img_name))

        if img_name[-4:] == ".png":
            tmp_img = Image.open(os.path.join('static', 'icons', img_name))
            tmp_rgb_img = tmp_img.convert('RGB')
            tmp_rgb_img.save(os.path.join('static', 'icons', img_name[:-4] + ".jpeg"), quality=100)

        db.session.execute("UPDATE users SET icon = :i_name WHERE id = :u_id", {"i_name": img_name, "u_id": session['user_id']})

    db.session.commit()

    return redirect(url_for('users_edit'))


@app.route('/users/password', methods=['GET', 'POST'])
def users_password():
    if 'user_id' not in session:
        abort(403)

    if request.method == "GET":
        return render_template('users_password.html', name=get_session_name())

    form_password = request.form['password']
    form_password_current = request.form['password_current']
    form_password_confirm = request.form['password_confirm']

    empty_error = validate.check_empty(form_password, form_password_current, form_password_confirm)
    length_error = validate.check_length(password=form_password)
    diff_error = validate.check_diff(form_password, form_password_confirm)

    user_id = session['user_id']
    res = db.session.execute("SELECT password FROM users WHERE id = :u_id", {"u_id": user_id})
    password_db = res.fetchone()[0]
    password_form = hashlib.sha256(form_password_current.encode('utf-8')).hexdigest()

    if empty_error or length_error or diff_error or password_db != password_form:
        if empty_error:
            flash(empty_error, 'error_msg')

        if length_error:
            flash(length_error[0], 'error_msg')

        if diff_error:
            flash(diff_error, 'error_msg')

        if password_db != password_form:
            flash("現在のパスワードが誤っています", 'error_msg')

        return render_template('users_password.html', name=get_session_name()), 409

    new_password = hashlib.sha256(form_password.encode('utf-8')).hexdigest()
    db.session.execute("UPDATE users SET password=:new_pass WHERE id=:u_id", {"new_pass": new_password, "u_id": user_id})
    db.session.commit()
    flash("パスワードを更新しました", 'success_msg')

    return redirect(url_for('users_password'))


if __name__ == "__main__":
    app.run(host='0.0.0.0', port=5000, debug=True)
