-- DayTrader schema on PostgreSQL. Same tables and columns as
-- daytrader3-ee6-web/src/main/webapp/dbscripts/derby/Table.ddl (DOUBLE -> DOUBLE PRECISION).
-- KEYGENEJB keeps the legacy key-block allocator; OPENJPASEQ is dropped (OpenJPA only).
DROP TABLE IF EXISTS holdingejb;
DROP TABLE IF EXISTS accountprofileejb;
DROP TABLE IF EXISTS quoteejb;
DROP TABLE IF EXISTS keygenejb;
DROP TABLE IF EXISTS accountejb;
DROP TABLE IF EXISTS orderejb;

CREATE TABLE holdingejb (
  purchaseprice     DECIMAL(14, 2),
  holdingid         INTEGER NOT NULL,
  quantity          DOUBLE PRECISION NOT NULL,
  purchasedate      TIMESTAMP,
  account_accountid INTEGER,
  quote_symbol      VARCHAR(250),
  CONSTRAINT pk_holdingejb PRIMARY KEY (holdingid)
);

CREATE TABLE accountprofileejb (
  address    VARCHAR(250),
  passwd     VARCHAR(250),
  userid     VARCHAR(250) NOT NULL,
  email      VARCHAR(250),
  creditcard VARCHAR(250),
  fullname   VARCHAR(250),
  CONSTRAINT pk_accountprofile2 PRIMARY KEY (userid)
);

CREATE TABLE quoteejb (
  low         DECIMAL(14, 2),
  open1       DECIMAL(14, 2),
  volume      DOUBLE PRECISION NOT NULL,
  price       DECIMAL(14, 2),
  high        DECIMAL(14, 2),
  companyname VARCHAR(250),
  symbol      VARCHAR(250) NOT NULL,
  change1     DOUBLE PRECISION NOT NULL,
  CONSTRAINT pk_quoteejb PRIMARY KEY (symbol)
);

CREATE TABLE keygenejb (
  keyval  INTEGER NOT NULL,
  keyname VARCHAR(250) NOT NULL,
  CONSTRAINT pk_keygenejb PRIMARY KEY (keyname)
);

CREATE TABLE accountejb (
  creationdate   TIMESTAMP,
  openbalance    DECIMAL(14, 2),
  logoutcount    INTEGER NOT NULL,
  balance        DECIMAL(14, 2),
  accountid      INTEGER NOT NULL,
  lastlogin      TIMESTAMP,
  logincount     INTEGER NOT NULL,
  profile_userid VARCHAR(250),
  CONSTRAINT pk_accountejb PRIMARY KEY (accountid)
);

CREATE TABLE orderejb (
  orderfee          DECIMAL(14, 2),
  completiondate    TIMESTAMP,
  ordertype         VARCHAR(250),
  orderstatus       VARCHAR(250),
  price             DECIMAL(14, 2),
  quantity          DOUBLE PRECISION NOT NULL,
  opendate          TIMESTAMP,
  orderid           INTEGER NOT NULL,
  account_accountid INTEGER,
  quote_symbol      VARCHAR(250),
  holding_holdingid INTEGER,
  CONSTRAINT pk_orderejb PRIMARY KEY (orderid)
);

CREATE INDEX account_userid ON accountejb (profile_userid);
CREATE INDEX holding_accountid ON holdingejb (account_accountid);
CREATE INDEX order_accountid ON orderejb (account_accountid);
CREATE INDEX order_holdingid ON orderejb (holding_holdingid);
CREATE INDEX closed_orders ON orderejb (account_accountid, orderstatus);
