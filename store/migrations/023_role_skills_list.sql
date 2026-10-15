UPDATE role SET
  skills = CASE WHEN p.is_list THEN p.names ELSE '[]' END,
  persona = role.persona || CASE
    WHEN p.extra = '' THEN ''
    WHEN role.persona = '' THEN p.extra
    ELSE char(10) || char(10) || p.extra
  END
FROM (
  SELECT id, is_list,
    (SELECT json_group_array(value) FROM json_each(safe_raw) WHERE value <> '') AS names,
    CASE WHEN is_list THEN rest ELSE old END AS extra
  FROM (
    SELECT id, old, rest, raw,
      head <> '' AND instr(replace(head, ', ', ''), ' ') = 0 AND json_valid(raw) AS is_list,
      CASE WHEN json_valid(raw) THEN raw ELSE '[]' END AS safe_raw
    FROM (
      SELECT id, old, head, rest,
        '["' || replace(replace(head, ', ', ','), ',', '","') || '"]' AS raw
      FROM (
        SELECT id, old,
          trim(CASE WHEN instr(old, '. ') > 0 THEN substr(old, 1, instr(old, '. ') - 1)
                    WHEN substr(old, -1) = '.' THEN substr(old, 1, length(old) - 1)
                    ELSE old END, char(32, 9, 10, 13)) AS head,
          trim(CASE WHEN instr(old, '. ') > 0 THEN substr(old, instr(old, '. ') + 2)
                    ELSE '' END, char(32, 9, 10, 13)) AS rest
        FROM (SELECT id, trim(skills, char(32, 9, 10, 13)) AS old FROM role)
      )
    )
  )
) AS p
WHERE role.id = p.id;
