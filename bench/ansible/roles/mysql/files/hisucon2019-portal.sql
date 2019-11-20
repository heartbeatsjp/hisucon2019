--  HISUCON2019 Portal

DROP DATABASE IF EXISTS hisucon2019_portal;
CREATE DATABASE IF NOT EXISTS hisucon2019_portal DEFAULT CHARACTER SET utf8mb4;
USE hisucon2019_portal;

DROP TABLE IF EXISTS bench;

CREATE TABLE bench (
    id              int(11)         NOT NULL AUTO_INCREMENT,
    team            VARCHAR(64)     NOT NULL,
    ipaddress       VARCHAR(64)     NOT NULL,
    result          json            NOT NULL,
    created_at      datetime(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    resultfile      VARCHAR(100),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
