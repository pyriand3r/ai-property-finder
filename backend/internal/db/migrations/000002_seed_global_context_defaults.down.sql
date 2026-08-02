UPDATE context
SET content_md = '',
    updated_at = NOW()
WHERE scope = 'global'
  AND profile_id IS NULL;
