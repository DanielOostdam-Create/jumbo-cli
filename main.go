package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	graphqlURL    = "https://www.jumbo.com/api/graphql"
	clientName    = "JUMBO_MOBILE-orders"
	clientVersion = "30.14.0"
)

// Auth stores the session cookies and customer ID
type Auth struct {
	Cookies    []CookieEntry `json:"cookies"`
	CustomerID string        `json:"customer_id"`
	ExpiresAt  time.Time     `json:"expires_at"`
}

type CookieEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "grocery-assistant", "jumbo")
}

func authPath() string {
	return filepath.Join(configDir(), "auth.json")
}

func loadAuth() (*Auth, error) {
	data, err := os.ReadFile(authPath())
	if err != nil {
		return nil, err
	}
	var auth Auth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, err
	}
	return &auth, nil
}

func saveAuth(auth *Auth) error {
	os.MkdirAll(configDir(), 0700)
	data, _ := json.MarshalIndent(auth, "", "  ")
	return os.WriteFile(authPath(), data, 0600)
}

// buildCookieHeader constructs a cookie string from auth, cleaning values
func buildCookieHeader(auth *Auth) string {
	var parts []string
	for _, c := range auth.Cookies {
		// Clean cookie values that have quotes
		val := strings.Trim(c.Value, `"`)
		parts = append(parts, c.Name+"="+val)
	}
	return strings.Join(parts, "; ")
}

func doGraphQL(ctx context.Context, auth *Auth, operationName, query string, variables map[string]any) (json.RawMessage, error) {
	body := map[string]any{
		"operationName": operationName,
		"query":         query,
		"variables":     variables,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", graphqlURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	// Required headers discovered via MITM
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apollographql-client-name", clientName)
	req.Header.Set("apollographql-client-version", clientVersion)
	req.Header.Set("x-source", clientName)
	req.Header.Set("jmb-device-id", "jumbo-cli-001")
	req.Header.Set("Origin", "capacitor://jumbo")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15")
	req.Header.Set("Cookie", buildCookieHeader(auth))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized — session may have expired. Run: jumbo login")
	}
	if resp.StatusCode != 200 {
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	return result.Data, nil
}

func mustAuth() *Auth {
	auth, err := loadAuth()
	if err != nil {
		fatal("Not authenticated. Run: jumbo login\nError: %v", err)
	}
	return auth
}

func usage() {
	fmt.Fprintf(os.Stderr, `jumbo — CLI for Jumbo supermarket (Netherlands)

Usage: jumbo <command> [args...]

Commands:
  login                          Save session cookies (paste from browser)
  search <query>                 Search products
  basket                         View current basket (winkelmandje)
  basket-add <sku> [qty]         Add product to basket
  basket-update <line-id> <qty>  Update item quantity
  basket-remove <line-id>        Remove item from basket
  promotions                     Show personal promotions
  stamps                         Show koopzegels and digital stamps
  delivery-slots                 Show available delivery slots
  orders                         Show order history
  receipts                       Show online orders and store receipts

Config: ~/grocery-assistant/jumbo/auth.json (session cookies)

Authentication:
  1. Log in to jumbo.com in your browser
  2. Open DevTools (Cmd+Option+I) → Console
  3. Type: document.cookie
  4. Run: jumbo login
  5. Paste the cookie string when prompted
`)
	os.Exit(1)
}

// ---- Login ----

func cmdLogin(ctx context.Context) {
	fmt.Fprintf(os.Stderr, "Jumbo CLI login\n\n")
	fmt.Fprintf(os.Stderr, "1. Go to jumbo.com in your browser and log in\n")
	fmt.Fprintf(os.Stderr, "2. Open DevTools (Cmd+Option+I) → Console\n")
	fmt.Fprintf(os.Stderr, "3. Type: document.cookie\n")
	fmt.Fprintf(os.Stderr, "4. Copy the entire output\n\n")
	fmt.Fprintf(os.Stderr, "Paste your cookie string here:\n> ")

	var input string
	buf := make([]byte, 64*1024)
	n, _ := os.Stdin.Read(buf)
	input = strings.TrimSpace(string(buf[:n]))

	if input == "" {
		fatal("No cookies provided.")
	}

	// Parse cookie string (format: "name1=value1; name2=value2; ...")
	// Remove surrounding quotes if present
	input = strings.Trim(input, "'\"")

	var cookies []CookieEntry
	var customerID string
	for _, part := range strings.Split(input, "; ") {
		eqIdx := strings.Index(part, "=")
		if eqIdx <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:eqIdx])
		value := strings.TrimSpace(part[eqIdx+1:])
		cookies = append(cookies, CookieEntry{Name: name, Value: value})
		if name == "CdId" {
			customerID = value
		}
	}

	if len(cookies) == 0 {
		fatal("Could not parse cookies.")
	}
	if customerID == "" {
		fmt.Fprintf(os.Stderr, "Warning: CdId cookie not found — customer ID unknown.\n")
	}

	auth := &Auth{
		Cookies:    cookies,
		CustomerID: customerID,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	if err := saveAuth(auth); err != nil {
		fatal("Failed to save auth: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Saved %d cookies to %s\n", len(cookies), authPath())
	if customerID != "" {
		fmt.Fprintf(os.Stderr, "Customer ID: %s\n", customerID)
	}

	// Test the session
	fmt.Fprintf(os.Stderr, "Testing session...\n")
	data, err := doGraphQL(ctx, auth, "BasketPageActiveBasket", `{ activeBasket { ... on ActiveBasketResult { basket { id totalProductCount } } } }`, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: session test failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "The cookies may be expired. Try logging in again.\n")
	} else {
		fmt.Fprintf(os.Stderr, "Session works! Basket: %s\n", string(data))
	}
}

// ---- Commands ----

func cmdSearch(ctx context.Context, args []string) {
	if len(args) < 1 {
		fatal("Usage: jumbo search <query>")
	}
	auth := mustAuth()
	searchTerm := strings.Join(args, " ")

	// Sanitize
	searchTerm = strings.ReplaceAll(searchTerm, `"`, "")
	searchTerm = strings.ReplaceAll(searchTerm, `\`, "")

	gql := `query SearchProducts($input: ProductSearchInput!) {
		searchProducts(input: $input) {
			products { sku title subtitle }
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"searchTerms": searchTerm,
			"searchType":  "PRODUCTS",
		},
	}

	data, err := doGraphQL(ctx, auth, "SearchProducts", gql, vars)
	if err != nil {
		fatal("Search failed: %v", err)
	}

	printJSON(data)
}

func cmdBasket(ctx context.Context) {
	auth := mustAuth()

	gql := `{
		activeBasket {
			... on ActiveBasketResult {
				basket {
					id totalProductCount type
					lines {
						sku id quantity
						details { sku title subtitle }
					}
					calculation {
						totals { subTotal { amount } }
					}
				}
			}
			... on BasketError { errorMessage reason }
		}
	}`

	data, err := doGraphQL(ctx, auth, "BasketPageActiveBasket", gql, nil)
	if err != nil {
		fatal("Basket query failed: %v", err)
	}

	printJSON(data)
}

func cmdBasketAdd(ctx context.Context, args []string) {
	if len(args) < 1 {
		fatal("Usage: jumbo basket-add <sku> [quantity]")
	}
	auth := mustAuth()

	sku := args[0]
	qty := 1
	if len(args) > 1 {
		fmt.Sscanf(args[1], "%d", &qty)
		if qty <= 0 {
			qty = 1
		}
	}

	gql := `mutation BasketPageAddBasketItems($input: AddBasketLinesInput!) {
		addBasketLines(input: $input) {
			... on Basket {
				id totalProductCount
				lines { sku id quantity details { title } }
			}
			... on BasketError { errorMessage reason }
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"lines": []map[string]any{{"sku": sku, "quantity": qty}},
			"type":  "ECOMMERCE",
		},
	}

	data, err := doGraphQL(ctx, auth, "BasketPageAddBasketItems", gql, vars)
	if err != nil {
		fatal("Basket add failed: %v", err)
	}

	printJSON(map[string]any{"ok": true, "action": "added", "sku": sku, "quantity": qty, "data": data})
}

func cmdBasketUpdate(ctx context.Context, args []string) {
	if len(args) < 2 {
		fatal("Usage: jumbo basket-update <line-id> <quantity>")
	}
	auth := mustAuth()

	lineID := args[0]
	qty := 0
	fmt.Sscanf(args[1], "%d", &qty)

	gql := `mutation BasketPageUpdateBasketItemQuantity($input: UpdateBasketLineQuantityInput!) {
		updateBasketLineQuantity(input: $input) {
			... on Basket {
				id totalProductCount
				lines { sku id quantity details { title } }
			}
			... on BasketError { errorMessage reason }
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"id":       lineID,
			"quantity": qty,
			"type":     "ECOMMERCE",
		},
	}

	data, err := doGraphQL(ctx, auth, "BasketPageUpdateBasketItemQuantity", gql, vars)
	if err != nil {
		fatal("Basket update failed: %v", err)
	}

	printJSON(map[string]any{"ok": true, "action": "updated", "lineId": lineID, "quantity": qty, "data": data})
}

func cmdBasketRemove(ctx context.Context, args []string) {
	if len(args) < 1 {
		fatal("Usage: jumbo basket-remove <line-id>")
	}
	auth := mustAuth()

	lineID := args[0]

	gql := `mutation BasketPageRemoveBasketItems($input: RemoveBasketLinesInput!) {
		removeBasketLines(input: $input) {
			... on Basket {
				id totalProductCount
				lines { sku id quantity details { title } }
			}
			... on BasketError { errorMessage reason }
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"ids":  []string{lineID},
			"type": "ECOMMERCE",
		},
	}

	data, err := doGraphQL(ctx, auth, "BasketPageRemoveBasketItems", gql, vars)
	if err != nil {
		fatal("Basket remove failed: %v", err)
	}

	printJSON(map[string]any{"ok": true, "action": "removed", "lineId": lineID, "data": data})
}

func cmdPromotions(ctx context.Context) {
	auth := mustAuth()

	gql := `query GetCustomerPromotions {
		customerPromotions {
			id title subtitle description
			validFrom validUntil
			imageUrl
			badge
			redemptionLimit
		}
	}`

	data, err := doGraphQL(ctx, auth, "GetCustomerPromotions", gql, nil)
	if err != nil {
		fatal("Promotions query failed: %v", err)
	}

	printJSON(data)
}

func cmdStamps(ctx context.Context) {
	auth := mustAuth()

	gql1 := `query GetBuyingStampsBalance {
		buyingStampsBalance {
			amountOfFullCards amountOfStamps amountOfStampsPerCard monetaryValue
		}
	}`

	data1, err := doGraphQL(ctx, auth, "GetBuyingStampsBalance", gql1, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Buying stamps: %v\n", err)
		data1 = json.RawMessage(`null`)
	}

	gql2 := `query GetDigitalStamps {
		digitalStamps { balance campaignName }
	}`

	data2, err := doGraphQL(ctx, auth, "GetDigitalStamps", gql2, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Digital stamps: %v\n", err)
		data2 = json.RawMessage(`null`)
	}

	printJSON(map[string]any{
		"buyingStamps":  data1,
		"digitalStamps": data2,
	})
}

func cmdDeliverySlots(ctx context.Context) {
	auth := mustAuth()

	// Create checkout session first
	sessionGql := `mutation CheckoutRetrieveSession($input: CheckoutCreateSessionInput) {
		createCheckoutSession(input: $input) {
			session {
				customerId
				deliveryAddress { street postalCode houseNumber city }
				fulfilmentType
			}
		}
	}`

	doGraphQL(ctx, auth, "CheckoutRetrieveSession", sessionGql, map[string]any{
		"input": map[string]any{"autofillSession": true},
	})

	// Then get delivery periods
	gql := `query CheckoutDeliveryPeriods($input: CheckoutDeliveryPeriodsInput) {
		checkoutDeliveryPeriods(input: $input) {
			deliveryDates {
				date
				deliveryPeriods {
					startDateTime endDateTime
					price { amount }
					isAvailable
					fulfilmentType
				}
			}
		}
	}`

	data, err := doGraphQL(ctx, auth, "CheckoutDeliveryPeriods", gql, map[string]any{
		"input": map[string]any{},
	})
	if err != nil {
		fatal("Delivery slots query failed: %v", err)
	}

	printJSON(data)
}

func cmdOrders(ctx context.Context) {
	auth := mustAuth()

	gql := `query OrdersPageOrders($input: OrdersInput) {
		orders(input: $input) {
			id orderDate status totalPrice { amount }
			fulfilmentType
		}
	}`

	data, err := doGraphQL(ctx, auth, "OrdersPageOrders", gql, map[string]any{
		"input": map[string]any{"first": 10},
	})
	if err != nil {
		fatal("Orders query failed: %v", err)
	}

	printJSON(data)
}

func cmdReceipts(ctx context.Context) {
	auth := mustAuth()

	gql := `query GetOnlineOrdersAndStoreReceipts {
		onlineOrdersAndStoreReceipts {
			items {
				... on OnlineOrder { id orderDate totalPrice { amount } status }
				... on StoreReceipt { id receiptDate totalPrice { amount } storeName }
			}
		}
	}`

	data, err := doGraphQL(ctx, auth, "GetOnlineOrdersAndStoreReceipts", gql, nil)
	if err != nil {
		fatal("Receipts query failed: %v", err)
	}

	printJSON(data)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	ctx := context.Background()
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "login":
		cmdLogin(ctx)
	case "search":
		cmdSearch(ctx, args)
	case "basket":
		cmdBasket(ctx)
	case "basket-add":
		cmdBasketAdd(ctx, args)
	case "basket-update":
		cmdBasketUpdate(ctx, args)
	case "basket-remove":
		cmdBasketRemove(ctx, args)
	case "promotions":
		cmdPromotions(ctx)
	case "stamps":
		cmdStamps(ctx)
	case "delivery-slots":
		cmdDeliverySlots(ctx)
	case "orders":
		cmdOrders(ctx)
	case "receipts":
		cmdReceipts(ctx)
	case "--help", "-h", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		usage()
	}
}
