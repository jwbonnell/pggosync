-- Creation of product table
CREATE TABLE IF NOT EXISTS product (
  product_id INT NOT NULL,
  name varchar(250) NOT NULL,
  PRIMARY KEY (product_id)
);

-- Creation of country table
CREATE TABLE IF NOT EXISTS country (
  country_id INT NOT NULL,
  country_name varchar(450) NOT NULL,
  PRIMARY KEY (country_id)
);

-- Creation of city table
CREATE TABLE IF NOT EXISTS city (
  city_id INT NOT NULL,
  city_name varchar(450) NOT NULL,
  country_id INT NOT NULL,
  PRIMARY KEY (city_id),
  CONSTRAINT fk_country
      FOREIGN KEY(country_id) 
	  REFERENCES country(country_id)
);

-- Creation of store table
CREATE TABLE IF NOT EXISTS store (
  store_id INT NOT NULL,
  name varchar(250) NOT NULL,
  city_id INT NOT NULL,
  PRIMARY KEY (store_id),
  CONSTRAINT fk_city
      FOREIGN KEY(city_id) 
	  REFERENCES city(city_id)
);

-- Creation of user table
CREATE TABLE IF NOT EXISTS users (
  user_id INT NOT NULL,
  name varchar(250) NOT NULL,
  PRIMARY KEY (user_id)
);

-- Creation of status_name table
CREATE TABLE IF NOT EXISTS status_name (
  status_name_id INT NOT NULL,
  status_name varchar(450) NOT NULL,
  PRIMARY KEY (status_name_id)
);

-- Creation of sale table
CREATE TABLE IF NOT EXISTS sale (
  sale_id varchar(200) NOT NULL,
  amount DECIMAL(20,3) NOT NULL,
  date_sale TIMESTAMP,
  product_id INT NOT NULL,
  user_id INT NOT NULL,
  store_id INT NOT NULL, 
  PRIMARY KEY (sale_id),
  CONSTRAINT fk_product
      FOREIGN KEY(product_id) 
	  REFERENCES product(product_id),
  CONSTRAINT fk_user
      FOREIGN KEY(user_id) 
	  REFERENCES users(user_id),
  CONSTRAINT fk_store
      FOREIGN KEY(store_id) 
	  REFERENCES store(store_id)	  
);

-- Creation of order_status table
CREATE TABLE IF NOT EXISTS order_status (
  order_status_id varchar(200) NOT NULL,
  update_at TIMESTAMP,
  sale_id varchar(200) NOT NULL,
  status_name_id INT NOT NULL,
  PRIMARY KEY (order_status_id),
  CONSTRAINT fk_sale
      FOREIGN KEY(sale_id) 
	  REFERENCES sale(sale_id),
  CONSTRAINT fk_status_name
      FOREIGN KEY(status_name_id) 
	  REFERENCES status_name(status_name_id)  
);

-- Dummy tables for testing
CREATE TABLE dummy (
  id INT NOT NULL,
  name TEXT,
  PRIMARY KEY (id)
);

CREATE OR REPLACE FUNCTION do_something()
RETURNS trigger AS
$$
BEGIN
  RAISE NOTICE 'DO SOMETHING!';
  RETURN NEW;
END;
$$
LANGUAGE plpgsql;
CREATE TRIGGER do_something_trigger BEFORE INSERT OR UPDATE ON dummy FOR EACH ROW EXECUTE PROCEDURE do_something();

-- Dummy sequence
CREATE SEQUENCE dummy_seq START 1;

-- Dummy tables for testing
CREATE TABLE dummy_truncate (
  id INT NOT NULL,
  name TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE dummy_delete (
  id INT NOT NULL,
  name TEXT,
  PRIMARY KEY (id)
);

-- Dummy tables for testing
CREATE TABLE dummy_seed (
  id INT NOT NULL,
  name TEXT,
  PRIMARY KEY (id)
);

-- BIGSERIAL PK, self-referential FK, BOOLEAN, TIMESTAMP
CREATE TABLE employee (
  employee_id  BIGSERIAL PRIMARY KEY,
  name         VARCHAR(250) NOT NULL,
  role         VARCHAR(100),
  manager_id   BIGINT REFERENCES employee(employee_id),
  hire_date    TIMESTAMP,
  active       BOOLEAN NOT NULL DEFAULT true
);

-- BOOLEAN column, DATE range, NUMERIC discount
CREATE TABLE promotion (
  promotion_id  INT PRIMARY KEY,
  code          VARCHAR(50) NOT NULL,
  active        BOOLEAN NOT NULL DEFAULT true,
  start_date    DATE,
  end_date      DATE,
  discount_pct  NUMERIC(5,2)
);

-- Composite PK junction table
CREATE TABLE promotion_sale (
  promotion_id  INT          NOT NULL REFERENCES promotion(promotion_id),
  sale_id       VARCHAR(200) NOT NULL REFERENCES sale(sale_id),
  applied_at    TIMESTAMP,
  PRIMARY KEY (promotion_id, sale_id)
);

-- SERIAL PK, JSONB, NUMERIC rating, TEXT body
CREATE TABLE review (
  review_id   SERIAL PRIMARY KEY,
  product_id  INT NOT NULL REFERENCES product(product_id),
  user_id     INT NOT NULL REFERENCES users(user_id),
  rating      NUMERIC(3,1),
  body        TEXT,
  metadata    JSONB,
  created_at  TIMESTAMP DEFAULT now()
);

-- catalog schema: self-referential category tree, composite PK, JSONB product details
CREATE SCHEMA catalog;

CREATE TABLE catalog.category (
  category_id  INT PRIMARY KEY,
  name         VARCHAR(250) NOT NULL,
  parent_id    INT REFERENCES catalog.category(category_id),
  description  TEXT
);

CREATE TABLE catalog.product_category (
  product_id   INT NOT NULL REFERENCES public.product(product_id),
  category_id  INT NOT NULL REFERENCES catalog.category(category_id),
  PRIMARY KEY (product_id, category_id)
);

CREATE TABLE catalog.product_detail (
  product_id    INT PRIMARY KEY REFERENCES public.product(product_id),
  attributes    JSONB,
  discontinued  BOOLEAN NOT NULL DEFAULT false,
  barcode       BIGINT,
  weight_kg     NUMERIC(10,4)
);

-- inventory schema: cross-schema FK chain, composite PK, BIGINT quantity
CREATE SCHEMA inventory;

CREATE TABLE inventory.warehouse (
  warehouse_id  INT PRIMARY KEY,
  name          VARCHAR(250) NOT NULL,
  city_id       INT NOT NULL REFERENCES public.city(city_id),
  capacity      INT
);

CREATE TABLE inventory.stock_level (
  warehouse_id  INT     NOT NULL REFERENCES inventory.warehouse(warehouse_id),
  product_id    INT     NOT NULL REFERENCES public.product(product_id),
  quantity      BIGINT  NOT NULL DEFAULT 0,
  last_updated  TIMESTAMP DEFAULT now(),
  PRIMARY KEY (warehouse_id, product_id)
);

CREATE OR REPLACE VIEW summary_vw AS
   SELECT
      (SELECT count(*) as product_rows FROM public.product),
      (SELECT count(*) as city_rows FROM public.city),
      (SELECT count(*) as country_rows FROM public.country),
      (SELECT count(*) as store_rows FROM public.store),
      (SELECT count(*) as users_rows FROM public.users),
      (SELECT count(*) as sale_rows FROM public.sale),
      (SELECT count(*) as order_rows FROM public.order_status),
      (SELECT count(*) as status_rows FROM public.status_name),
      (SELECT count(*) as employee_rows FROM public.employee),
      (SELECT count(*) as promotion_rows FROM public.promotion),
      (SELECT count(*) as review_rows FROM public.review),
      (SELECT count(*) as category_rows FROM catalog.category),
      (SELECT count(*) as product_detail_rows FROM catalog.product_detail),
      (SELECT count(*) as warehouse_rows FROM inventory.warehouse),
      (SELECT count(*) as stock_level_rows FROM inventory.stock_level)
;