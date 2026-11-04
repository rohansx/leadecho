-- +goose Up
-- HN items crawled before cleanHNText existed were stored as raw HTML
-- (<p>, &#x27;, &#x2F;, ...). Mirror cleanHNText: <p> -> blank line, strip
-- tags, unescape. ponytail: only the entities HN actually emits; &amp; last.
-- +goose StatementBegin
CREATE FUNCTION pg_temp.clean_hn(s TEXT) RETURNS TEXT LANGUAGE sql IMMUTABLE AS $$
  SELECT btrim(
    replace(replace(replace(replace(replace(replace(replace(
      regexp_replace(replace(s, '<p>', E'\n\n'), '<[^>]*>', '', 'g'),
    '&#x2F;', '/'), '&#x27;', ''''), '&#39;', ''''), '&quot;', '"'),
    '&gt;', '>'), '&lt;', '<'), '&amp;', '&'),
  E' \n\t')
$$;
-- +goose StatementEnd

UPDATE mentions
SET title = pg_temp.clean_hn(title),
    content = pg_temp.clean_hn(content)
WHERE platform = 'hackernews'
  AND (content ~ '<[a-zA-Z/]|&(#x?[0-9a-fA-F]+|quot|amp|lt|gt);'
       OR title ~ '&(#x?[0-9a-fA-F]+|quot|amp|lt|gt);');

-- Other platforms (dev.to, Reddit) only escape entities: decode, keep markup.
-- +goose StatementBegin
CREATE FUNCTION pg_temp.unescape_entities(s TEXT) RETURNS TEXT LANGUAGE sql IMMUTABLE AS $$
  SELECT replace(replace(replace(replace(replace(replace(replace(s,
    '&#x2F;', '/'), '&#x27;', ''''), '&#39;', ''''), '&quot;', '"'),
    '&gt;', '>'), '&lt;', '<'), '&amp;', '&')
$$;
-- +goose StatementEnd

UPDATE mentions
SET title = pg_temp.unescape_entities(title),
    content = pg_temp.unescape_entities(content)
WHERE platform <> 'hackernews'
  AND (content ~ '&(#x?[0-9a-fA-F]+|quot|amp|lt|gt);'
       OR title ~ '&(#x?[0-9a-fA-F]+|quot|amp|lt|gt);');

-- +goose Down
-- Irreversible data cleanup; nothing to undo.
SELECT 1;
