# Lab: WebSphere → Spring Boot on AWS ECS Fargate

Three prompts for the live demo, run in order in a Devin session on this repo. Each assumes the previous one is merged or on the same branch.

## 1. WebSphere inventory and migration plan

```
In Cognition-Partner-Workshops/uc-java-ee-6-modernization, read AGENTS.md, docs/BASELINE.md,
docs/WEBSPHERE_INVENTORY.md, docs/TARGET_STATE.md and legacy-ops/README.md.
Run `make inventory` and confirm the inventory is current. Then fill in the
"Proposed Spring Boot / AWS replacement" column for every row (no TBD left), grouped consistently
(datasource -> spring.datasource.* on Aurora PostgreSQL, J2C alias -> Secrets Manager, MQ/JMS ->
SQS/@SqsListener or Amazon MQ, EJB -> @Service/@Transactional, JNDI lookups -> injection, Liberty
features -> starters). Do this by changing tools/ws_inventory.py (a mapping table), not by
hand-editing the generated file. Add docs/MIGRATION_PLAN.md with the ordered steps, risks and the
JSF/Thymeleaf decision. Do not claim WebSphere-proprietary API usage: there is none. Open one PR.
```

## 2. Migrate to Spring Boot 3.3 / Java 21 and pass parity

```
Migrate DayTrader to Spring Boot 3.3+ on Java 21 in a new module `daytrader-boot/`, following
docs/TARGET_STATE.md and docs/MIGRATION_PLAN.md. Keep the URLs (/daytrader/app?action=..., /rest/addresses),
port the JSPs to Thymeleaf, use Spring Data JPA on the same tables (H2 or PostgreSQL in a container
locally), configure the datasource only through spring.datasource.*, and expose /actuator/health.
Provide a seed endpoint or startup job equivalent to /config?action=buildDBTables and buildDB.
Port the JUnit 5 tests. Start the legacy app (`make up build run`) and the new app side by side and
iterate until `make parity TARGET=http://localhost:8080/daytrader` reports 13/13 PASS.
Never edit parity/transcripts or loosen parity/rules.yaml without a written reason in
parity/NORMALIZATION.md. List any deliberate behaviour changes. Open one PR with the parity report.
```

## 3. Containerize and Terraform for ECS Fargate + ALB (plan only)

```
Add a multi-stage Dockerfile for daytrader-boot (JDK 21 build, JRE 21 runtime, non-root, port 8080)
and prove the container passes `make parity TARGET=http://localhost:8080/daytrader`.
Then create Terraform under infra/terraform/{network,ecs,alb,rds}: VPC with public/private subnets,
ALB with a target group health check on /daytrader/actuator/health, ECS Fargate service and task
definition with configuration from SSM Parameter Store / environment variables and DB credentials
from Secrets Manager, and Aurora PostgreSQL. Run terraform fmt, terraform init -backend=false and
terraform validate, and terraform plan if it can run without credentials. Never run terraform
apply and do not use AWS credentials. Open one PR with the validate/plan output.
```
