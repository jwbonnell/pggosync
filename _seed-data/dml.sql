-- Set params
set session my.number_of_sales = '100';
set session my.number_of_users = '100';
set session my.number_of_products = '100';
set session my.number_of_stores = '100';
set session my.number_of_countries = '100';
set session my.number_of_cities = '30';
set session my.status_names = '5';
set session my.start_date = '2019-01-01 00:00:00';
set session my.end_date = '2020-02-01 00:00:00';
set session my.number_of_dummy_recs = '100';

-- load the pgcrypto extension to gen_random_uuid ()
CREATE EXTENSION pgcrypto;

-- Filling of products
INSERT INTO product
select id, concat('Product ', id) 
FROM GENERATE_SERIES(1, current_setting('my.number_of_products')::int) as id;

-- Filling of countries
INSERT INTO country
select id, concat('Country ', id) 
FROM GENERATE_SERIES(1, current_setting('my.number_of_countries')::int) as id;

-- Filling of cities
INSERT INTO city
select id
	, concat('City ', id)
	, floor(random() * (current_setting('my.number_of_countries')::int) + 1)::int
FROM GENERATE_SERIES(1, current_setting('my.number_of_cities')::int) as id;

-- Filling of stores
INSERT INTO store
select id
	, concat('Store ', id)
	, floor(random() * (current_setting('my.number_of_cities')::int) + 1)::int
FROM GENERATE_SERIES(1, current_setting('my.number_of_stores')::int) as id;

-- Filling of users
INSERT INTO users
select id
	, concat('User ', id)
FROM GENERATE_SERIES(1, current_setting('my.number_of_users')::int) as id;

-- Filling of users
INSERT INTO status_name
select status_name_id
	, concat('Status Name ', status_name_id)
FROM GENERATE_SERIES(1, current_setting('my.status_names')::int) as status_name_id;

-- Filling of sales  
INSERT INTO sale
select gen_random_uuid ()
	, round(CAST(float8 (random() * 10000) as numeric), 3)
	, TO_TIMESTAMP(start_date, 'YYYY-MM-DD HH24:MI:SS') +
		random()* (TO_TIMESTAMP(end_date, 'YYYY-MM-DD HH24:MI:SS') 
							- TO_TIMESTAMP(start_date, 'YYYY-MM-DD HH24:MI:SS'))
	, floor(random() * (current_setting('my.number_of_products')::int) + 1)::int
	, floor(random() * (current_setting('my.number_of_users')::int) + 1)::int
	, floor(random() * (current_setting('my.number_of_stores')::int) + 1)::int
FROM GENERATE_SERIES(1, current_setting('my.number_of_sales')::int) as id
	, current_setting('my.start_date') as start_date
	, current_setting('my.end_date') as end_date;

-- Filling of order_status  
INSERT INTO order_status
select gen_random_uuid ()
	, date_sale + random()* (date_sale + '5 days' - date_sale)
	, sale_id
	, floor(random() * (current_setting('my.status_names')::int) + 1)::int
from sale;

-- Filling of dummy tables
INSERT INTO dummy
   select id, concat('DUMMY_', id::TEXT)
   FROM GENERATE_SERIES(1, current_setting('my.number_of_dummy_recs')::int) as id;

-- Additional seed data used for testing

DO
$$
    DECLARE
        countryId INT := 1000;
        cityId INT := 1000;
        num_recs INT := 10;

    BEGIN
        FOR i IN 1 .. num_recs+1 LOOP
                RAISE NOTICE 'COUNTRY %', countryId;
                INSERT INTO country (country_id, country_name) VALUES (countryId, 'Country ' || countryId);
                FOR j IN 1 .. num_recs+1 LOOP
                        RAISE NOTICE 'CITY %', cityId;
                        INSERT INTO city (city_id, city_name, country_id) VALUES (cityId, 'City ' || cityId, countryId);
                        cityId = cityId + 1;
                    END LOOP;
                countryId = countryId + 1;
            END LOOP;
    END;
$$;

-- Filling of employee (50 rows: 1 CEO, 5 managers, 44 staff)
-- Uses a DO block so the self-referential manager_id FK is satisfied in insertion order.
DO $$
DECLARE
    ceo_id   BIGINT;
    mgr_ids  BIGINT[] := '{}';
    mgr_id   BIGINT;
BEGIN
    INSERT INTO employee (name, role, manager_id, hire_date, active)
    VALUES ('Alice Chen', 'CEO', NULL, '2010-03-15 09:00:00', true)
    RETURNING employee_id INTO ceo_id;

    FOR i IN 1..5 LOOP
        INSERT INTO employee (name, role, manager_id, hire_date, active)
        VALUES (
            'Manager ' || i,
            'Manager',
            ceo_id,
            '2013-01-01'::timestamp + (i * 90 || ' days')::interval,
            true
        )
        RETURNING employee_id INTO mgr_id;
        mgr_ids := array_append(mgr_ids, mgr_id);
    END LOOP;

    FOR i IN 1..44 LOOP
        INSERT INTO employee (name, role, manager_id, hire_date, active)
        VALUES (
            'Employee ' || i,
            CASE i % 3
                WHEN 0 THEN 'Senior Staff'
                WHEN 1 THEN 'Staff'
                ELSE 'Junior Staff'
            END,
            mgr_ids[1 + (i % 5)],
            '2015-01-01'::timestamp + (i * 7 || ' days')::interval,
            (i % 10 != 0)
        );
    END LOOP;
END;
$$;

-- Filling of promotion (20 rows, ~60% active)
INSERT INTO promotion (promotion_id, code, active, start_date, end_date, discount_pct)
SELECT
    id,
    'PROMO' || lpad(id::text, 3, '0'),
    (random() > 0.4),
    ('2023-01-01'::date + ((id - 1) * 18))::date,
    ('2023-01-01'::date + ((id - 1) * 18) + 30)::date,
    round((random() * 45 + 5)::numeric, 2)
FROM generate_series(1, 20) AS id;

-- Filling of promotion_sale (~150 rows)
INSERT INTO promotion_sale (promotion_id, sale_id, applied_at)
SELECT
    (row_number() OVER () % 20 + 1)::int,
    sale_id,
    date_sale + (random() * interval '2 days')
FROM (
    SELECT sale_id, date_sale FROM sale ORDER BY random() LIMIT 150
) x;

-- Filling of review (200 rows)
INSERT INTO review (product_id, user_id, rating, body, metadata, created_at)
SELECT
    (floor(random() * 100) + 1)::int,
    (floor(random() * 100) + 1)::int,
    round((random() * 4 + 1)::numeric, 1),
    'Review ' || id || ': ' || (
        CASE (id % 4)
            WHEN 0 THEN 'Great product, would buy again.'
            WHEN 1 THEN 'Good quality for the price.'
            WHEN 2 THEN 'Arrived on time, as described.'
            ELSE 'Decent, but room for improvement.'
        END
    ),
    jsonb_build_object(
        'verified',      random() > 0.3,
        'helpful_votes', floor(random() * 50)::int,
        'source',        CASE WHEN random() < 0.5 THEN 'web' ELSE 'mobile' END
    ),
    '2023-01-01'::timestamp + (floor(random() * 730) || ' days')::interval
FROM generate_series(1, 200) AS id;

-- Filling of catalog.category (5 top-level + 25 subcategories)
INSERT INTO catalog.category (category_id, name, parent_id, description)
VALUES
    (1, 'Electronics',       NULL, 'Consumer electronics and accessories'),
    (2, 'Apparel',           NULL, 'Clothing and fashion'),
    (3, 'Home & Garden',     NULL, 'Home improvement and garden supplies'),
    (4, 'Sports & Outdoors', NULL, 'Sports equipment and outdoor gear'),
    (5, 'Food & Beverage',   NULL, 'Food, drinks, and groceries');

INSERT INTO catalog.category (category_id, name, parent_id, description)
SELECT
    10 + id,
    'Subcategory ' || (10 + id),
    ((id - 1) / 5 + 1),
    'Subcategory under category ' || ((id - 1) / 5 + 1)
FROM generate_series(1, 25) AS id;

-- Filling of catalog.product_category (~150 unique assignments)
INSERT INTO catalog.product_category (product_id, category_id)
SELECT DISTINCT product_id, category_id
FROM (
    SELECT
        (floor(random() * 100) + 1)::int AS product_id,
        (floor(random() * 25) + 11)::int AS category_id
    FROM generate_series(1, 400)
) x
ON CONFLICT DO NOTHING;

-- Filling of catalog.product_detail (one row per product)
INSERT INTO catalog.product_detail (product_id, attributes, discontinued, barcode, weight_kg)
SELECT
    id,
    jsonb_build_object(
        'color', CASE id % 5
            WHEN 0 THEN 'red'
            WHEN 1 THEN 'blue'
            WHEN 2 THEN 'green'
            WHEN 3 THEN 'black'
            ELSE 'white'
        END,
        'size',     CASE id % 3 WHEN 0 THEN 'S' WHEN 1 THEN 'M' ELSE 'L' END,
        'in_stock', random() > 0.2
    ),
    (random() < 0.1),
    (floor(random() * 9000000000) + 1000000000)::bigint,
    round((random() * 50 + 0.1)::numeric, 4)
FROM generate_series(1, 100) AS id;

-- Filling of inventory.warehouse (10 rows)
INSERT INTO inventory.warehouse (warehouse_id, name, city_id, capacity)
SELECT
    id,
    'Warehouse ' || id,
    (floor(random() * 30) + 1)::int,
    (floor(random() * 10000) + 1000)::int
FROM generate_series(1, 10) AS id;

-- Filling of inventory.stock_level (~400 unique warehouse+product pairs)
INSERT INTO inventory.stock_level (warehouse_id, product_id, quantity, last_updated)
SELECT DISTINCT warehouse_id, product_id, quantity, last_updated
FROM (
    SELECT
        (floor(random() * 10) + 1)::int  AS warehouse_id,
        (floor(random() * 100) + 1)::int AS product_id,
        floor(random() * 10000)::bigint  AS quantity,
        now() - (floor(random() * 90) || ' days')::interval AS last_updated
    FROM generate_series(1, 700)
) x
ON CONFLICT DO NOTHING;