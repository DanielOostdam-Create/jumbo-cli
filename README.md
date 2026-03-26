# jumbo-cli

CLI for [Jumbo](https://www.jumbo.com/) supermarket (Netherlands).

Interact with Jumbo's GraphQL API: search products, manage your basket, check promotions, view delivery slots, and more.

## Commands

| Command | Description |
|---|---|
| `jumbo login` | Authenticate (paste cookies from browser) |
| `jumbo search <query>` | Search products |
| `jumbo basket` | View current basket (winkelmandje) |
| `jumbo basket-add <sku> [qty]` | Add product to basket |
| `jumbo basket-update <line-id> <qty>` | Update item quantity |
| `jumbo basket-remove <line-id>` | Remove item from basket |
| `jumbo promotions` | Show personal promotions |
| `jumbo stamps` | Show koopzegels and digital stamps |
| `jumbo delivery-slots` | Show available delivery slots |
| `jumbo orders` | Show order history |
| `jumbo receipts` | Show online orders and store receipts |

All output is JSON.

## Installation

```bash
go install github.com/DanielOostdam-Create/jumbo-cli@latest
```

Or build from source:

```bash
git clone https://github.com/DanielOostdam-Create/jumbo-cli.git
cd jumbo-cli
go build -o jumbo .
```

## Authentication

Jumbo uses cookie-based authentication. To set up:

1. Log in to [jumbo.com](https://www.jumbo.com) in your browser
2. Open DevTools (Cmd+Option+I or F12) → Console
3. Type: `document.cookie`
4. Copy the entire output
5. Run `jumbo login` and paste when prompted

Session cookies typically expire after ~24 hours. Re-run `jumbo login` when they expire.

Auth is stored in `~/grocery-assistant/jumbo/auth.json` (chmod 600).

## Discovered API Details

Jumbo's web app uses a GraphQL API at `https://www.jumbo.com/api/graphql` powered by Apollo Router.

### Required Headers

The API requires specific headers (discovered via MITM analysis):
- `apollographql-client-name: JUMBO_MOBILE-orders`
- `apollographql-client-version: 30.14.0`
- `x-source: JUMBO_MOBILE-orders`
- `jmb-device-id: <device-id>`

### Key GraphQL Operations

**Basket:**
- `addBasketLines` — add products (mutation, input: `AddBasketLinesInput`)
- `updateBasketLineQuantity` — change quantity (mutation)
- `removeBasketLines` — remove items (mutation)
- `activeBasket` — view basket (query)

**Search:**
- `SearchProducts` — product search (input: `ProductSearchInput` with `searchTerms`, `searchType`)
- `SearchSuggestions` — search autocomplete

**Checkout:**
- `CheckoutDeliveryPeriods` — available delivery time slots
- `createCheckoutSession` — initialize checkout
- `CheckoutStores` — available stores

**Promotions & Loyalty:**
- `GetCustomerPromotions` — personalized promotions
- `EarnPromotions` / `BurnPromotions` — earn and redeem
- `GetBuyingStampsBalance` — koopzegels
- `GetDigitalStamps` — digital stamp campaigns

**Orders:**
- `OrdersPageOrders` — order history
- `GetOnlineOrdersAndStoreReceipts` — online + in-store receipts

**Profile:**
- `GetProfile` — loyalty profile
- `GetCustomerProductLists` — saved product lists

### Product SKU Format

Jumbo uses SKU strings like `131278BAK` (alphanumeric with suffix).

### Notes

- GraphQL introspection is disabled
- The API uses Apollo Router with federated services (search, basket, checkout, etc.)
- Some queries may return different results based on authentication state
- Rate limiting may apply after excessive requests

## Credits

- API endpoints discovered via MITM analysis of the Jumbo mobile app
- Inspired by [gwillem/appie-go](https://github.com/gwillem/appie-go) and [appie-extra](https://github.com/DanielOostdam-Create/appie-extra)
- Community resources: [SupermarktConnector](https://github.com/bartmachielsen/SupermarktConnector), [python-jumbo-api](https://github.com/peternijssen/python-jumbo-api)

## Disclaimer

This is an unofficial tool using undocumented APIs. Not affiliated with Jumbo. Use at your own risk.

## License

[AGPL-3.0](LICENSE)
