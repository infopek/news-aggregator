ALTER TABLE article_sources
    ADD COLUMN external_ids_json TEXT NOT NULL DEFAULT '[]'
    CHECK (json_valid(external_ids_json) AND json_type(external_ids_json) = 'array');

UPDATE article_sources
SET external_ids_json = CASE
    WHEN external_id = '' THEN '[]'
    ELSE json_array(external_id)
END;
