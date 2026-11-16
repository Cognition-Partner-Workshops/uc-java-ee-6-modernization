#!/usr/bin/env python3
"""Scan the repository for WebSphere/Java EE container dependencies and write docs/WEBSPHERE_INVENTORY.md.

Sources: ibm-*.xml bindings/extensions, Java EE descriptors, Java annotations and JNDI lookups,
EJBs and MDBs, Liberty server.xml features and resources, com.ibm.* imports, and the synthetic
wsadmin scripts in legacy-ops/. The replacement column (target: Go + PostgreSQL on AWS) comes from REPLACEMENT_RULES.
Run: python3 tools/ws_inventory.py [--check]
"""
import argparse
import os
import re
import sys
import xml.etree.ElementTree as ET

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUTPUT = os.path.join(REPO, "docs", "WEBSPHERE_INVENTORY.md")
SKIP_DIRS = {".git", "target", "build", ".venv", "node_modules", "work", "__pycache__", ".gradle"}
PROJECT_PACKAGE = "com.ibm.websphere.samples.daytrader"


def repoFiles():
    for root, dirs, files in os.walk(REPO):
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        # the committed copy of application.xml under the Liberty apps dir is a build output
        for name in sorted(files):
            yield os.path.relpath(os.path.join(root, name), REPO)


def local(tag):
    return tag.split("}", 1)[-1]


def parseXml(path):
    try:
        return ET.parse(os.path.join(REPO, path)).getroot()
    except ET.ParseError as exc:
        sys.exit(f"cannot parse {path}: {exc}")


def text(element, childName):
    for child in element:
        if local(child.tag) == childName:
            return (child.text or "").strip()
    return ""


class Inventory:
    def __init__(self):
        self.rows = []

    def add(self, category, item, source, detail=""):
        row = (category, item, source, detail)
        if row not in self.rows:
            self.rows.append(row)


def scanIbmXml(inv, path):
    root = parseXml(path)
    inv.add("IBM binding/extension file", os.path.basename(path), path, f"root <{local(root.tag)}>")
    owner = ""
    for element in root.iter():
        tag = local(element.tag)
        attrs = element.attrib
        if tag in ("session", "message-driven"):
            owner = attrs.get("name", "")
        name = attrs.get("name", "")
        binding = attrs.get("binding-name", "")
        if tag in ("resource-ref", "message-destination-ref", "resource-env-ref", "ejb-ref", "message-destination"):
            scoped = owner and "ejb-jar" in path and tag != "message-destination"
            detail = f"<{tag}>" + (f" in {owner}" if scoped else "")
            if binding:
                detail += f" -> binding {binding}"
            if "isolation-level" in attrs:
                detail += f", isolation {attrs['isolation-level']}"
            inv.add("IBM binding", name, path, detail)
        elif tag == "authentication-alias":
            inv.add("J2C auth alias reference", name, path, "<authentication-alias>")
        elif tag == "jca-adapter":
            inv.add("MDB activation binding", owner, path,
                    f"activation spec {attrs.get('activation-spec-binding-name')} -> destination "
                    f"{attrs.get('destination-binding-name')}, auth alias {attrs.get('activation-spec-auth-alias')}")
        elif tag == "virtual-host":
            inv.add("IBM binding", name, path, "<virtual-host>")
        elif tag in ("jsp-attribute", "file-serving-attribute", "reload-interval", "enable-serving-servlets-by-class-name"):
            inv.add("IBM extension setting", name or tag, path, f"<{tag}> {attrs.get('value', '')}".strip())


def scanDescriptor(inv, path):
    root = parseXml(path)
    kind = os.path.basename(path)
    for element in root.iter():
        tag = local(element.tag)
        if tag in ("resource-ref", "resource-env-ref", "message-destination-ref"):
            name = text(element, "res-ref-name") or text(element, "resource-env-ref-name") \
                or text(element, "message-destination-ref-name")
            kind_ = text(element, "res-type") or text(element, "resource-env-ref-type") \
                or text(element, "message-destination-type")
            inv.add("JNDI reference", name, path, f"<{tag}> {kind_}".strip())
        elif tag == "ejb-local-ref" or tag == "ejb-ref":
            inv.add("JNDI reference", text(element, "ejb-ref-name"), path, f"<{tag}> {text(element, 'local')}".strip())
        elif tag in ("session", "message-driven") and kind == "ejb-jar.xml":
            category = "MDB" if tag == "message-driven" else "EJB"
            inv.add(category, text(element, "ejb-name"), path,
                    f"ejb-jar.xml {text(element, 'session-type') or tag} {text(element, 'ejb-class')}".strip())
        elif tag == "persistence-unit":
            inv.add("JPA persistence unit", element.attrib.get("name", ""), path,
                    f"jta {text(element, 'jta-data-source')}, non-jta {text(element, 'non-jta-data-source')}")
        elif tag == "message-destination" and kind == "ejb-jar.xml":
            inv.add("MDB destination", text(element, "message-destination-name"), path, "ejb-jar.xml <message-destination>")
        elif tag == "web" and kind == "application.xml":
            inv.add("EAR module", text(element, "web-uri"), path, f"context root {text(element, 'context-root')}")
        elif tag == "ejb" and kind == "application.xml":
            inv.add("EAR module", (element.text or "").strip(), path, "EJB module")
        elif tag == "security-constraint":
            inv.add("Web security constraint", text(element, "display-name") or "security-constraint", path, "web.xml")


def scanServerXml(inv, path):
    root = parseXml(path)
    for element in root.iter():
        tag = local(element.tag)
        attrs = element.attrib
        if tag == "feature":
            inv.add("Liberty feature", (element.text or "").strip(), path, "<featureManager>")
        elif tag in ("dataSource", "jmsQueueConnectionFactory", "jmsTopicConnectionFactory", "jmsQueue", "jmsTopic",
                     "jmsConnectionFactory"):
            inv.add("JNDI binding (Liberty)", attrs.get("jndiName", attrs.get("id", "")), path, f"<{tag}>")
        elif tag == "jmsActivationSpec":
            props = next((c.attrib for c in element if local(c.tag).startswith("properties")), {})
            inv.add("JNDI binding (Liberty)", attrs.get("id", ""), path,
                    f"<jmsActivationSpec> destination {props.get('destinationRef', '')}")
        elif tag == "authData":
            inv.add("J2C auth alias (Liberty)", attrs.get("id", ""), path, "<authData>")
        elif tag in ("library", "messagingEngine", "application", "httpEndpoint", "transaction"):
            inv.add("Liberty server config", attrs.get("id", attrs.get("name", tag)), path, f"<{tag}>")


ANNOTATION = re.compile(r"@(Stateless|Stateful|Singleton|MessageDriven)\b")
CLASS = re.compile(r"\bclass\s+(\w+)")
RESOURCE = re.compile(r"@Resource\s*\(([^)]*)\)")
NAME_ATTR = re.compile(r"\b(?:name|mappedName|lookup)\s*=\s*\"([^\"]+)\"")
LOOKUP = re.compile(r"\.lookup\(\s*\"([^\"]+)\"")
ACTIVATION = re.compile(r"propertyName\s*=\s*\"(destination|destinationType)\"\s*,\s*propertyValue\s*=\s*\"([^\"]+)\"")
IMPORT = re.compile(r"^import\s+(?:static\s+)?(com\.ibm\.[\w.*]+)\s*;", re.M)
CONSTANT_JNDI = re.compile(r"\"(java:comp/[^\"]+|jdbc/[^\"]+|jms/[^\"]+|eis/[^\"]+)\"")


def scanJava(inv, path, imports):
    source = open(os.path.join(REPO, path), encoding="utf-8", errors="replace").read()
    className = (CLASS.search(source) or [None, os.path.basename(path)[:-5]])[1]
    for match in ANNOTATION.finditer(source):
        kind = match.group(1)
        if kind == "MessageDriven":
            props = dict(ACTIVATION.findall(source))
            inv.add("MDB", className, path, f"@MessageDriven destination {props.get('destination', '?')} "
                                             f"({props.get('destinationType', '?')})")
            if props.get("destination"):
                inv.add("MDB destination", props["destination"], path, f"consumed by {className}")
        else:
            inv.add("EJB", className, path, f"@{kind}")
    for match in RESOURCE.finditer(source):
        names = NAME_ATTR.findall(match.group(1))
        if names:
            inv.add("JNDI reference", names[0], path, f"@Resource in {className}")
    for name in LOOKUP.findall(source):
        inv.add("JNDI lookup", name, path, f"InitialContext.lookup in {className}")
    for name in CONSTANT_JNDI.findall(source):
        inv.add("JNDI name constant", name, path, f"string literal in {className}")
    for name in IMPORT.findall(source):
        imports.setdefault(name, set()).add(path)


WSADMIN_CALL = re.compile(r"(AdminTask|AdminConfig|AdminApp)\.(\w+)\(")
OPTION = re.compile(r"-(name|jndiName|alias|type|destinationJndiName|queueName|topicName|variableName)\s+\"?([^\s\"\]]+)")


def scanWsadmin(inv, path):
    source = open(os.path.join(REPO, path), encoding="utf-8").read()
    # join each statement (calls can span several lines) up to the next top-level call
    for match in WSADMIN_CALL.finditer(source):
        obj, verb = match.groups()
        if verb in ("save", "getid", "showAttribute", "modify", "queryNames", "invoke"):
            continue
        end = source.find("\n\n", match.end())
        statement = source[match.start(): end if end != -1 else len(source)]
        options = dict((k, v) for k, v in OPTION.findall(statement))
        if obj == "AdminApp" and verb == "install":
            inv.add("wsadmin resource", "daytrader3", path, "AdminApp.install (resource-ref, MDB and shared-library mappings)")
            continue
        if verb == "create":
            lib = re.search(r"\['name',\s*'([^']+)'\]", statement)
            inv.add("wsadmin resource", lib.group(1) if lib else "?", path, "AdminConfig.create Library (shared library)")
            continue
        item = options.get("jndiName") or options.get("alias") or options.get("name") or options.get("variableName") or "?"
        detail = f"{obj}.{verb}"
        for key in ("type", "destinationJndiName", "queueName", "topicName"):
            if key in options:
                detail += f" {key}={options[key]}"
        inv.add("wsadmin resource", item, path, detail)


def build():
    inv, imports = Inventory(), {}
    for path in repoFiles():
        name = os.path.basename(path)
        if "/apps/META-INF/" in path:
            continue
        if re.match(r"ibm-.*\.xml$", name):
            scanIbmXml(inv, path)
        elif name in ("web.xml", "ejb-jar.xml", "persistence.xml", "application.xml"):
            scanDescriptor(inv, path)
        elif name.startswith("server") and name.endswith(".xml") and "wlpcfg" in path:
            scanServerXml(inv, path)
        elif name.endswith(".java") and "/src/main/" in path:
            scanJava(inv, path, imports)
        elif path.startswith("legacy-ops/wsadmin/") and name.endswith(".py"):
            scanWsadmin(inv, path)
    for name, paths in sorted(imports.items()):
        note = "project package (not a WebSphere API)" if name.startswith(PROJECT_PACKAGE) else "IBM API"
        inv.add("com.ibm.* import", name, f"{len(paths)} file(s)", note)
    return inv.rows


# --- Replacement mapping (target: Go + PostgreSQL on AWS ECS Fargate) -------------------------------
# Ordered rules: the first rule whose category matches and whose regex is found in "item detail"
# wins. See docs/MIGRATION_PLAN.md for the reasoning behind each group.

DATASOURCE = "pgx pool (`pgxpool.Pool`) on Aurora PostgreSQL, built from `DATABASE_URL` and passed to services by constructor"
SECRET_DB = "AWS Secrets Manager secret (Aurora master credentials), injected as an ECS task secret into `DATABASE_URL`"
SECRET_MQ = "No stored credential: the ECS task role's IAM policy grants SQS access"
SQS_QUEUE = "Amazon SQS queue `trade-broker`, consumed by the Go order consumer (`ORDER_PROCESSING_MODE=async`; default is sync, no queue)"
SQS_TOPIC = "Amazon SNS topic `trade-streamer` fanned out to SQS; quote-price publishing is off by default (`PUBLISH_QUOTE_PRICE_CHANGE=false`)"
SQS_CLIENT = "AWS SDK for Go v2 SQS client, built in `main` and passed by constructor; unused in the default sync mode"
SERVICE = "Go trade service (`daytrader-go/internal/app`) with explicit `pgx.Tx` transactions (`BEGIN`/`COMMIT`/`ROLLBACK`)"
INJECTION = "Constructor injection: the Go trade service is built in `main` and passed to the handlers; no JNDI"
DROPPED_PRIMS = "Not migrated: the `web/prims` ping servlets are benchmark primitives outside the parity scope"
PROJECT_PKG = "Go package under `daytrader-go/internal/` (same domain type, no WebSphere API involved)"

REPLACEMENT_RULES = [
    # category, regex on "item detail source", replacement
    ("IBM binding/extension file", r"ibm-ejb-jar-bnd", "Removed: no EJB container. Resources are wired explicitly in `daytrader-go/cmd/daytrader/main.go`"),
    ("IBM binding/extension file", r"ibm-web-bnd", "Removed: no resource-ref bindings. Context path `/daytrader` is a route prefix in the Go `net/http` mux"),
    ("IBM binding/extension file", r"ibm-web-ext", "Removed: templates are embedded with `embed.FS` and parsed once at start-up"),
    ("IBM binding", r"virtual-host", "ALB listener + target group; the Go server listens on `:8080`"),
    ("IBM binding", r"jdbc/", DATASOURCE + "; isolation is PostgreSQL's default READ COMMITTED"),
    ("IBM binding", r"mdc", "Removed: no message-driven context. The SQS consumer receives a `context.Context`"),
    ("IBM binding", r"Topic(?!Conn)", SQS_TOPIC),
    ("IBM binding", r"ConnectionFactory|QCF|TCF", SQS_CLIENT),
    ("IBM binding", r"Queue", SQS_QUEUE),
    ("IBM extension setting", r"", "Removed: Go templates are compiled into the binary, there is no JSP reload"),
    ("J2C auth alias reference", r"TradeDataSourceAuthData", SECRET_DB),
    ("J2C auth alias reference", r"TradeAdminAuthData", SECRET_MQ),
    ("J2C auth alias (Liberty)", r"TradeDataSourceAuthData", SECRET_DB),
    ("J2C auth alias (Liberty)", r"TradeAdminAuthData", SECRET_MQ),
    ("MDB activation binding", r"DTBroker3MDB", "SQS long-poll consumer goroutine on `trade-broker` (behind `ORDER_PROCESSING_MODE=async`, off by default); access by IAM task role"),
    ("MDB activation binding", r"DTStreamer3MDB", "SQS consumer on the `trade-streamer` SNS fan-out queue (off by default); access by IAM task role"),
    ("JNDI reference", r"ejb/local", INJECTION),
    ("JNDI reference", r"jdbc/", DATASOURCE),
    ("JNDI reference", r"Topic(?!Conn)", SQS_TOPIC),
    ("JNDI reference", r"ConnectionFactory", SQS_CLIENT),
    ("JNDI reference", r"Queue", SQS_QUEUE),
    ("JNDI lookup", r"PingServlet", DROPPED_PRIMS),
    ("JNDI lookup", r"UserTransaction", "Explicit `pgx.Tx` (`pool.Begin` / `Commit` / `Rollback`) in the Go trade service"),
    ("JNDI lookup", r"SLSBBean", INJECTION),
    ("JNDI lookup", r"Topic(?!Conn)", SQS_TOPIC),
    ("JNDI lookup", r"ConnectionFactory", SQS_CLIENT),
    ("JNDI lookup", r"Queue", SQS_QUEUE),
    ("JNDI name constant", r"PingServlet", DROPPED_PRIMS),
    ("JNDI name constant", r"UserTransaction", "Explicit `pgx.Tx` (`pool.Begin` / `Commit` / `Rollback`) in the Go trade service"),
    ("JNDI name constant", r"jdbc/", "`DATABASE_URL` environment variable read by `internal/config`; " + DATASOURCE),
    ("JNDI name constant", r"SLSBBean", INJECTION),
    ("JNDI name constant", r"Topic(?!Conn)", SQS_TOPIC),
    ("JNDI name constant", r"ConnectionFactory", SQS_CLIENT),
    ("JNDI name constant", r"Queue", SQS_QUEUE),
    ("JPA persistence unit", r"", "SQL through pgx on one `pgxpool.Pool`; schema in `daytrader-go/migrations/*.sql` (same six tables). No ORM, no JTA/non-JTA split"),
    ("EAR module", r"Rest\.war", "Same Go binary: routes `/rest/addresses...` on the `net/http` mux"),
    ("EAR module", r"dt-ejb", "Go packages `daytrader-go/internal/{app,store,config}` linked into the one binary"),
    ("EAR module", r"web\.war", "Same Go binary: `/daytrader` routes, `html/template` pages and static files embedded with `embed.FS`"),
    ("Web security constraint", r"", "None in the legacy app to carry over; TLS terminates at the ALB"),
    ("EJB", r"DirectSLSBBean", SERVICE + ". The EJB3/Direct runtime-mode switch collapses to this one implementation"),
    ("EJB", r"TradeSLSBBean", SERVICE),
    ("MDB", r"DTBroker3MDB", "Go SQS consumer for order completion, behind `ORDER_PROCESSING_MODE=async` (off; sync is the default and what parity records)"),
    ("MDB", r"DTStreamer3MDB", "Go SQS consumer for quote-price messages (SNS fan-out), off by default; it only fed benchmark statistics"),
    ("MDB destination", r"Topic", SQS_TOPIC),
    ("MDB destination", r"Queue", SQS_QUEUE),
    ("Liberty feature", r"ejbLite", "Go services + explicit transactions with `github.com/jackc/pgx/v5`"),
    ("Liberty feature", r"jaxrs", "Go standard library `net/http` + `encoding/json`"),
    ("Liberty feature", r"jmsMdb", "`github.com/aws/aws-sdk-go-v2/service/sqs` consumer loop (async mode only)"),
    ("Liberty feature", r"jpa", "`github.com/jackc/pgx/v5` + `pgxpool`, hand-written SQL"),
    ("Liberty feature", r"jsf", "Dropped: the JSF pages are outside the parity scope. The JSP pages are ported to `html/template`"),
    ("Liberty feature", r"wasJmsClient", "`github.com/aws/aws-sdk-go-v2/service/sqs` (async mode only)"),
    ("Liberty feature", r"wasJmsServer", "Amazon SQS / SNS managed service; no embedded messaging engine"),
    ("JNDI binding (Liberty)", r"<dataSource>", DATASOURCE + ". One pool replaces the XA and non-transactional datasources"),
    ("JNDI binding (Liberty)", r"jmsActivationSpec.*Topic", "SQS consumer on the `trade-streamer` SNS fan-out queue (off by default)"),
    ("JNDI binding (Liberty)", r"jmsActivationSpec", "SQS long-poll consumer on `trade-broker` (`ORDER_PROCESSING_MODE=async`, off by default)"),
    ("JNDI binding (Liberty)", r"ConnectionFactory", SQS_CLIENT),
    ("JNDI binding (Liberty)", r"jmsTopic", SQS_TOPIC),
    ("JNDI binding (Liberty)", r"jmsQueue", SQS_QUEUE),
    ("Liberty server config", r"<library>", "`github.com/jackc/pgx/v5` in `go.mod`; no JDBC driver jar or shared library"),
    ("Liberty server config", r"<application>", "One statically linked Go binary in a non-root container image (ECS Fargate task)"),
    ("Liberty server config", r"<httpEndpoint>", "`net/http` server on `:8080` behind the ALB (health check `/daytrader/health`)"),
    ("Liberty server config", r"<messagingEngine>", "Amazon SQS / SNS; nothing embedded"),
    ("com.ibm.* import", r"TradeConfig", "`daytrader-go/internal/config` (TradeConfig values from environment variables / SSM)"),
    ("com.ibm.* import", r"ejb3\.|direct\.|TradeAction|TradeServices", "Go trade service in `daytrader-go/internal/app` (one implementation, constructor-injected)"),
    ("com.ibm.* import", r"FinancialUtils", "`daytrader-go/internal/javafmt` (same rounding and HTML fragments)"),
    ("com.ibm.* import", r"MDBStats|TimerStat|RunStatsDataBean", "Not migrated: benchmark statistics only"),
    ("com.ibm.* import", r"util\.Log", "Go standard library `log/slog`, JSON to stdout -> CloudWatch Logs"),
    ("com.ibm.* import", r"DataBean", "Go struct in `daytrader-go/internal/store` (same table and columns)"),
    ("com.ibm.* import", r"", PROJECT_PKG),
    ("wsadmin resource", r"createJDBCProvider|setVariable", "Removed: pgx is compiled in. Terraform `infra/terraform/rds` provisions Aurora PostgreSQL (synthetic tWAS resource)"),
    ("wsadmin resource", r"createAuthDataEntry", SECRET_MQ + " (synthetic tWAS resource)"),
    ("wsadmin resource", r"createDatasource", DATASOURCE + "; pool size from `DB_MAX_CONNS` (SSM). Synthetic tWAS resource"),
    ("wsadmin resource", r"ActivationSpec", "SQS consumer on the `trade-streamer` fan-out queue, off by default (synthetic tWAS / IBM MQ resource)"),
    ("wsadmin resource", r"ConnectionFactory", SQS_CLIENT + " (synthetic tWAS / IBM MQ resource)"),
    ("wsadmin resource", r"createWMQQueue", SQS_QUEUE + " (synthetic tWAS / IBM MQ resource)"),
    ("wsadmin resource", r"createWMQTopic", SQS_TOPIC + " (synthetic tWAS / IBM MQ resource)"),
    ("wsadmin resource", r"Library", "Removed: dependencies are in `go.mod` and linked into the binary (synthetic tWAS resource)"),
    ("wsadmin resource", r"AdminApp\.install", "Container image + ECS Fargate service/task definition in `infra/terraform/ecs`; routes replace context roots (synthetic tWAS resource)"),
]


def replacementFor(category, item, source, detail):
    haystack = f"{item} {detail}"
    for ruleCategory, pattern, replacement in REPLACEMENT_RULES:
        if ruleCategory == category and re.search(pattern, haystack):
            return replacement
    return "TBD"


ORDER = ["IBM binding/extension file", "IBM binding", "IBM extension setting", "J2C auth alias reference",
         "MDB activation binding", "JNDI reference", "JNDI lookup", "JNDI name constant", "JPA persistence unit",
         "EAR module", "Web security constraint", "EJB", "MDB", "MDB destination", "Liberty feature",
         "JNDI binding (Liberty)", "J2C auth alias (Liberty)", "Liberty server config", "com.ibm.* import",
         "wsadmin resource"]


def render(rows):
    rows = sorted(rows, key=lambda r: (ORDER.index(r[0]) if r[0] in ORDER else 99, r[2], r[1], r[3]))
    counts = {}
    for row in rows:
        counts[row[0]] = counts.get(row[0], 0) + 1
    ibmApis = sum(1 for r in rows if r[0] == "com.ibm.* import" and r[3] == "IBM API")
    unmapped = sum(1 for r in rows if replacementFor(*r) == "TBD")
    out = [
        "# WebSphere dependency inventory",
        "",
        "<!-- Generated by tools/ws_inventory.py. Do not edit by hand; run `make inventory`. -->",
        "",
        f"**{len(rows)} rows, {unmapped} without a replacement.** The replacement column targets **Go + PostgreSQL on AWS ECS Fargate** "
        "(mapping table: `REPLACEMENT_RULES` in `tools/ws_inventory.py`; reasoning: `docs/MIGRATION_PLAN.md`).",
        "",
        f"- `com.ibm.*` imports that are IBM APIs (outside the `{PROJECT_PACKAGE}` package): **{ibmApis}**. "
        "The app's own package is named `com.ibm.websphere.*`; that is a name, not a dependency on WebSphere-proprietary APIs.",
        "- `legacy-ops/wsadmin` rows come from **synthetic** tWAS scripts (see `legacy-ops/README.md`). "
        "They describe the estate the migration must translate, not this repo's Liberty runtime.",
        "",
        "| Category | Rows |",
        "| --- | --- |",
        *[f"| {c} | {counts[c]} |" for c in ORDER if c in counts],
        "",
        "| # | Category | Item | Source | Detail | Proposed Go / AWS replacement |",
        "| --- | --- | --- | --- | --- | --- |",
    ]
    for i, (category, item, source, detail) in enumerate(rows, 1):
        cells = [str(i), category, f"`{item}`", f"`{source}`" if "/" in source else source, detail,
                 replacementFor(category, item, source, detail)]
        out.append("| " + " | ".join(c.replace("|", "\\|") for c in cells) + " |")
    return "\n".join(out) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail if docs/WEBSPHERE_INVENTORY.md is out of date")
    args = parser.parse_args()
    content = render(build())
    if args.check:
        current = open(OUTPUT).read() if os.path.exists(OUTPUT) else ""
        if current != content:
            sys.exit("docs/WEBSPHERE_INVENTORY.md is stale: run make inventory")
        print("inventory up to date")
        return
    with open(OUTPUT, "w") as handle:
        handle.write(content)
    print(f"wrote {os.path.relpath(OUTPUT)} ({content.count(chr(10)) } lines)")


if __name__ == "__main__":
    main()
