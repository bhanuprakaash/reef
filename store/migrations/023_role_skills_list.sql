UPDATE role SET
  skills = CASE WHEN p.is_list THEN p.names ELSE '[]' END,
  persona = role.persona || CASE
    WHEN p.extra = '' THEN ''
    WHEN role.persona = '' THEN p.extra
    ELSE char(10) || char(10) || p.extra
  END
FROM (
  SELECT id, names, is_list, CASE WHEN is_list THEN rest ELSE old END AS extra
  FROM (
    SELECT id, old, rest, names,
      head <> '' AND instr(replace(head, ', ', ''), ' ') = 0 AND json_valid(names) AS is_list
    FROM (
      SELECT id, old, rest, head,
        '["' || replace(replace(head, ', ', ','), ',', '","') || '"]' AS names
      FROM (
        SELECT id, trim(skills, char(32, 9, 10, 13)) AS old,
          trim(CASE WHEN instr(skills, '.') > 0 THEN substr(skills, 1, instr(skills, '.') - 1)
                    ELSE skills END, char(32, 9, 10, 13)) AS head,
          trim(CASE WHEN instr(skills, '.') > 0 THEN substr(skills, instr(skills, '.') + 1)
                    ELSE '' END, char(32, 9, 10, 13)) AS rest
        FROM role
      )
    )
  )
) AS p
WHERE role.id = p.id;
