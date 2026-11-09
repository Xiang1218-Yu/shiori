CREATE TABLE IF NOT EXISTS bookmark_alias(
		alias       TEXT        NOT NULL,
		alias_hash  CHAR(64)    NOT NULL,
		bookmark_id INTEGER     NOT NULL,
		created_at  TIMESTAMP(0) NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT bookmark_alias_PK PRIMARY KEY(alias_hash),
		CONSTRAINT bookmark_alias_bookmark_FK FOREIGN KEY(bookmark_id) REFERENCES bookmark(id) ON DELETE CASCADE);

CREATE INDEX IF NOT EXISTS bookmark_alias_bookmark_id_IDX ON bookmark_alias(bookmark_id);
