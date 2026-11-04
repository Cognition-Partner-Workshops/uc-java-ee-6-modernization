# AGENTS.md

DayTrader 3 (Java EE 6), the "before" estate for a WebSphere → Spring Boot on AWS ECS Fargate modernization. The app really runs on **WebSphere Liberty** (`daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml`). The traditional WebSphere-on-AIX material in `legacy-ops/` is **synthetic**.

## Commands

```bash
make up                    # pull the Liberty image, create .venv with parity deps (parity/requirements.txt)
make build                 # JDK 8 + Maven: clean package, includes the JUnit 5 unit tests
make test                  # unit tests only (daytrader3-ee6-ejb)
make run                   # Liberty container on :9083 with the repo's server.xml, then build + seed Derby
make parity TARGET=http://localhost:9083/daytrader   # replay 13 scenarios, writes work/parity/report.md, non-zero on any diff
make record                # re-record parity/transcripts from the LIBERTY BASELINE ONLY
make inventory             # regenerate docs/WEBSPHERE_INVENTORY.md (CI fails if it is stale)
make down
```

- Build with JDK 8 (the POMs target Java 7 via `net.wasdev.maven.parent:java7-parent`). The Makefile picks up `/usr/lib/jvm/java-8-openjdk-amd64` by itself.
- App: `http://localhost:9083/daytrader/`. REST: `http://localhost:9083/rest/addresses`. Seeded login: `uid:0` / `xxx`.
- Seeding (`buildDB`) takes about 1–2 minutes.

## Rules

- **Do not edit `parity/transcripts/` to make parity pass.** They are recorded from the Liberty baseline. A diff means the target behaves differently: fix the target, or write down a deliberate decision. Re-record only from the Liberty baseline (`make run && make record`) and only when the harness itself changes.
- Changes to `parity/rules.yaml` (normalisation) need a reason in `parity/NORMALIZATION.md`.
- **No WebSphere-proprietary APIs exist in this code. Don't claim otherwise.** `com.ibm.websphere.samples.daytrader` is just the package name. The WebSphere coupling is in the deployment descriptors (`ibm-*.xml`), the JNDI names, EJBs/MDBs and the Liberty features. See `docs/WEBSPHERE_INVENTORY.md`.
- `legacy-ops/` is synthetic and never executed. Don't present it as this app's runtime.
- Don't change application behaviour in the legacy modules. The known quirks (invalid login → HTTP 500, `Total Orders: 0`, REST 415) are recorded on purpose. See `docs/BASELINE.md`.
- Infrastructure: `terraform validate` / `plan` only, **never `terraform apply`**, and no cloud credentials.
- Keep the IBM Apache-2.0 license headers on existing files.

## Map

| Path | What |
| --- | --- |
| `docs/BASELINE.md` | Versions, commands, smoke results, known quirks |
| `docs/WEBSPHERE_INVENTORY.md` | Generated inventory (`tools/ws_inventory.py`) |
| `docs/TARGET_STATE.md` | Spring Boot 3.3 / Java 21 / ECS Fargate target and Terraform layout |
| `docs/LAB.md` | Prompts for the three live-demo steps |
| `parity/` | Harness: `record.py`, `compare.py`, `scenarios.py`, `rules.yaml`, transcripts |
| `legacy-ops/` | Synthetic tWAS wsadmin + AIX ksh artefacts |
| `.agents/skills/daytrader-parity/` | How to run and interpret parity |
