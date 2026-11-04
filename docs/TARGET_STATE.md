# Target state: DayTrader on Spring Boot, AWS ECS Fargate

This is the agreed destination for the live migration demo. It is text only: none of it is built in this repository yet. The "before" estate is described in `docs/BASELINE.md`, `docs/WEBSPHERE_INVENTORY.md` and `legacy-ops/`.

## Application

| Concern | Legacy (today) | Target |
| --- | --- | --- |
| Language / runtime | Java 7 source on IBM J9 8, WebSphere Liberty (synthetic estate: tWAS 9 on AIX) | **Java 21** (Eclipse Temurin), **Spring Boot 3.3+**, embedded Tomcat |
| Packaging | EAR (`dt-ejb.jar`, `web.war`, `Rest.war`) | One executable Spring Boot jar in an OCI container image |
| Business logic | `TradeSLSBBean` (@Stateless EJB), `TradeDirect` (JDBC) | `@Service` + `@Transactional` beans. Keep both the JPA and the direct-JDBC paths only if the demo needs the runtime-mode switch. |
| Persistence | JPA 2.0 entities on `jdbc/TradeDataSource` (Derby / Db2) | Spring Data JPA (Jakarta Persistence 3.1, Hibernate 6) on **Aurora PostgreSQL**. Same tables and columns (`accountejb`, `holdingejb`, `orderejb`, `quoteejb`, `accountprofileejb`, `keygenejb`). |
| Datasource config | JNDI `jdbc/TradeDataSource`, J2C alias `TradeDataSourceAuthData` | `spring.datasource.url/username/password` (HikariCP). Credentials from **AWS Secrets Manager**, injected as ECS task secrets. |
| Messaging | MDBs `DTBroker3MDB` (`jms/TradeBrokerQueue`) and `DTStreamer3MDB` (`jms/TradeStreamerTopic`) | Broker queue → **Amazon SQS** consumed with `@SqsListener` (Spring Cloud AWS 3.x). Streamer topic → SNS topic fanned out to SQS. Use Amazon MQ (ActiveMQ) with `@JmsListener` only if the demo has to keep JMS semantics. Synchronous order processing (the default, and what parity records) does not use messaging at all. |
| Web UI | JSP + servlets (`TradeAppServlet`), some JSF 2.0 pages | **Decision: keep the server-rendered UI and port the JSPs to Thymeleaf** under the same URLs (`/daytrader/app?action=…`), with a Spring MVC controller replacing `TradeAppServlet`. The parity transcripts compare visible text and form fields, so a Thymeleaf port can match them. A REST-only boundary would need a new UI and new parity scenarios. The JSF pages are not covered by parity and are dropped (see open gaps). |
| REST sample | JAX-RS 1.1 `AddressBook` at `/rest/addresses` | Spring MVC `@RestController`. Keep `/rest/addresses` and `/rest/addresses/search/{s}`. Whether to fix the 415 on `/rest/addresses/{name}` is a conscious decision (parity records 415 today). |
| Context path | `/daytrader` (web), `/rest` (REST) | `server.servlet.context-path=/daytrader`, with REST mapped at `/rest`. Or put both behind ALB path rules and pass `--rest-target` to the parity harness. |
| Health | none | **`/actuator/health`** (Spring Boot Actuator), with liveness/readiness groups. Used by the ALB target group. |
| Configuration | `server.xml`, `TradeConfig` statics, wsadmin variables | `application.yml` defaults overridden by **environment variables**, which ECS fills from **SSM Parameter Store** (non-secret) and Secrets Manager (secret). `TradeConfig` values (runtime mode, order processing mode, max users/quotes) become `@ConfigurationProperties`. |
| Logging / ops | `SystemOut.log`, cron rotation, `ulimit` in ksh | stdout JSON logs → CloudWatch Logs (awslogs driver) with a retention policy; ulimits in the task definition |

## Infrastructure (Terraform, plan only)

```
infra/terraform/
  network/   VPC, 2+ AZ public subnets (ALB) and private subnets (tasks, DB), NAT, security groups
  alb/       Application Load Balancer, HTTPS listener, target group (ip targets) with health check on /daytrader/actuator/health
  ecs/       ECS cluster, Fargate task definition (image, CPU/mem, secrets from Secrets Manager, env from SSM), service, autoscaling, CloudWatch log group, IAM task/execution roles
  rds/       Aurora PostgreSQL cluster, subnet group, Secrets Manager-managed master credentials
```

- Each folder is a root module, or a module called from a thin root, with `variables.tf`, `outputs.tf` and no hard-coded account IDs.
- Run **`terraform init -backend=false`, `terraform validate` and `terraform plan` only. Never `terraform apply`.** The demo uses no AWS credentials. If a plan needs a provider, it runs with mock or skipped credentials (`skip_credentials_validation`), or the demo stops at `validate`.
- Container image: a multi-stage build (Maven/JDK 21 build stage, JRE 21 runtime stage), running as a non-root user, exposing 8080.

## Definition of done for the live demo

1. `make parity TARGET=<spring-boot-url>` gives 13/13 PASS against the transcripts recorded from Liberty, without editing the transcripts or `parity/rules.yaml`. Every rule change needs a written reason.
2. The unit tests are ported and still pass.
3. Every row in `docs/WEBSPHERE_INVENTORY.md` has a replacement in place of `TBD`.
4. `terraform validate` passes for `infra/terraform/{network,ecs,alb,rds}` and `terraform plan` is reviewed. Nothing is applied.

## Open gaps

- **JSF on Spring Boot.** JSF 2.0 pages exist (`web/jsf/*`). Spring Boot has no first-party JSF support (JoinFaces is third-party). The decision above drops them. Confirm that is acceptable for the audience.
- **IBM MQ.** The MQ resources are synthetic (`legacy-ops/`). Nothing has been tested against a real queue manager, and the IBM MQ developer image license (development use only) was not reviewed. See `legacy-ops/README.md`.
- **Db2 → PostgreSQL.** The legacy DDL is Derby/Db2 dialect (`daytrader3-ee6-web/src/main/webapp/dbscripts/`). Schema conversion and the `KEYGENEJB` key-block allocator need a PostgreSQL equivalent (sequences, or keep the table).
