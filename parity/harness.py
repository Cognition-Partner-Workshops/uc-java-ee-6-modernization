"""Shared engine for parity/record.py and parity/compare.py.

A scenario is a list of HTTP steps run in one cookie session. Each step becomes a normalised
transcript entry; some scenarios also compute "checks" (business arithmetic on raw values) so
that numbers masked by the normalisation rules are still verified.
"""
import json
import os
import random
import re
import time
import uuid
from decimal import Decimal
from urllib.parse import urlsplit

import requests
import yaml
from bs4 import BeautifulSoup, Comment

HERE = os.path.dirname(os.path.abspath(__file__))
TRANSCRIPT_DIR = os.path.join(HERE, "transcripts")
LOGIN_USER, LOGIN_PASSWORD = "uid:0", "xxx"
ORDER_FEE = Decimal("24.95")
OPENING_BALANCE = "100000"


def loadRules():
    with open(os.path.join(HERE, "rules.yaml")) as handle:
        rules = yaml.safe_load(handle)
    for rule in rules["textRules"]:
        rule["regex"] = re.compile(rule["pattern"])
    return rules


RULES = loadRules()


def normalise(text, scopes=()):
    for rule in RULES["textRules"]:
        if rule.get("scope") and rule["scope"] not in scopes:
            continue
        text = rule["regex"].sub(rule["replace"], text)
    return text


def money(text):
    return Decimal(re.sub(r"[^\d.\-]", "", text))


def parseHtml(body):
    soup = BeautifulSoup(body, "html.parser")
    for node in soup(["script", "style"]):
        node.decompose()
    for comment in soup.find_all(string=lambda s: isinstance(s, Comment)):
        comment.extract()
    return soup


def tableRows(soup):
    """Leaf tables as lists of rows of cell text."""
    tables = []
    for table in soup.find_all("table"):
        if table.find("table"):
            continue
        rows = []
        for row in table.find_all("tr"):
            cells = [re.sub(r"\s+", " ", c.get_text(" ", strip=True)) for c in row.find_all(["td", "th"])]
            if any(cells):
                rows.append(cells)
        if rows:
            tables.append(rows)
    return tables


def findTable(soup, firstHeader):
    for rows in tableRows(soup):
        if rows[0] and rows[0][0] == firstHeader:
            header = rows[0]
            return [dict(zip(header, row)) for row in rows[1:] if len(row) == len(header)]
    return []


def labelValue(soup, label):
    """Value cell that follows a 'label:' cell (account page layout)."""
    for rows in tableRows(soup):
        for row in rows:
            for i, cell in enumerate(row[:-1]):
                if cell == label:
                    return row[i + 1]
    return None


def visibleLines(soup):
    for table in soup.find_all("table"):
        rows = table.find_all("tr")
        if not rows:
            continue
        header = [c.get_text(" ", strip=True) for c in rows[0].find_all(["td", "th"])]
        if header in RULES["dropTablesWithHeader"]:
            table.decompose()
    text = soup.get_text("\n")
    return [re.sub(r"\s+", " ", line).strip() for line in text.split("\n") if line.strip()]


def formInputs(soup):
    inputs = {}
    for field in soup.find_all("input"):
        name = field.get("name")
        if not name or (field.get("type") or "text").lower() in ("submit", "image", "button"):
            continue
        inputs[name] = field.get("value", "")
    return inputs


class Runner:
    def __init__(self, baseUrl, restUrl, seed):
        self.baseUrl = baseUrl.rstrip("/")
        self.restUrl = restUrl.rstrip("/")
        self.seed = seed
        self.runToken = uuid.uuid4().hex[:8]
        self.userCount = 0

    def newUserId(self):
        self.userCount += 1
        return f"ru:p{self.seed}-{self.runToken}-{self.userCount}"

    # --- step execution -------------------------------------------------------------------------

    def step(self, ctx, name, method, path, params=None, data=None, headers=None, rest=False,
             mode="page", phrases=(), volatileAccount=False):
        url = (self.restUrl if rest else self.baseUrl) + path
        response = ctx["session"].request(method, url, params=params, data=data, headers=headers,
                                          timeout=120, allow_redirects=True)
        scopes = ("volatileAccount",) if volatileAccount else ()
        shown = dict(params or {}, **(data or {}))
        entry = {
            "step": name,
            "request": normalise(f"{method} {'/rest' if rest else ''}{path} "
                                 + json.dumps(shown, sort_keys=True), scopes),
            "status": response.status_code,
            "contentType": (response.headers.get("Content-Type") or "").split(";")[0].strip().lower(),
        }
        if mode == "page":
            soup = parseHtml(response.text)
            entry["title"] = normalise(soup.title.get_text(strip=True), scopes) if soup.title else None
            entry["text"] = [normalise(line, scopes) for line in visibleLines(parseHtml(response.text))]
            entry["inputs"] = {k: normalise(v, scopes) for k, v in sorted(formInputs(soup).items())}
        elif mode == "phrases":
            entry["containsPhrases"] = {p: (p in response.text) for p in phrases}
        elif mode == "json":
            entry["json"] = normaliseJson(response.json())
        # mode == "status": status and content type only
        ctx["steps"].append(entry)
        return response

    def login(self, ctx, user, password, name="login", volatileAccount=False):
        return self.step(ctx, name, "POST", "/app", data={"action": "login", "uid": user, "passwd": password},
                         volatileAccount=volatileAccount)

    def register(self, ctx, user, money=OPENING_BALANCE, name="register"):
        return self.step(ctx, name, "POST", "/app", data={
            "action": "register", "user id": user, "passwd": "pw", "confirm passwd": "pw",
            "Full Name": "Parity Tester", "Credit Card Number": "1111-2222-3333-4444",
            "money": money, "email": "parity@example.com", "snail mail": "1 Test Street"})

    def account(self, ctx, name="account"):
        soup = parseHtml(self.step(ctx, name, "GET", "/app", params={"action": "account"}).text)
        return {
            "cash": money(labelValue(soup, "cash balance:")),
            "opening": money(labelValue(soup, "opening balance:")),
            "inputs": formInputs(soup),
        }

    def quotePrice(self, ctx, symbol, name="quote"):
        soup = parseHtml(self.step(ctx, name, "GET", "/app", params={"action": "quotes", "symbols": symbol}).text)
        rows = findTable(soup, "symbol")
        return money(rows[0]["current price"])

    def portfolio(self, ctx, name="portfolio"):
        soup = parseHtml(self.step(ctx, name, "GET", "/app", params={"action": "portfolio"}).text)
        return [row for row in findTable(soup, "holding ID") if row.get("holding ID", "").isdigit()]

    def buy(self, ctx, symbol, quantity, name="buy"):
        soup = parseHtml(self.step(ctx, name, "GET", "/app",
                                   params={"action": "buy", "symbol": symbol, "quantity": str(quantity)}).text)
        return findTable(soup, "order ID")

    def sell(self, ctx, holdingId, name="sell"):
        soup = parseHtml(self.step(ctx, name, "GET", "/app",
                                   params={"action": "sell", "holdingID": holdingId}).text)
        return findTable(soup, "order ID")


def normaliseJson(value):
    if isinstance(value, dict):
        return {k: normaliseJson(v) for k, v in sorted(value.items())}
    if isinstance(value, list):
        items = [normaliseJson(v) for v in value]
        return sorted(items, key=lambda item: json.dumps(item, sort_keys=True))
    if isinstance(value, str):
        return normalise(value)
    return value


def primeUid0(runner):
    """Unrecorded uid:0 login + home + logout (rule P1 in NORMALIZATION.md).

    buildDB creates uid:0's holdings through buy orders that stay queued as "completed order"
    alerts until uid:0's next page view, so only the first login after a seed would show them.
    """
    session = requests.Session()
    session.post(runner.baseUrl + "/app", data={"action": "login", "uid": LOGIN_USER, "passwd": LOGIN_PASSWORD},
                 timeout=120)
    session.get(runner.baseUrl + "/app", params={"action": "home"}, timeout=120)
    session.get(runner.baseUrl + "/app", params={"action": "logout"}, timeout=120)


def runScenario(runner, scenario):
    ctx = {"session": requests.Session(), "steps": [], "checks": {},
           "rng": random.Random("%s:%s" % (runner.seed, scenario["id"]))}
    started = time.time()
    error = None
    try:
        if scenario.get("primeUid0"):
            primeUid0(runner)
        scenario["run"](runner, ctx)
    except Exception as exc:  # a crash is a parity failure, recorded in the transcript
        error = f"{type(exc).__name__}: {exc}"
    transcript = {
        "scenario": scenario["id"],
        "description": scenario["description"],
        "steps": ctx["steps"],
        "checks": ctx["checks"],
    }
    if error:
        transcript["error"] = normalise(error)
    return transcript, time.time() - started


def restBaseFor(baseUrl):
    parts = urlsplit(baseUrl)
    return f"{parts.scheme}://{parts.netloc}/rest"


def writeTranscript(directory, transcript):
    os.makedirs(directory, exist_ok=True)
    path = os.path.join(directory, transcript["scenario"] + ".json")
    with open(path, "w") as handle:
        json.dump(transcript, handle, indent=2, sort_keys=True)
        handle.write("\n")
    return path
