# Baseline: DayTrader 3 (Java EE 6) as it runs today

This file records the legacy application running unchanged, before any migration work. It is the reference point the parity harness (`parity/`) checks against.

## Versions

| Item | Value |
| --- | --- |
| Source level | Java 7 (`source`/`target` 1.7, inherited from `net.wasdev.maven.parent:java7-parent:1.3`) |
| Build JDK | OpenJDK 1.8 (`/usr/lib/jvm/java-8-openjdk-amd64`). JDK 7 is no longer packaged for current Linux distributions; JDK 8 compiles `-source 1.7 -target 1.7` and is the JDK the old Travis job used (`openjdk8`). |
| Maven | 3.6.3 (`maven-compiler-plugin` 3.1, `maven-surefire-plugin` 2.22.2 for the new JUnit 5 tests) |
| Runtime image | `icr.io/appcafe/websphere-liberty:full-java8-ibmjava-ubi` |
| Runtime reported by the server | WebSphere Application Server (Liberty) 26.0.0.11, IBM J9 VM 8.0.8.x |
| Liberty features (`server.xml`) | `ejbLite-3.1`, `jsf-2.0`, `jaxrs-1.1`, `jpa-2.0`, `jmsMdb-3.1`, `wasJmsServer-1.0`, `wasJmsClient-1.1` |
| Database | Embedded Apache Derby 10.10.1.1 (`derby-10.10.1.1.jar`, copied by the EAR module into `daytrader3-ee6-wlpcfg/shared/resources/Daytrader3_SampleDerbyLibs`) |
| Messaging | Liberty embedded messaging engine (`wasJmsServer`). Not WebSphere MQ. |
| Python (parity harness) | 3.10+, dependencies in `parity/requirements.txt` |

The real runtime is **Liberty**, not traditional WebSphere (tWAS). The tWAS-on-AIX material in `legacy-ops/` is synthetic; see `legacy-ops/README.md`.

## Commands

```bash
make up      # docker pull the Liberty image; create .venv with parity deps
make build   # JAVA_HOME=<jdk8> mvn -B -DskipFVT=true clean package  (also runs the unit tests)
make run     # docker run Liberty with the repo's server.xml + EAR + Derby lib, then buildDBTables + buildDB
make parity TARGET=http://localhost:9083/daytrader
make down
```

`make run` mounts three things into the container:

| Host path | Container path |
| --- | --- |
| `daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml` | `/config/server.xml` |
| `daytrader3-ee6/target/daytrader3-ee6-1.0-SNAPSHOT.ear` | `/config/apps/daytrader3-ee6.ear` |
| `daytrader3-ee6-wlpcfg/shared/resources/Daytrader3_SampleDerbyLibs/` | `/opt/ibm/wlp/usr/shared/resources/Daytrader3_SampleDerbyLibs/` |

Then it calls the app's own config servlet: `GET /daytrader/config?action=buildDBTables` (about 1 s) and `GET /daytrader/config?action=buildDB` (about 65 s; 15,000 users and 10,000 quotes). No server restart is needed between the two, despite what the original README suggests.

The app is served at `http://localhost:9083/daytrader/` and the REST sample at `http://localhost:9083/rest/`.

## Smoke test

Run by hand with `curl` and a cookie jar, then automated as the parity scenarios.

| Step | Request | Result |
| --- | --- | --- |
| Login `uid:0` / `xxx` | `POST /daytrader/app action=login` | 200, home page for `uid:0` |
| Home | `GET /daytrader/app?action=home` | 200 |
| Quotes | `GET /daytrader/app?action=quotes&symbols=s:0,s:1,s:2` | 200, three quote rows |
| Buy | `GET /daytrader/app?action=buy&symbol=s:1&quantity=10` | 200, "Order N to buy 10.0 shares of s:1 has been submitted"; order status `closed` (synchronous mode) |
| Sell | `GET /daytrader/app?action=sell&holdingID=N` | 200, "Order N to sell 10.0 shares of s:1 has been submitted" |
| Portfolio after trades | `GET /daytrader/app?action=portfolio` | 200, new holding listed after the buy, gone after the sell |
| Account | `GET /daytrader/app?action=account` | 200 |
| Update profile | `POST /daytrader/app action=update_profile` | 200, new full name / address / email shown |
| Register | `POST /daytrader/app action=register` | 200, home page for the new user, opening balance as submitted |
| Logout | `GET /daytrader/app?action=logout` | 200, welcome page |
| REST list | `GET /rest/addresses` | 200, JSON list of 5 entries |
| REST search | `GET /rest/addresses/search/Entry1` | 200, one entry |

## Broken or surprising behaviour (kept as-is)

These are recorded in the parity transcripts on purpose. A migrated app that "fixes" them will show a parity diff, which is the point: each one is a decision for the migration, not something to paper over.

1. **Invalid login returns HTTP 500.** `TradeAppServlet` throws a `ServletException` and Liberty renders the DayTrader error page with a stack trace, instead of a 200 "login failed" page or a 401.
2. **Account page shows `Total Orders: 0` after trades** in the default `Full EJB3` runtime mode, even though the buy and sell orders exist and the stats page counts them. The recent-orders table is empty.
3. **`GET /rest/addresses/{entryName}` returns HTTP 415** for an existing entry. `AddressBook.getAddress` is declared as a JAX-RS sub-resource locator with no HTTP method, so JAX-RS 1.1 cannot dispatch a plain `GET` to it. An unknown entry returns 404.
4. **REST list order is not sorted.** Entries come from a `HashMap`; the order is stable on this JVM but not specified. The parity harness compares the list as a set.
5. **`RND_USER` and random quote movement.** Trades move the quote price by a random factor (`TradeConfig.getRandomPriceChangeFactor`), seeded from the clock. The harness cannot reseed the server over HTTP, so it normalises prices and checks the arithmetic instead (see `parity/NORMALIZATION.md`).

## Fixes made to get it running (build only, no app code changed)

| Problem | Fix |
| --- | --- |
| `mvn clean install` from the root left no EAR in the Liberty server directory: `daytrader3-ee6-wlpcfg` was built after the EAR module and its `clean` deleted the copied `apps/*.ear` and Derby jar. | Reordered `<modules>` in the root `pom.xml` so `daytrader3-ee6-wlpcfg` builds before `daytrader3-ee6`. |
| `maven-ear-plugin` had no version (Maven warning, version drift). | Pinned to 2.8. |
| Pulling `websphere-liberty` from Docker Hub hit `429 Too Many Requests` through the registry mirror. | Use IBM's registry image `icr.io/appcafe/websphere-liberty:full-java8-ibmjava-ubi`. |
| `.gitignore` in `daytrader3-ee6-wlpcfg` pointed at an old `daytrader7Sample` server name. | Added ignores for the generated EAR, logs and workarea under `daytrader3_Sample`. |

Not fixed, left for the migration: the committed Derby/transaction-log/messaging runtime files under `daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/` and `shared/resources/data`, the Gradle build (unchanged, not used by CI), and the FVT module path (`-DskipFVT=true`).
