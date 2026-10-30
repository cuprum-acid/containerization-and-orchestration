"""Tickerline public API: a small in-memory mock.

The contract integrators rely on is openapi.yaml next to this package. It is
generated from this code by export_openapi.py and committed, so every change
of the public API shows up in the diff of one file.
"""

from datetime import datetime, timezone
from decimal import Decimal
from enum import Enum
from uuid import UUID, uuid4

from fastapi import FastAPI, HTTPException, status
from pydantic import BaseModel, Field

app = FastAPI(
    title="Tickerline API",
    version="1.0.0",
    description="Quotes, instruments and order placement for brokers and fintech apps.",
)


class Instrument(BaseModel):
    ticker: str = Field(examples=["SBER"])
    name: str
    isin: str = Field(examples=["RU0009029540"])
    currency: str = Field(examples=["RUB"])
    lot_size: int = Field(ge=1)


class Quote(BaseModel):
    ticker: str
    bid: Decimal
    ask: Decimal
    last: Decimal
    timestamp: datetime


class Side(str, Enum):
    buy = "buy"
    sell = "sell"


class OrderStatus(str, Enum):
    new = "new"
    filled = "filled"
    cancelled = "cancelled"


class OrderCreate(BaseModel):
    ticker: str
    side: Side
    quantity: int = Field(ge=1, description="Number of lots.")
    limit_price: Decimal = Field(gt=0, description="Limit price per share.")


class Order(OrderCreate):
    id: UUID
    status: OrderStatus
    created_at: datetime


INSTRUMENTS = {
    i.ticker: i
    for i in [
        Instrument(ticker="SBER", name="Sberbank", isin="RU0009029540", currency="RUB", lot_size=10),
        Instrument(ticker="GAZP", name="Gazprom", isin="RU0007661625", currency="RUB", lot_size=10),
        Instrument(ticker="YDEX", name="Yandex", isin="RU000A107T19", currency="RUB", lot_size=1),
    ]
}
PRICES = {"SBER": Decimal("312.40"), "GAZP": Decimal("128.15"), "YDEX": Decimal("4215.0")}
ORDERS: dict[UUID, Order] = {}


def _instrument(ticker: str) -> Instrument:
    try:
        return INSTRUMENTS[ticker.upper()]
    except KeyError:
        raise HTTPException(status.HTTP_404_NOT_FOUND, f"unknown ticker {ticker}")


def _quote(ticker: str) -> Quote:
    t = _instrument(ticker).ticker
    last = PRICES[t]
    spread = last * Decimal("0.0005")
    return Quote(ticker=t, bid=last - spread, ask=last + spread, last=last,
                 timestamp=datetime.now(timezone.utc))


@app.get("/health", include_in_schema=False)
def health() -> dict:
    return {"status": "ok"}


@app.get("/v2/instruments", tags=["instruments"])
def list_instruments() -> list[Instrument]:
    return list(INSTRUMENTS.values())


@app.get("/v2/instruments/{ticker}", tags=["instruments"])
def get_instrument(ticker: str) -> Instrument:
    return _instrument(ticker)


@app.get("/v2/quotes/{ticker}", tags=["quotes"])
def get_quote(ticker: str) -> Quote:
    return _quote(ticker)


# The v1 shape: only the last price. Kept for integrators that never moved to v2.
class QuoteV1(BaseModel):
    ticker: str
    price: Decimal


@app.get("/v1/quotes/{ticker}", tags=["quotes"], deprecated=True)
def get_quote_v1(ticker: str) -> QuoteV1:
    q = _quote(ticker)
    return QuoteV1(ticker=q.ticker, price=q.last)


class PriceBand(BaseModel):
    ticker: str
    min_price: Decimal
    max_price: Decimal


@app.get("/v2/instruments/{ticker}/band", tags=["instruments"])
def get_price_band(ticker: str) -> PriceBand:
    last = _quote(ticker).last
    return PriceBand(ticker=ticker.upper(), min_price=last * Decimal("0.8"), max_price=last * Decimal("1.2"))


@app.post("/v2/orders", tags=["orders"], status_code=status.HTTP_201_CREATED)
def create_order(body: OrderCreate) -> Order:
    _instrument(body.ticker)
    order = Order(**body.model_dump(), id=uuid4(), status=OrderStatus.new,
                  created_at=datetime.now(timezone.utc))
    ORDERS[order.id] = order
    return order


@app.get("/v2/orders/{order_id}", tags=["orders"])
def get_order(order_id: UUID) -> Order:
    try:
        return ORDERS[order_id]
    except KeyError:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "order not found")
