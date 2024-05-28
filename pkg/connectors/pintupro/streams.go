package pintupro

/*
API - WebSocket Channels Subscription
Channels Subscription

Successful Subscription
{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1676869976772,
  "method": "subscribe", // subscribe/unsubscribe
  "code": 0,
  "message": "SUCCESS",
  "data": {
    "channel": "trades.BTC-IDR",
    // specific data goes here
  }
}

Error Subscription
{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1676869976772,
  "method": "subscribe", // subscribe/unsubscribe
  "code": 10001,
  "message": "BAD_REQUEST",
  "reason": "request is missing required fields"
  "data": {
    "channel": "trades.BTC-IRD",  // wrong channel name given in request
  }
}

Websocket connections allow for subscriptions to streaming data over the dedicated channel. For this, there’s a dedicated subscribe method available as part of the request wrapper:
Field 	Type 	Required 	Description
channels 	array of strings 	yes 	name of the respective channel to which we need to subscribe to or unsubscribe from

Example for channel subscription {
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"method": "subscribe", // subscribe/unsubscribe
"params": {
"channels": [
"trades.BTC-IDR",
...
]
}
}

Channels to subscribe can be both public and private. In case of the private channel the Websocket Channels Subscription routine should be performed first.
One subscription message can contain an array of channels to subscribe to. The subscription replies will be one per requested channel.

Public channels:
Channel Name 	Description
aggrbook.snapshot.{num_levels}.{symbol} 	aggregated book snapshot for {num_levels} levels of symbol {symbol}
trades.{symbol} 	trades happened in the matching engine for symbol {symbol}

Private channels:
Channel Name 	Description
user.orders.{symbol} 	statuses updates for all orders sent for a given symbol {symbol}
user.orders 	statuses updates for all order for all symbols
user.trades.{symbol} 	trades/fills notifications for a given symbol {symbol}
user.trades 	trades/fills notification for all symbols
user.balance.snapshot 	periodic updates of user’s account balance

The response for the channel subscription comes in the form of Generic Response Wrapper (both success and error), with the following addition:
- there’s additional channel field sent back with the name of the channel the message belongs to, it’s a part of data field and is always sent
Channel Unsubscription

You can unsubscribe from the required channel at any point in time. For this you can use the similar Subscription request wrapper, but with “method”: “unsubscribe” field. Response structure is similar as subscription response
Channel Data Stream

In case the subscription to the channel is successful and the respective return code is returned, gateway will start streaming the data to the client. The stream message has the following fields:
Field 	Type 	Description
timestamp 	integer 	gateway timestamp(unix milli) when the message was sent to the client
method 	string 	always “subscription” to show that the message is the channel data message
channels 	string 	the actual name of the channel the update is sent for
data 	object 	subscription stream message payload data object
Channel Data Stream Errors

In case any unrecoverable application or the protocol error is encountered during the subscription streaming process, the gateway will close the web-socket with the respective Close Frame specifying the closure status code
Heartbeats

In order to ensure the client’s liveness, the backend gateway may from time to time send the respective heartbeat request to which client needs to send the response within 10 seconds from timeframe from the value specified in request.

Failure to reply to heartbeat request within the given timeframe will result in server terminating the websocket connection

Example of hearbeat request message
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 1676869976772,
"method": "heartbeat-request"
}

Example of hearbeat response message
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2", // has to match the one in request
"timestamp": 1676869978800,
"method": "heartbeat-response"
}

Websocket Subscriptions - Private
User Balance Snapshot

Streaming data
{
"timestamp": 1676869976772,
"method": "subscription", // signal that is an active update
"channel": "user.balance.snapshot",  // only set when "method" is "subscription"
"data": { // object, structure depends on the "channel" definition
  "assets": {
    "btc": {
      "balance": "10",
      "available": "8",
      "order": "2"
    },
    "eth": {
      "balance": "32",
      "available": "30",
      "order": "2",
    },
    ...
    }
  }
}

Channel: user.balance

{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"method": "subscribe",
"params": {
"channels": ["user.balance.snapshot"]
}
}
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
assets 	object 	List of all non-zero assets in the wallet in dictionaries. The keys of the dict is the asset name such as "eth", "btc"
assets.[asset_name].balance 	string 	Total balance of current asset
assets.[asset_name].available 	string 	Available amount to withdraw of current asset
assets.[asset_name].order 	string 	Total asset currently in open order
User Orders per symbol

Streaming data
{
"timestamp": 1676869976772,
"method": "subscription", // signal that is an active update
"channel": "user.orders.BTC-IDR",  // only set when "method" is "subscription
"data": {
  "orders": [
    {
      "status": "PARTIALLY_FILLED",
      "symbol": "BTC-IDR",
      "type": "LIMIT"
      "time_in_force": "GTC",
      "exec_inst": "POST_ONLY",
      "side": "BUY",
      "price": "15000",
      "size": "2",
      "cum_price": "15000",
      "cum_size": "0.35",
      "cum_value": "5250",
      "order_id": "12345678-abcd-efgh-ijkl-1234567890ab",
      "client_order_id": "my-cool-order-id-1",
      "created_at": 16775774795300,
      "updated_at": 16775774797678
    },
      ...
    ]
  }
}

Unsubscription
{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1676869976772,
  "method": "unsubscribe",
  "code": "OK",
  "message": "Success",
  "data": {
    "channel": "user.orders.BTC-IDR"
  }
}

{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1676869976775,
  "method": "unsubscribe",
  "code": "OK",
  "message": "Success",
  "data": {
    "channel": "user.orders.ETH-IDR",
  }
}

This is a private API, which requires authorization fields to be provided on the body. See this page for reference.

Channel: user.orders.{symbol}

{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "subscribe", // subscribe
"params": {
"channels": [
"user.orders.BTC-IDR",
...
]
}
}

Unsubscription Request
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "unsubscribe", // unsubscribe
"params": {
"channels": [
"user.orders.BTC-IDR",
"user.orders.ETH-IDR",
...
]
}
}

NOTE: in case unsubscribe request contains multiple channels in it’s payload, the client should expect one response message per channel in return
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
data.orders 	2d-array (object) 	statuses of the orders that were found successfully. See below section for single order status definition
Streaming Reply - Single order status definition contains the following fields:
Field 	Type 	Description
status 	string(enum) 	current order status: PLACED, CANCELED, REJECTED, PARTIALLY_FILLED, FILLED
reason 	string 	(optional) only sent for REJECTED order, actual rejection reason as string
symbol 	string 	instrument the order is placed for, ex. ‘BTC-IDR’
type 	string(enum) 	order type: MARKET, LIMIT
time_in_force 	string(enum) 	order validity instructions:
* for Market Orders - empty
* for Limit Orders - IOC (immediate-or-cancel) or FOK (fill-or-kill) or GTC (good-till-cancel)
exec_inst 	string(enum) 	(for Limit Orders only) - empty or POST_ONLY
side 	string(enum) 	BUY or SELL
price 	string 	price at which the order was placed. (Limit Order only)
size 	string 	original order quantity
cum_price 	string 	average price of already filled qty for this order (in case of the partial fill)
cum_size 	string 	cumulative filled quantity for this order (in case of the partial fill)
cum_value 	string 	cumulative filled quantity in QUOTE currency for this order (in case of the partial fill), ex. for ‘BTC-IDR’ the value will be in IDR
order_id 	string 	order id assigned by the Exchange
client_order_id 	string 	(optional) client id if assigned by the client while placing the order
created_at 	integer 	timestamp(unix milli) of order creation within Exchange
updated_at 	integer 	timestamp(unix milli) of the latest update of this order
User Orders (all symbols)

Streaming data
{
"timestamp": 1676869976772,
"method": "subscription", // signal that is an active update
"channel": "user.orders",  // only set when "method" is "subscription
"data": {
  "orders": [
    {
      "status": "PARTIALLY_FILLED",
      "symbol": "BTC-IDR",
      "type": "LIMIT"
      "time_in_force": "GTC",
      "exec_inst": "POST_ONLY",
      "side": "BUY",
      "price": "15000",
      "size": "2",
      "cum_price": "15000",
      "cum_size": "0.35",
      "cum_value": "5250",
      "order_id": "12345678-abcd-efgh-ijkl-1234567890ab",
      "client_order_id": "my-cool-order-id-1",
      "created_at": 16775774795300,
      "updated_at": 16775774797678
    },
      ...
    ]
  }
}

Unsubscription
{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1676869976772,
  "method": "unsubscribe",
  "code": "OK",
  "message": "Success",
  "data": {
    "channel": "user.orders"
  }
}

{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1676869976775,
  "method": "unsubscribe",
  "code": "OK",
  "message": "Success",
  "data": {
    "channel": "user.orders",
  }
}

This is a private API, which requires authorization fields to be provided on the body. See this page for reference.

Channel: user.orders

{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "subscribe", // subscribe
"params": {
"channels": [
"user.orders",
...
]
}
}

Unsubscription Request
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "unsubscribe", // unsubscribe
"params": {
"channels": [
"user.orders",
"user.orders.ETH-IDR",
...
]
}
}

NOTE: in case unsubscribe request contains multiple channels in it’s payload, the client should expect one response message per channel in return
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
data.orders 	2d-array (object) 	statuses of the orders that were found successfully. See below section for single order status definition
Streaming Reply - Single order status definition contains the following fields:
Field 	Type 	Description
status 	string(enum) 	current order status: PLACED, CANCELED, REJECTED, PARTIALLY_FILLED, FILLED
reason 	string 	(optional) only sent for REJECTED order, actual rejection reason as string
symbol 	string 	instrument the order is placed for, ex. ‘BTC-IDR’
type 	string(enum) 	order type: MARKET, LIMIT
time_in_force 	string(enum) 	order validity instructions:
* for Market Orders - empty
* for Limit Orders - IOC (immediate-or-cancel) or FOK (fill-or-kill) or GTC (good-till-cancel)
exec_inst 	string(enum) 	(for Limit Orders only) - empty or POST_ONLY
side 	string(enum) 	BUY or SELL
price 	string 	price at which the order was placed. (Limit Order only)
size 	string 	original order quantity
cum_price 	string 	average price of already filled qty for this order (in case of the partial fill)
cum_size 	string 	cumulative filled quantity for this order (in case of the partial fill)
cum_value 	string 	cumulative filled quantity in QUOTE currency for this order (in case of the partial fill), ex. for ‘BTC-IDR’ the value will be in IDR
order_id 	string 	order id assigned by the Exchange
client_order_id 	string 	(optional) client id if assigned by the client while placing the order
created_at 	integer 	timestamp(unix milli) of order creation within Exchange
updated_at 	integer 	timestamp(unix milli) of the latest update of this order
User Trades per symbol

Streaming data
{
  "timestamp": 1672304484978,
  "method": "subscription", // signal that is an active update
  "channel": "user.trades.BTC-IDR",
  "data": { // empty in case of an error
    "trades": [
      {
      "trade_id": "fake-trade-id-2",
      "order_id": "aaa-bbb-ccc-2",
      "symbol": "BTC-IDR",
      "side": "buy",
      "price": "351000000",
      "fee": "0.001",
      "fee_asset": "BTC",
      "fee_type": "maker",
      "traded_size": "0.105",
      "client_order_id": "xxx-yyy-zzz-2",
      "traded_at": 1676869976772
      },
      ...
      {
      "trade_id": "fake-trade-id-1",
      "order_id": "aaa-bbb-ccc-1",
      "symbol": "BTC-IDR",
      "side": "buy",
      "price": "350000000",
      "fee": "0.001",
      "fee_asset": "BTC",
      "fee_type": "maker",
      "traded_size": "0.905",
      "client_order_id": "xxx-yyy-zzz-1",
      "traded_at": 1676869976760
      }
    ]
  }
}

Unsubscription
{
    "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
    "timestamp": 1676869976772,
    "method": "unsubscribe",
    "code": "OK",
    "message": "Success",
    "data": {
            "channel": "user.trades.BTC-IDR"
    }
}

{
    "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
    "timestamp": 1676869976775,
    "method": "unsubscribe",
    "code": "OK",
    "message": "Success",
    "data": {
            "channel": "user.trades.ETH-IDR",
    }
}

This is a private API, which requires authorization fields to be provided on the body. See this page for reference.

NOTE: the order of the elements in the trades array is by traded_at field, where most recent message comes first. This way some parsing level optimisations (re reaction time) can be possible on the client side

Channel: user.trades.{symbol}

{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "subscribe", // subscribe
"params": {
"channels": [
"user.trades.BTC-IDR",
...
]
}
}

Unsubscription Request
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "unsubscribe", // unsubscribe
"params": {
"channels": [
"user.trades.BTC-IDR",
"user.trades.ETH-IDR",
...
]
}
}

NOTE: in case unsubscribe request contains multiple channels in it’s payload, the client should expect one response message per channel in return
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
data.trades 	array 	Updates on trades happened since last message
Streaming Reply - Single trade definition contains the following fields:
Field 	Type 	Description
trades.[index].trade_id 	string 	ID of a trade that is generated by Exchange
trades.[index].order_id 	string 	ID of an order that is generated by Exchange
trades.[index].symbol 	string 	Symbol name (example: BTC-IDR, ETH-IDR, etc.)
trades.[index].side 	string(enum) 	side of order [BUY, SELL]
trades.[index].price 	string 	price where order wants to be placed
trades.[index].traded_size 	string 	amount in base that has been executed/filled
trades.[index].fee 	string 	fee related to the trade
NOTE: value can be negative in case it’s a rebate
trades.[index].fee_asset 	string 	asset of what the deducted fee is based in, consisting either base or quote (example: BTC, IDR, etc.)
trades.[index].fee_type 	string(enum) 	source of the fee application [maker, taker]
trades.[index].client_order_id 	string 	unique key that will be used by clients to identify their orders
trades.[index].traded_at 	integer 	timestamp(unix milli) of when trade happened
User Trades (all symbols)

Streaming data
{
  "timestamp": 1672304484978,
  "method": "subscription", // signal that is an active update
  "channel": "user.trades",
  "data": { // empty in case of an error
    "trades": [
      {
      "trade_id": "fake-trade-id-2",
      "order_id": "aaa-bbb-ccc-2",
      "symbol": "BTC-IDR",
      "side": "buy",
      "price": "351000000",
      "fee": "0.001",
      "fee_asset": "BTC",
      "fee_type": "maker",
      "traded_size": "0.105",
      "client_order_id": "xxx-yyy-zzz-2",
      "traded_at": 1676869976772
      },
      ...
      {
      "trade_id": "fake-trade-id-1",
      "order_id": "aaa-bbb-ccc-1",
      "symbol": "SOL-IDR",
      "side": "buy",
      "price": "350000000",
      "fee": "0.001",
      "fee_asset": "SOL",
      "fee_type": "maker",
      "traded_size": "0.905",
      "client_order_id": "xxx-yyy-zzz-1",
      "traded_at": 1676869976760
      }
    ]
  }
}

Unsubscription
{
    "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
    "timestamp": 1676869976772,
    "method": "unsubscribe",
    "code": "OK",
    "message": "Success",
    "data": {
            "channel": "user.trades"
    }
}

{
    "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
    "timestamp": 1676869976775,
    "method": "unsubscribe",
    "code": "OK",
    "message": "Success",
    "data": {
            "channel": "user.trades",
    }
}

This is a private API, which requires authorization fields to be provided on the body. See this page for reference.

NOTE: the order of the elements in the trades array is by traded_at field, where most recent message comes first. This way some parsing level optimisations (re reaction time) can be possible on the client side

Channel: user.trades.{symbol}

{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "subscribe", // subscribe
"params": {
"channels": [
"user.trades,
...
]
}
}

Unsubscription Request
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "unsubscribe", // unsubscribe
"params": {
"channels": [
"user.trades",
"user.trades.ETH-IDR",
...
]
}
}

NOTE: in case unsubscribe request contains multiple channels in it’s payload, the client should expect one response message per channel in return
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
data.trades 	array 	Updates on trades happened since last message
Streaming Reply - Single trade definition contains the following fields:
Field 	Type 	Description
trades.[index].trade_id 	string 	ID of a trade that is generated by Exchange
trades.[index].order_id 	string 	ID of an order that is generated by Exchange
trades.[index].symbol 	string 	Symbol name (example: BTC-IDR, ETH-IDR, etc.)
trades.[index].side 	string(enum) 	side of order [BUY, SELL]
trades.[index].price 	string 	price where order wants to be placed
trades.[index].traded_size 	string 	amount in base that has been executed/filled
trades.[index].fee 	string 	fee related to the trade
NOTE: value can be negative in case it’s a rebate
trades.[index].fee_asset 	string 	asset of what the deducted fee is based in, consisting either base or quote (example: BTC, IDR, etc.)
trades.[index].fee_type 	string(enum) 	source of the fee application [maker, taker]
trades.[index].client_order_id 	string 	unique key that will be used by clients to identify their orders
trades.[index].traded_at 	integer 	timestamp(unix milli) of when trade happened
Websocket Subscriptions - Public
Orderbook Snapshot

Streaming data
{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 1672304484978,
  "method": "subscription", // signal that is an active update
  "channel": "aggrbook.snapshot.10.BTC-USDT",
  "data": {
    "symbol": "BTC-USDT",
    "bids": [
        ...,
        [
            "16493.50",
            "0.006",
            "100",
        ],
        [
            "16493.00",
            "0.100",
            "87",
        ]
    ],
    "asks": [
        [
            "16611.00",
            "0.029",
            "124",
        ],
        [
            "16612.00",
            "0.213",
            "90",
        ],
        ...,
    ]
  }
}

Channel: aggrbook.snapshot.{depth}.{symbol}


{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"method": "subscribe",
"params": {
"channels": ["aggrbook.snapshot.{depth}.{symbol}"]
}
}
Request Parameters
Field 	Type 	Required 	Description
symbol 	string 	yes 	Symbol name
depth 	integer 	no 	The depth (number of price levels) of the market data book. Default is 1 for level 1 order book (quotes). Valid values are 1 or 10.
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
data.symbol 	string 	Symbol name
data.bids 	array (string) 	Price levels ordered by descending by price. Each price level is three elements array of decimals. Bids array: [0] = Price, [1] = Quantity, [2] = Number of Orders
data.asks 	array (string) 	Price levels ordered by descending by price. Each price level is three elements array of decimals. Bids array: [0] = Price, [1] = Quantity, [2] = Number of Orders
Trades

Streaming data
{
  "timestamp": 1672304484978,
  "method": "subscription", // signal that is an active update
  "channel": "trades.BTC-USDT",
  "data":
    "trades": [
      {
        "side": "SELL",
        "price": "51327.500000",
        "size": "0.000100",
        "timestamp": 1613581138462,
        "symbol": "BTC-USDT"
      }
    ]
  }
}

Channel: trades.{symbol}


{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"method": "subscribe",
"params": {
"channels": ["trades.{symbol}"]
}
}

Unsubscription Request
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-api-key",
"signature": "my-signature",
"method": "unsubscribe", // unsubscribe
"params": {
"channels": [
"user.trades.BTC-IDR",
"user.trades.ETH-IDR",
...
]
}
}
Request Parameters
Field 	Type 	Required 	Description
symbol 	string 	yes 	Symbol name
Streaming Reply - Channel update contains the following fields:
Field 	Type 	Description
data.trades 	array 	Updates on trades happened since last message
Streaming Reply - Single trade definition contains the following fields:
Field 	Type 	Description
trades.[index].side 	string 	Side of the taker order (buy or sell)
trades.[index].price 	string 	Trade price
trades.[index].size 	string 	Trade quantity
trades.[index].timestamp 	integer 	Trade timestamp(unix milli)
trades.[index].symbol 	string 	Symbol name (example: BTC-IDR, ETH-IDR, etc.)
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func (p *PintuPro) wsReconnectLoop(ctx context.Context, wsBaseURL string) {
	var connectedAt time.Time
	for {
		if err := p.ws.Connect(ctx, wsBaseURL); err != nil {
			log.Printf("pintupro websocket connect error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		if p.key != "" {
			if err := p.auth(ctx); err != nil {
				log.Printf("pintupro auth write error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
		}
		// Connected. Subscribe to streams.
		connectedAt = time.Now()

		var toSubscribe []string
		p.mux.RLock()
		if len(p.subscribedStreams) > 0 {
			toSubscribe = p.subscribedStreams
			p.subscribedStreams = p.subscribedStreams[:0]
		}
		p.mux.RUnlock()

		if len(toSubscribe) > 0 {
			log.Printf("pintupro: subscribing: %q", strings.Join(toSubscribe, ","))
			if err := p.subscribeStreams(ctx, toSubscribe); err != nil {
				log.Printf("pintupro websocket subscribe error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
		}

	ignore:
		select {
		case <-ctx.Done():
			return
		case reason := <-p.reconnectCh:
			// If last connection was established less than 10 seconds ago, ignore reconnect request.
			// To avoid reconnect loop that can lead to IP ban.
			if time.Since(connectedAt) < time.Second*10 {
				goto ignore
			}
			log.Printf("reconnecting: %s", reason)
			p.ws.Close()
			continue
		}
	}
}

func (p *PintuPro) auth(ctx context.Context) error {
	req := WrapAndSign("public/auth", p.key, p.secret, uuid.NewString(), nil, time.Now())
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ws.Write(ctx, b)
}

func (p *PintuPro) subscribeStreams(ctx context.Context, streams []string) error {
	req := Envelope{
		RequestID: uuid.NewString(),
		Method:    "subscribe",
		Params:    map[string]interface{}{"channels": streams},
		Timestamp: time.Now().UnixMilli(),
	}
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ws.Write(ctx, b)
}

func (p *PintuPro) SubscribeBookTickers(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	if symbols[0] == "error" {
		return errors.New("SubscribeBookTickers error")
	}

	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@bookTicker"
	}
	p.subscribedStreams = append(p.subscribedStreams, streams...)

	return nil
}

func (p *PintuPro) SubscribeBookAggTrades(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	if symbols[0] == "error" {
		return errors.New("SubscribeBookAggTrades error")
	}
	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@aggTrade"
	}

	p.subscribedStreams = append(p.subscribedStreams, streams...)

	return nil
}

//easyjson:json
type wsMessage struct {
	RequestID string `json:"request_id"`
	Timestamp int64  `json:"timestamp"`
	Method    string `json:"method"`
	Channel   string `json:"channel"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Reason    string `json:"reason"`
	Data      json.RawMessage
}

func (p *PintuPro) registerWSHandlers() {
	p.wsHandlers = map[string]wsHandlerFunc{
		"heartbeat-request":  p.handleHeartbeat,
		"subscription":       p.handleSubscription,
		"trades.":            p.handlePublicTrades,
		"aggrbook.snapshot.": p.handleOrderBook,
		// "user.balance":       p.handleUserBalance,
		// "user.orders":        p.handleUserOrders,
		// "user.orders.snapshot": p.handleUserOrdersSnapshot,
		// "user.trades":        p.handleUserTrades,
		// "user.trades.snapshot": p.handleUserTradesSnapshot,
	}
}

func (p *PintuPro) getWsHandler(method, channel string) (wsHandlerFunc, error) {
	handler, ok := p.wsHandlers[method]
	if ok {
		return handler, nil
	}

	handler, ok = p.wsHandlers[channel]
	if ok {
		return handler, nil
	}

	for k, h := range p.wsHandlers {
		if strings.HasPrefix(channel, k) {
			return h, nil
		}
	}

	err := fmt.Errorf("unsupported method: %s", method)
	if channel != "" {
		err = fmt.Errorf("unsupported channel: %s", channel)
	}

	return nil, err
}

func (p *PintuPro) handleHeartbeat(msg wsMessage, _ chan<- models.ExchangeMessage) error {
	req := wsMessage{
		RequestID: msg.RequestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    "heartbeat-response",
	}
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat response: %w", err)
	}
	return p.ws.Write(context.Background(), b)
}

func (p *PintuPro) handleSubscription(msg wsMessage, _ chan<- models.ExchangeMessage) error {
	var sub struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(msg.Data, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}
	log.Printf("subscribed to %s", sub.Channel)
	p.subscribedStreams = append(p.subscribedStreams, sub.Channel)

	return nil
}

// easyjson:json
type tradesMsg struct {
	Trades []struct {
		Side      string `json:"side"`
		Price     string `json:"price"`
		Size      string `json:"size"`
		Timestamp int64  `json:"timestamp"`
		Symbol    string `json:"symbol"`
	} `json:"trades"`
}

func tsToTime(ts int64) time.Time {
	return time.Unix(0, ts*int64(time.Millisecond))
}

func (p *PintuPro) handlePublicTrades(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	var trades tradesMsg
	if err := json.Unmarshal(msg.Data, &trades); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	for _, trade := range trades.Trades {
		side := models.OrderSideBuy
		if trade.Side == "SELL" {
			side = models.OrderSideSell
		}

		price, err := decimal.NewFromString(trade.Price)
		if err != nil {
			return fmt.Errorf("failed to parse price: %w", err)
		}

		size, err := decimal.NewFromString(trade.Size)
		if err != nil {
			return fmt.Errorf("failed to parse size: %w", err)
		}

		ch <- models.ExchangeMessage{
			Exchange:  Name,
			Symbol:    trade.Symbol,
			Timestamp: tsToTime(trade.Timestamp),
			MsgType:   models.MsgTypeTrade,
			Payload: models.Trade{
				Side:      side,
				Timestamp: tsToTime(trade.Timestamp),
				Price:     price,
				Size:      size,
			},
		}
	}

	return nil
}

// easyjson:json
type orderBookMsg struct {
	Symbol string     `json:"symbol"`
	Bids   [][]string `json:"bids"`
	Asks   [][]string `json:"asks"`
}

func (p *PintuPro) handleOrderBook(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	var ob orderBookMsg
	if err := json.Unmarshal(msg.Data, &ob); err != nil {
		return fmt.Errorf("failed to unmarshal orderbook: %w", err)
	}

	// For now I don't care much about depth, so I will just take the first level.
	bbo := models.BBO{
		Timestamp: tsToTime(msg.Timestamp),
		Bid:       models.PriceLevel{},
		Ask:       models.PriceLevel{},
	}

	if len(ob.Bids) > 0 {
		bbo.Bid.Price, _ = decimal.NewFromString(ob.Bids[0][0])
		bbo.Bid.Size, _ = decimal.NewFromString(ob.Bids[0][1])
	}

	if len(ob.Asks) > 0 {
		bbo.Ask.Price, _ = decimal.NewFromString(ob.Asks[0][0])
		bbo.Ask.Size, _ = decimal.NewFromString(ob.Asks[0][1])
	}

	ch <- models.ExchangeMessage{
		Exchange:  Name,
		Symbol:    ob.Symbol,
		Timestamp: tsToTime(msg.Timestamp),
		MsgType:   models.MsgTypeBBO,
		Payload:   bbo,
	}

	return nil
}

func (p *PintuPro) Listen(ctx context.Context, ch chan<- models.ExchangeMessage) {
	errCnt := 0
	rawCh := make(chan []byte, 100)
	go func() {
		for {
			err := p.ws.Listen(rawCh)
			if err != nil {
				log.Printf("PintuPro.Listen returned: %v\n", err)
			}

			select {
			case <-ctx.Done():
				return
			default:
				msg := "normal close"
				if err != nil {
					msg = err.Error()
				}
				p.reconnectCh <- msg
				// Wait for some time before listening again.
				time.Sleep(time.Second)
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
	loop:
		if errCnt > 10 {
			p.reconnectCh <- "too many errors"
			errCnt = 0
			goto loop
		}

		select {
		case <-ticker.C:
			ts := atomic.LoadInt64(&p.lastReceived)
			if time.Since(time.Unix(0, ts)) > p.idleTimeout {
				p.reconnectCh <- fmt.Sprintf("no messages for %s", p.idleTimeout)
				time.Sleep(time.Second)
				goto loop
			}
		case msg := <-rawCh:
			var r wsMessage
			if err := json.Unmarshal(msg, &r); err != nil {
				log.Printf("failed to unmarshal msg: %v\n%v\n", err, string(msg))
				errCnt++
				goto loop
			}

			if r.Code != 0 {
				log.Printf("%s:%s %s", r.Method, r.Message, r.Reason)
				goto loop
			}

			handler, err := p.getWsHandler(r.Method, r.Channel)
			if err != nil {
				log.Println(err)
				goto loop
			}

			if err := handler(r, ch); err != nil {
				log.Printf("handler error: %v", err)
				errCnt++
				goto loop
			}
		}
	}
}
