CREATE DATABASE bb_app;
use bb_app;

CREATE TABLE `comments` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `bulletin_id` int(10) unsigned NOT NULL,
  `user_id` int(10) unsigned NOT NULL,
  `body` text NOT NULL,
  `created` datetime NOT NULL,
  `modified` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `bulletins` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int(10) unsigned NOT NULL,
  `title` text NOT NULL,
  `body` text NOT NULL,
  `created` datetime NOT NULL,
  `modified` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `users` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(16) NOT NULL,
  `password` varchar(255) NOT NULL,
  `nickname` varchar(32) NOT NULL,
  `icon` text,
  `created` datetime NOT NULL,
  `modified` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

create table accesslog (
  id int(10) unsigned not null auto_increment,
  bulletin_id int(10) unsigned not null,
  access datetime not null,
  primary key (id)
) engine=innodb default charset=utf8mb4;

create table comments_star (
  id int(10) unsigned not null auto_increment,
  comment_id int(10) unsigned not null,
  access datetime not null,
  primary key (id)
) engine=innodb default charset=utf8mb4;

create table bulletins_star (
  id int(10) unsigned not null auto_increment,
  bulletin_id int(10) unsigned not null,
  access datetime not null,
  primary key (id)
) engine=innodb default charset=utf8mb4;

LOAD DATA LOCAL INFILE '/tmp/accesslog.txt' INTO TABLE accesslog;
LOAD DATA LOCAL INFILE '/tmp/bulletins.txt' INTO TABLE bulletins LINES TERMINATED BY '\n';
LOAD DATA LOCAL INFILE '/tmp/comments.txt' INTO TABLE comments;
LOAD DATA LOCAL INFILE '/tmp/users.txt' INTO TABLE users;
LOAD DATA LOCAL INFILE '/tmp/bulletins_star.txt' INTO TABLE bulletins_star;
LOAD DATA LOCAL INFILE '/tmp/comments_star.txt' INTO TABLE comments_star;
