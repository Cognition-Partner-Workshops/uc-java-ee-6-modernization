"""Parity scenarios. Each run() gets a Runner and a context with its own seeded RNG (ctx["rng"])."""
from decimal import Decimal

from harness import LOGIN_PASSWORD, LOGIN_USER, ORDER_FEE, findTable, parseHtml

CENT = Decimal("0.01")


def pickSymbols(rng, count):
    return sorted(rng.sample(range(100), count))


def orderFacts(rows):
    order = rows[0] if rows else {}
    return {key: order.get(key) for key in ("type", "symbol", "quantity", "txn fee", "order status")}


def loginUid0(runner, ctx):
    runner.login(ctx, LOGIN_USER, LOGIN_PASSWORD, volatileAccount=True)


def homeUid0(runner, ctx):
    runner.login(ctx, LOGIN_USER, LOGIN_PASSWORD, volatileAccount=True)
    runner.step(ctx, "home", "GET", "/app", params={"action": "home"}, volatileAccount=True)


def quotesThreeSymbols(runner, ctx):
    symbols = ["s:%d" % n for n in pickSymbols(ctx["rng"], 3)]
    runner.login(ctx, LOGIN_USER, LOGIN_PASSWORD, volatileAccount=True)
    response = runner.step(ctx, "quotes", "GET", "/app", params={"action": "quotes", "symbols": ",".join(symbols)})
    rows = findTable(parseHtml(response.text), "symbol")
    ctx["checks"]["requestedSymbols"] = symbols
    ctx["checks"]["returnedSymbols"] = [row["symbol"] for row in rows]
    ctx["checks"]["companyNames"] = [row["company"] for row in rows]


def registerNewUser(runner, ctx):
    runner.register(ctx, runner.newUserId())
    account = runner.account(ctx)
    holdings = runner.portfolio(ctx)
    ctx["checks"]["openingBalance"] = str(account["opening"])
    ctx["checks"]["cashEqualsOpening"] = account["cash"] == account["opening"]
    ctx["checks"]["holdingCount"] = len(holdings)
    ctx["checks"]["profileFullName"] = account["inputs"].get("fullname")
    ctx["checks"]["profileEmail"] = account["inputs"].get("email")


def buy(runner, ctx):
    rng = ctx["rng"]
    symbol, quantity = "s:%d" % rng.randrange(100), rng.randint(1, 50)
    runner.register(ctx, runner.newUserId())
    before = runner.account(ctx, "account-before")
    price = runner.quotePrice(ctx, symbol)
    order = runner.buy(ctx, symbol, quantity)
    after = runner.account(ctx, "account-after")
    holdings = runner.portfolio(ctx)
    ctx["checks"]["order"] = orderFacts(order)
    ctx["checks"]["cashDebitIsPriceTimesQuantityPlusFee"] = (
        (before["cash"] - after["cash"]).quantize(CENT) == (price * quantity + ORDER_FEE).quantize(CENT))
    ctx["checks"]["holdings"] = [(h["symbol"], h["quantity"]) for h in holdings]
    ctx["checks"]["purchasePriceIsQuotePrice"] = [Decimal(h["purchase price"]) == price for h in holdings]


def sell(runner, ctx):
    rng = ctx["rng"]
    symbol, quantity = "s:%d" % rng.randrange(100), rng.randint(1, 50)
    runner.register(ctx, runner.newUserId())
    runner.buy(ctx, symbol, quantity)
    holding = runner.portfolio(ctx, "portfolio-before")[0]
    before = runner.account(ctx, "account-before")
    order = runner.sell(ctx, holding["holding ID"])
    after = runner.account(ctx, "account-after")
    remaining = runner.portfolio(ctx, "portfolio-after")
    proceeds = Decimal(holding["current price"]) * quantity - ORDER_FEE
    ctx["checks"]["order"] = orderFacts(order)
    ctx["checks"]["cashCreditIsPriceTimesQuantityMinusFee"] = (
        (after["cash"] - before["cash"]).quantize(CENT) == proceeds.quantize(CENT))
    ctx["checks"]["holdingCountAfterSell"] = len(remaining)


def portfolioAfterTrades(runner, ctx):
    rng = ctx["rng"]
    first, second = pickSymbols(rng, 2)
    qtyFirst, qtySecond = rng.randint(1, 50), rng.randint(1, 50)
    runner.register(ctx, runner.newUserId())
    runner.buy(ctx, "s:%d" % first, qtyFirst, "buy-1")
    runner.buy(ctx, "s:%d" % second, qtySecond, "buy-2")
    both = runner.portfolio(ctx, "portfolio-after-buys")
    target = next(h for h in both if h["symbol"] == "s:%d" % first)
    runner.sell(ctx, target["holding ID"])
    remaining = runner.portfolio(ctx, "portfolio-after-sell")
    ctx["checks"]["holdingsAfterBuys"] = sorted((h["symbol"], h["quantity"]) for h in both)
    ctx["checks"]["holdingsAfterSell"] = sorted((h["symbol"], h["quantity"]) for h in remaining)
    ctx["checks"]["basisIsPurchasePriceTimesQuantity"] = [
        (Decimal(h["purchase price"]) * Decimal(h["quantity"])).quantize(CENT) == Decimal(h["purchase basis"])
        for h in both]


def accountUpdate(runner, ctx):
    user = runner.newUserId()
    runner.register(ctx, user)
    runner.step(ctx, "update_profile", "POST", "/app", data={
        "action": "update_profile", "userID": user, "fullname": "Renamed Tester", "password": "pw2",
        "cpassword": "pw2", "address": "2 New Road", "creditcard": "1111-2222-3333-4444",
        "email": "renamed@example.com"})
    account = runner.account(ctx)
    runner.step(ctx, "logout", "GET", "/app", params={"action": "logout"})
    relogin = runner.login(ctx, user, "pw2", name="login-new-password")
    ctx["checks"]["profile"] = {k: account["inputs"].get(k) for k in ("fullname", "address", "email", "creditcard")}
    ctx["checks"]["loginWithNewPasswordStatus"] = relogin.status_code


def invalidLogin(runner, ctx):
    runner.step(ctx, "login-bad-password", "POST", "/app",
                data={"action": "login", "uid": LOGIN_USER, "passwd": "wrong-password"},
                mode="phrases", phrases=("An Error has occured during DayTrader processing", "DayTrader Error"))


def logout(runner, ctx):
    runner.login(ctx, LOGIN_USER, LOGIN_PASSWORD, volatileAccount=True)
    runner.step(ctx, "logout", "GET", "/app", params={"action": "logout"})
    runner.step(ctx, "home-after-logout", "GET", "/app", params={"action": "home"})


def restList(runner, ctx):
    runner.step(ctx, "list", "GET", "/addresses", rest=True, mode="json")


def restSearch(runner, ctx):
    runner.step(ctx, "search", "GET", "/addresses/search/Entry1", rest=True, mode="json")


def restEntry(runner, ctx):
    runner.step(ctx, "entry-existing", "GET", "/addresses/Entry2", rest=True, mode="status")
    runner.step(ctx, "entry-unknown", "GET", "/addresses/Nope", rest=True, mode="status")


SCENARIOS = [
    {"id": "01-login", "primeUid0": True, "run": loginUid0, "description": "Log in as the seeded user uid:0"},
    {"id": "02-home", "primeUid0": True, "run": homeUid0, "description": "Home page for uid:0"},
    {"id": "03-quotes", "primeUid0": True, "run": quotesThreeSymbols, "description": "Quotes for three seeded-random symbols"},
    {"id": "04-register", "run": registerNewUser, "description": "Register a new user with a 100000 opening balance"},
    {"id": "05-buy", "run": buy, "description": "New user buys one symbol; cash debit = price x qty + 24.95"},
    {"id": "06-sell", "run": sell, "description": "New user buys then sells; cash credit = price x qty - 24.95"},
    {"id": "07-portfolio", "run": portfolioAfterTrades, "description": "Portfolio after two buys and one sell"},
    {"id": "08-account-update", "run": accountUpdate, "description": "Update profile, check it, log in with new password"},
    {"id": "09-invalid-login", "run": invalidLogin, "description": "Wrong password for uid:0"},
    {"id": "10-logout", "primeUid0": True, "run": logout, "description": "Log out, then request home without a session"},
    {"id": "11-rest-list", "run": restList, "description": "GET /rest/addresses"},
    {"id": "12-rest-search", "run": restSearch, "description": "GET /rest/addresses/search/Entry1"},
    {"id": "13-rest-entry", "run": restEntry, "description": "GET /rest/addresses/{entryName}, existing and unknown"},
]
