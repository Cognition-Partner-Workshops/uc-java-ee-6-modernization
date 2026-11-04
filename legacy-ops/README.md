# legacy-ops: SYNTHETIC traditional WebSphere on AIX estate

> **Everything in this folder is synthetic.** It was written for the modernization demo. It is not taken from any customer and it is not how this repo runs. The app runs on **WebSphere Liberty** using `daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml`, with embedded Derby and the embedded Liberty messaging engine (see `docs/BASELINE.md`). Nothing here runs in CI or from the Makefile.

## Why it exists

A typical insurer's WebSphere estate is more than the EAR. It is **traditional WAS (tWAS) 8.5.5 / 9.0 Network Deployment on AIX**: datasources, J2C aliases, IBM MQ resources and the app install are scripted in wsadmin, and the servers are driven by ksh wrappers, `ulimit` settings and cron jobs. Those are the artefacts a migration has to read and translate. This folder recreates that layer around DayTrader's **real** JNDI names, so the migration has a realistic "as-is" estate to work from.

Every resource name below matches the app's real bindings: `ibm-web-bnd.xml`, `ibm-ejb-jar-bnd.xml`, `server.xml` and the `@Resource`/`@MessageDriven` annotations. `tools/ws_inventory.py` puts the wsadmin resources in `docs/WEBSPHERE_INVENTORY.md`, next to the real ones.

## Contents

| File | Represents |
| --- | --- |
| `wsadmin/00_env.py` | Cell / node / server names, Db2 driver path, MQ queue manager and channel |
| `wsadmin/01_jdbc_provider.py` | Db2 Universal JDBC Driver Provider (XA), type 4, `db2jcc4.jar` |
| `wsadmin/02_auth_aliases.py` | J2C aliases `TradeDataSourceAuthData` (Db2) and `TradeAdminAuthData` (MQ/MDB) |
| `wsadmin/03_datasources.py` | `jdbc/TradeDataSource` (XA) and `jdbc/NoTxTradeDataSource` on Db2 `TRADEDB`, with pool sizing |
| `wsadmin/04_mq_jms.py` | IBM MQ QCF `jms/TradeBrokerQCF`, TCF `jms/TradeStreamerTCF`, queue `jms/TradeBrokerQueue`, topic `jms/TradeStreamerTopic`, activation specs `eis/TradeBrokerMDB` → `DTBroker3MDB` and `eis/TradeStreamerMDB` → `DTStreamer3MDB` |
| `wsadmin/05_shared_library.py` | Shared library `DayTraderSharedLib` |
| `wsadmin/06_install_app.py` | `AdminApp.install`: context roots `/daytrader` and `/rest`, resource-ref, message-destination and MDB bindings, shared-library mapping |
| `aix/setenv_was.ksh` | `WAS_HOME=/usr/IBM/WebSphere/AppServer`, profile `AppSrv01`, AIX env vars, `ulimit` |
| `aix/start_tradeserver.ksh`, `aix/stop_tradeserver.ksh` | Wrappers around `profiles/AppSrv01/bin/startServer.sh` / `stopServer.sh` |
| `aix/rotate_was_logs.ksh` | Nightly `SystemOut.log` / `SystemErr.log` rotation plus javacore/FFDC clean-up, with its cron lines |

## What the migration must translate

| tWAS / AIX artefact | Target (see `docs/TARGET_STATE.md`) |
| --- | --- |
| `jdbc/TradeDataSource`, `jdbc/NoTxTradeDataSource`, Db2 JDBC provider, pool settings | `spring.datasource.*` (HikariCP) against Aurora PostgreSQL |
| J2C alias `TradeDataSourceAuthData` / `TradeAdminAuthData` | AWS Secrets Manager secrets, injected into the ECS task |
| MQ QCF/TCF, `jms/TradeBrokerQueue`, `jms/TradeStreamerTopic` | Amazon SQS (queue) and SNS→SQS fan-out (topic), or Amazon MQ for JMS semantics |
| Activation specs `eis/TradeBrokerMDB`, `eis/TradeStreamerMDB` | `@SqsListener` (Spring Cloud AWS) or `@JmsListener` on Amazon MQ |
| Shared library, `AdminApp.install` bindings, context roots | Dependencies in the Spring Boot fat jar; `server.servlet.context-path`; ALB listener rules |
| `startServer.sh`/`stopServer.sh` wrappers, `ulimit`, cron log rotation | ECS service lifecycle, task-definition `ulimits`, CloudWatch Logs with retention |

## Not included: a Liberty + IBM MQ variant

An optional `server-mq.xml` running Liberty against a local IBM MQ container was **not added**, because it was not proven to run here. It would need the IBM MQ resource adapter (`wmqJmsClient` feature plus the RAR) and the `icr.io/ibm-messaging/mq` developer image. That image only starts with `LICENSE=accept`, and its terms (IBM MQ Advanced for Developers: development use only, not for production) need a license review before anyone uses it in a customer-facing demo. This is an open gap: the MQ wiring above is synthetic and has never been run against a real queue manager.
