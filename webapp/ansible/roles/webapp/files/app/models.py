from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy import Column, Integer, String, DATETIME, Text
from datetime import datetime


Base = declarative_base()


class Bulletins(Base):
    __tablename__ = 'bulletins'

    id = Column(Integer, primary_key=True)
    user_id = Column(Integer)
    title = Column(String)
    body = Column(String(255))
    created = Column(DATETIME, default=datetime.now)
    modified = Column(DATETIME, default=datetime.now)

    def __init__(self, user_id, title, body):
        self.user_id = user_id
        self.title = title
        self.body = body
        now = datetime.now()
        self.created = now
        self.modified = now


class Users(Base):
    __tablename__ = 'users'

    id = Column(Integer, primary_key=True)
    username = Column(String(16))
    password = Column(String(255))
    nickname = Column(String(32))
    icon = Column(Text)

    def __init__(self, username, password, nickname):
        self.username = username
        self.password = password
        self.nickname = nickname
        self.icon = "takefusa.png"


class Comments(Base):
    __tablename__ = 'comments'

    id = Column(Integer, primary_key=True)
    bulletin_id = Column(Integer)
    user_id = Column(Integer)
    body = Column(String(255))
    created = Column(DATETIME, default=datetime.now)
    modified = Column(DATETIME, default=datetime.now)

    def __init__(self, bulletin_id, user_id, body):
        self.bulletin_id = bulletin_id
        self.user_id = user_id
        self.body = body
        now = datetime.now()
        self.created = now
        self.modified = now


class Accesslog(Base):
    __tablename__ = 'accesslog'

    id = Column(Integer, primary_key=True)
    bulletin_id = Column(Integer)
    access = Column(DATETIME, default=datetime.now)

    def __init__(self, bulletin_id):
        self.bulletin_id = bulletin_id
        now = datetime.now()
        self.access = now


class Bulletins_Star(Base):
    __tablename__ = 'bulletins_star'

    id = Column(Integer, primary_key=True)
    bulletin_id = Column(Integer)
    access = Column(DATETIME, default=datetime.now)

    def __init__(self, bulletin_id):
        self.bulletin_id = bulletin_id
        now = datetime.now()
        self.access = now


class Comments_Star(Base):
    __tablename__ = 'comments_star'

    id = Column(Integer, primary_key=True)
    comment_id = Column(Integer)
    access = Column(DATETIME, default=datetime.now)

    def __init__(self, comment_id):
        self.comment_id = comment_id
        now = datetime.now()
        self.access = now
