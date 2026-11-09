CREATE TABLE IF NOT EXISTS bookmark_alias(
		alias       TEXT       NOT NULL,
		alias_hash  CHAR(64)   NOT NULL,
		bookmark_id INT(11)    NOT NULL,
		created_at  TIMESTAMP  NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(alias_hash),
		CONSTRAINT bookmark_alias_bookmark_FK FOREIGN KEY(bookmark_id) REFERENCES bookmark(id) ON DELETE CASCADE)
		CHARACTER SET utf8mb4;

CREATE INDEX bookmark_alias_bookmark_id_IDX ON bookmark_alias(bookmark_id);
