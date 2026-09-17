-- Fork (list grid) -- the shared fixture of integrations LIST-GRID-SPEC I1, I2, I3, I5 and I12,
-- loaded by listgrid.Load (internal/migrations and cmd DB tests).
--
-- Two lists (grid-single, grid-double), and one subscriber per
--   language variant x subscriber status x subscription status x hold/no-hold x copy,
-- each subscribed to BOTH lists with the same subscription status and meta. The e-mail spells
-- the combination (variant-sstatus-slstatus-hold-copy@grid.test) so a test derives the expected
-- segment and language of every row from the address alone, never from the SQL under test.
-- The copy counts differ per variant and per subscription status so that no two cells of the
-- grid hold the same number by symmetry (a swapped cell must show).
INSERT INTO lists (uuid, name, type, optin) VALUES
    (gen_random_uuid(), 'grid-single', 'private', 'single'),
    (gen_random_uuid(), 'grid-double', 'private', 'double');

CREATE TEMP TABLE grid_fixture AS
WITH v(variant, attribs, vcopies) AS (VALUES
    ('absent',    '{"city": "x"}'::JSONB, 3),
    ('jsonnull',  'null'::JSONB,          1),
    ('empty',     '{"lang": ""}'::JSONB,  2),
    ('space',     '{"lang": " "}'::JSONB, 1),
    ('spacefr',   '{"lang": " fr"}'::JSONB, 1),
    ('nonstring', '{"lang": 5}'::JSONB,   1),
    ('upperen',   '{"lang": "EN"}'::JSONB, 4),
    ('upperfr',   '{"lang": "FR"}'::JSONB, 2),
    ('frca',      '{"lang": "fr-CA"}'::JSONB, 1),
    ('pt',        '{"lang": "pt"}'::JSONB, 2),
    ('es',        '{"lang": "es"}'::JSONB, 3),
    ('de',        '{"lang": "de"}'::JSONB, 1),
    ('upperit',   '{"lang": "IT"}'::JSONB, 2)
),
ss(s_status) AS (VALUES ('enabled'), ('disabled'), ('blocklisted')),
sl(sl_status, slcopies) AS (VALUES ('unconfirmed', 2), ('confirmed', 3), ('unsubscribed', 1)),
h(hold) AS (VALUES ('nohold'), ('hold'))
SELECT v.variant, v.attribs, ss.s_status, sl.sl_status, h.hold, n AS copy,
    FORMAT('%s-%s-%s-%s-%s@grid.test', v.variant, ss.s_status, sl.sl_status, h.hold, n) AS email
FROM v CROSS JOIN ss CROSS JOIN sl CROSS JOIN h
CROSS JOIN LATERAL GENERATE_SERIES(1, v.vcopies * sl.slcopies) n;

INSERT INTO subscribers (uuid, email, name, status, attribs)
    SELECT gen_random_uuid(), email, email, s_status::subscriber_status, attribs FROM grid_fixture;

INSERT INTO subscriber_lists (subscriber_id, list_id, status, meta)
    SELECT s.id, l.id, f.sl_status::subscription_status,
        (CASE WHEN f.hold = 'hold' THEN '{"hold": {"reason": "never-engaged", "source": "test", "rule": "r"}}'::JSONB ELSE '{}'::JSONB END)
    FROM grid_fixture f
    JOIN subscribers s ON (s.email = f.email)
    CROSS JOIN lists l WHERE l.name IN ('grid-single', 'grid-double');

DROP TABLE grid_fixture;
