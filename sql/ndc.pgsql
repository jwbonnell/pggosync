SELECT ns.nspname        AS schema
     , class.relname     AS "table"
     , con.conname       AS "constraint"
     , con.condeferrable AS "deferrable"
     , con.condeferred   AS deferred
     , con.contype
FROM pg_constraint con
INNER JOIN pg_class class ON class.oid = con.conrelid
INNER JOIN pg_namespace ns ON ns.oid = class.relnamespace
WHERE ns.nspname != 'pg_catalog'
ORDER BY 1, 2, 3;