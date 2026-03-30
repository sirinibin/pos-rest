package controller

import (
	"encoding/json"
	"net/http"
)

// ──────────────────────────────────────────────
// Minimal OpenAPI 3.1 type definitions
// ──────────────────────────────────────────────

type openAPISpec struct {
	OpenAPI    string                     `json:"openapi"`
	Info       openAPIInfo                `json:"info"`
	Servers    []openAPIServer            `json:"servers"`
	Security   []map[string][]string      `json:"security"`
	Components openAPIComponents          `json:"components"`
	Paths      map[string]openAPIPathItem `json:"paths"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type openAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type openAPIComponents struct {
	Schemas         map[string]interface{}      `json:"schemas"`
	SecuritySchemes map[string]openAPISecScheme `json:"securitySchemes"`
}

type openAPISecScheme struct {
	Type        string `json:"type"`
	In          string `json:"in"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type openAPIPathItem struct {
	Get    *openAPIOperation `json:"get,omitempty"`
	Post   *openAPIOperation `json:"post,omitempty"`
	Put    *openAPIOperation `json:"put,omitempty"`
	Delete *openAPIOperation `json:"delete,omitempty"`
}

type openAPIOperation struct {
	Summary     string                     `json:"summary"`
	Description string                     `json:"description,omitempty"`
	OperationID string                     `json:"operationId"`
	Parameters  []openAPIParam             `json:"parameters"`
	RequestBody *openAPIRequestBody        `json:"requestBody,omitempty"`
	Security    []map[string][]string      `json:"security"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

type openAPIParam struct {
	Name        string                 `json:"name"`
	In          string                 `json:"in"`
	Required    bool                   `json:"required"`
	Description string                 `json:"description,omitempty"`
	Schema      map[string]interface{} `json:"schema"`
	Example     interface{}            `json:"example,omitempty"`
}

type openAPIRequestBody struct {
	Required bool                        `json:"required"`
	Content  map[string]openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema map[string]interface{} `json:"schema"`
}

type openAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]openAPIMediaType `json:"content,omitempty"`
}

// ──────────────────────────────────────────────
// Reusable parameter helpers
// ──────────────────────────────────────────────

var authSecurity = []map[string][]string{{"BearerAuth": {}}}

func pathParam(name, description string) openAPIParam {
	return openAPIParam{
		Name:        name,
		In:          "path",
		Required:    true,
		Description: description,
		Schema:      map[string]interface{}{"type": "string"},
	}
}

func idPathParam() openAPIParam { return pathParam("id", "MongoDB ObjectID of the resource") }

func qParam(name, description string, required bool, example interface{}) openAPIParam {
	p := openAPIParam{
		Name:        name,
		In:          "query",
		Required:    required,
		Description: description,
		Schema:      map[string]interface{}{"type": "string"},
	}
	if example != nil {
		p.Example = example
	}
	return p
}

// Common query params shared by most list endpoints
func storeParam() openAPIParam {
	return qParam("search[store_id]",
		"REQUIRED. Use the store_id obtained from GET /v1/store/list result[0].id at session start. Always pass this — many fields (stock, prices) are stored per-store and will return wrong results without it.",
		true, nil)
}

func commonListParams() []openAPIParam {
	return []openAPIParam{
		storeParam(),
		qParam("page", "Page number (1-based)", false, 1),
		qParam("limit", "Number of records per page", false, 10),
		qParam("sort", "Sort field with optional direction prefix. Prefix with - for descending (newest first), no prefix for ascending (oldest first). Examples: sort=-created_at (default, latest first), sort=created_at (oldest first), sort=-date (latest date first), sort=date (earliest date first).", false, "-created_at"),
		qParam("select", "Comma-separated list of fields to include in the response. Omit to return all fields. See each endpoint description for the list of available fields.", false, "id,code,name"),
		qParam("search[timezone_offset]", "Timezone offset in hours, e.g. -3 for Saudi Arabia (UTC+3)", false, "-3"),
		qParam("search[date_str]", "Single date filter. Format: Jan 02 2006", false, "Mar 29 2026"),
		qParam("search[from_date]", "Start of date range. Format: Jan 02 2006", false, nil),
		qParam("search[to_date]", "End of date range. Format: Jan 02 2006", false, nil),
		qParam("search[created_at]", "Filter by exact creation date. Format: Jan 02 2006", false, nil),
		qParam("search[created_at_from]", "Creation date range start. Format: Jan 02 2006", false, nil),
		qParam("search[created_at_to]", "Creation date range end. Format: Jan 02 2006", false, nil),
	}
}

func searchCodeParam() openAPIParam {
	return qParam("search[code]", "Filter by code (partial match, case-insensitive)", false, nil)
}
func searchNameParam() openAPIParam {
	return qParam("search[name]", "Filter by name (partial match, case-insensitive)", false, nil)
}

func jsonBody() *openAPIRequestBody {
	return &openAPIRequestBody{
		Required: true,
		Content: map[string]openAPIMediaType{
			"application/json": {Schema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "additionalProperties": true}},
		},
	}
}

func okResp() map[string]openAPIResponse {
	return map[string]openAPIResponse{
		"200": {Description: "Success", Content: map[string]openAPIMediaType{
			"application/json": {Schema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "additionalProperties": true}},
		}},
		"401": {Description: "Unauthorized — invalid or missing access token"},
		"500": {Description: "Server error"},
	}
}

// ──────────────────────────────────────────────
// CRUD builder helpers
// ──────────────────────────────────────────────

// listOp builds a list/search GET operation with standard + extra params.
func listOp(resource, summary string, extra ...openAPIParam) *openAPIOperation {
	params := commonListParams()
	params = append(params, extra...)
	return &openAPIOperation{
		Summary:     summary,
		OperationID: "list_" + resource,
		Parameters:  params,
		Security:    authSecurity,
		Responses:   okResp(),
	}
}

// listOpDesc is like listOp but adds a description visible in the spec (use for field lists, operator notes, etc.).
func listOpDesc(resource, summary, description string, extra ...openAPIParam) *openAPIOperation {
	op := listOp(resource, summary, extra...)
	op.Description = description
	return op
}

// numParam builds a numeric filter query param that documents > < = comparison operator support.
func numParam(name, description string) openAPIParam {
	return qParam(name, description+". Supports comparison operators: prefix with > (greater than), < (less than), or = (exact). Examples: >100, <50, =0", false, nil)
}

// createOp builds a POST create operation.
func createOp(resource, summary string) *openAPIOperation {
	return &openAPIOperation{
		Summary:     summary,
		OperationID: "create_" + resource,
		Parameters:  []openAPIParam{},
		RequestBody: jsonBody(),
		Security:    authSecurity,
		Responses:   okResp(),
	}
}

// viewOp builds a GET /{id} view operation.
func viewOp(resource, summary string) *openAPIOperation {
	return &openAPIOperation{
		Summary:     summary,
		OperationID: "view_" + resource,
		Parameters:  []openAPIParam{idPathParam()},
		Security:    authSecurity,
		Responses:   okResp(),
	}
}

// updateOp builds a PUT /{id} update operation.
func updateOp(resource, summary string) *openAPIOperation {
	return &openAPIOperation{
		Summary:     summary,
		OperationID: "update_" + resource,
		Parameters:  []openAPIParam{idPathParam()},
		RequestBody: jsonBody(),
		Security:    authSecurity,
		Responses:   okResp(),
	}
}

// deleteOp builds a DELETE /{id} delete operation.
func deleteOp(resource, summary string) *openAPIOperation {
	return &openAPIOperation{
		Summary:     summary,
		OperationID: "delete_" + resource,
		Parameters:  []openAPIParam{idPathParam()},
		Security:    authSecurity,
		Responses:   okResp(),
	}
}

// ──────────────────────────────────────────────
// Spec builder
// ──────────────────────────────────────────────

func buildOpenAPISpec(baseURL string) openAPISpec {
	paths := map[string]openAPIPathItem{}

	// ── /v1/me ──────────────────────────────────
	paths["/v1/me"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "Get Current User Details",
			Description: "Returns the profile of the currently logged-in user (name, role, store assignments, etc.).",
			OperationID: "get_current_user_details",
			Parameters:  []openAPIParam{},
			Security:    authSecurity,
			Responses: map[string]openAPIResponse{
				"200": {
					Description: "Authenticated user profile",
					Content: map[string]openAPIMediaType{
						"application/json": {
							Schema: map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"result": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"role": map[string]interface{}{
												"type":        "string",
												"description": "User role: Admin or Staff",
											},
											"store_ids": map[string]interface{}{
												"type":  "array",
												"items": map[string]interface{}{"type": "string"},
											},
										},
									},
								},
							},
						},
					},
				},
				"401": {Description: "Unauthorized"},
			},
		},
	}

	// ── /v1/register ────────────────────────────
	paths["/v1/register"] = openAPIPathItem{
		Post: createOp("user_account", "Register a new user account"),
	}

	// ── Auth ─────────────────────────────────────
	paths["/v1/authorize"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Authorize — obtain auth_code",
			OperationID: "authorize",
			Parameters:  []openAPIParam{},
			RequestBody: jsonBody(),
			Security:    []map[string][]string{},
			Responses:   okResp(),
		},
	}
	paths["/v1/accesstoken"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Exchange auth_code for access token",
			OperationID: "get_access_token",
			Parameters:  []openAPIParam{},
			RequestBody: jsonBody(),
			Security:    []map[string][]string{},
			Responses:   okResp(),
		},
	}
	paths["/v1/refresh"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Refresh access token using refresh token",
			OperationID: "refresh_access_token",
			Parameters:  []openAPIParam{},
			RequestBody: jsonBody(),
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}
	paths["/v1/logout"] = openAPIPathItem{
		Delete: &openAPIOperation{
			Summary:     "Logout — revoke access token",
			OperationID: "logout",
			Parameters:  []openAPIParam{},
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}

	// ── Store ────────────────────────────────────
	storeListParams := []openAPIParam{
		qParam("search[name]", "Filter by store name (partial match, case-insensitive)", false, nil),
		qParam("search[branch_name]", "Filter by branch name (partial match, case-insensitive)", false, nil),
		searchCodeParam(),
		qParam("search[email]", "Filter by email (partial match, case-insensitive)", false, nil),
	}
	paths["/v1/store"] = openAPIPathItem{
		Get:  listOp("store", "List / Search Stores", storeListParams...),
		Post: createOp("store", "Create a new Store"),
	}
	paths["/v1/store/list"] = openAPIPathItem{
		Get: func() *openAPIOperation {
			op := listOp("store_list", "List Stores (slim)", storeListParams...)
			op.Description = "Call this at session start. " +
				"Returns id, name, name_in_arabic, code, branch_name, vat_no only. " +
				"Silently use result[0].id as the active store_id for all subsequent requests. " +
				"Do NOT show this list to the user and do NOT ask them to choose a store."
			return op
		}(),
	}
	paths["/v1/store/{id}"] = openAPIPathItem{
		Get:    viewOp("store", "View Store by ID"),
		Put:    updateOp("store", "Update Store"),
		Delete: deleteOp("store", "Delete Store"),
	}

	// ── Warehouse ────────────────────────────────
	paths["/v1/warehouse"] = openAPIPathItem{
		Get: listOp("warehouse", "List / Search Warehouses",
			searchNameParam(), searchCodeParam()),
		Post: createOp("warehouse", "Create Warehouse"),
	}
	paths["/v1/warehouse/{id}"] = openAPIPathItem{
		Get:    viewOp("warehouse", "View Warehouse"),
		Put:    updateOp("warehouse", "Update Warehouse"),
		Delete: deleteOp("warehouse", "Delete Warehouse"),
	}

	// ── Customer ─────────────────────────────────
	paths["/v1/customer"] = openAPIPathItem{
		Get: listOpDesc("customer", "List / Search Customers",
			"Available select fields: id,name,name_in_arabic,code,phone,phone2,vat_no,credit_balance,credit_limit,sales_count,sales_amount,sales_balance_amount,sales_not_paid_count,remarks,deleted. Numeric filter params support > < = operators.",
			searchNameParam(), searchCodeParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[phone]", "Filter by phone", false, nil),
			qParam("search[vat_no]", "Filter by VAT number", false, nil),
			qParam("search[query]", "General text search across name/code/email/phone", false, nil),
			numParam("search[credit_balance]", "Filter by credit balance amount"),
			numParam("search[credit_limit]", "Filter by credit limit"),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
		),
		Post: createOp("customer", "Create Customer"),
	}
	paths["/v1/customer/{id}"] = openAPIPathItem{
		Get:    viewOp("customer", "View Customer"),
		Put:    updateOp("customer", "Update Customer"),
		Delete: deleteOp("customer", "Delete Customer"),
	}
	paths["/v1/customer/restore/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Restore deleted Customer",
			OperationID: "restore_customer",
			Parameters:  []openAPIParam{pathParam("id", "Customer ID to restore")},
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}
	paths["/v1/customer/vat_no/name"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "Find Customer by VAT number or name",
			OperationID: "view_customer_by_vat_or_name",
			Parameters: []openAPIParam{
				qParam("vat_no", "VAT number", false, nil),
				qParam("name", "Customer name", false, nil),
			},
			Security:  authSecurity,
			Responses: okResp(),
		},
	}

	// ── Product ──────────────────────────────────
	paths["/v1/product"] = openAPIPathItem{
		Get: listOpDesc("product", "List / Search Products",
			"Available select fields: id,name,name_in_arabic,code,barcode,ean_12,part_number,category_id,category_name,brand_id,brand_name,unit,rack,stock,retail_unit_price,wholesale_unit_price,purchase_unit_price,retail_unit_profit,wholesale_unit_profit,sales_count,sales_quantity,purchase_count,purchase_quantity,is_set,deleted,product_warehouses,warehouse_stocks,images. Numeric filter params support > < = operators (e.g. search[stock]=>10).",
			searchNameParam(), searchCodeParam(),
			qParam("search[search_text]", "General text search across name/code/barcode/item_code", false, nil),
			qParam("search[item_code]", "Filter by item code (exact)", false, nil),
			qParam("search[bar_code]", "Filter by barcode value", false, nil),
			qParam("search[ean_12]", "Filter by EAN-12 barcode", false, nil),
			qParam("search[part_number]", "Filter by part number", false, nil),
			qParam("search[category_id]", "Filter by product category ID", false, nil),
			qParam("search[brand_id]", "Filter by product brand ID", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code (requires search[store_id])", false, nil),
			numParam("search[stock]", "Filter by stock quantity (requires search[store_id])"),
			qParam("search[rack]", "Filter by rack/shelf location", false, nil),
			numParam("search[retail_unit_price]", "Filter by retail unit price (requires search[store_id])"),
			numParam("search[wholesale_unit_price]", "Filter by wholesale unit price (requires search[store_id])"),
			numParam("search[retail_unit_profit]", "Filter by retail unit profit (requires search[store_id])"),
			numParam("search[wholesale_unit_profit]", "Filter by wholesale unit profit (requires search[store_id])"),
			numParam("search[purchase_unit_price]", "Filter by purchase unit price"),
			numParam("search[sales_count]", "Filter by number of times sold (requires search[store_id])"),
			numParam("search[sales_quantity]", "Filter by total quantity sold (requires search[store_id])"),
			numParam("search[purchase_count]", "Filter by number of purchases"),
			numParam("search[purchase_quantity]", "Filter by total quantity purchased"),
			qParam("search[is_set]", "Filter set/bundle products: 1=sets only, 0=non-sets", false, nil),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
			qParam("search[ids]", "Filter by comma-separated list of product IDs", false, nil),
			qParam("search[stats]", "Pass 1 to include per-store stock/sales stats in response (requires search[store_id])", false, nil),
		),
		Post: createOp("product", "Create Product"),
	}
	paths["/v1/product/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "View Product by ID",
			OperationID: "view_product",
			Parameters: []openAPIParam{
				idPathParam(),
				storeParam(),
				qParam("search[warehouse_code]", "Return stock for a specific warehouse (requires search[store_id])", false, nil),
			},
			Security:  authSecurity,
			Responses: okResp(),
		},
		Put:    updateOp("product", "Update Product"),
		Delete: deleteOp("product", "Delete Product"),
	}
	paths["/v1/product/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "View Product by item code",
			OperationID: "view_product_by_code",
			Parameters: []openAPIParam{
				pathParam("code", "Product item code"),
				storeParam(),
				qParam("search[warehouse_code]", "Return stock for a specific warehouse", false, nil),
			},
			Security:  authSecurity,
			Responses: okResp(),
		},
	}
	paths["/v1/product/barcode/{barcode}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "View Product by barcode",
			OperationID: "view_product_by_barcode",
			Parameters: []openAPIParam{
				pathParam("barcode", "Product barcode"),
				storeParam(),
				qParam("search[warehouse_code]", "Return stock for a specific warehouse", false, nil),
			},
			Security:  authSecurity,
			Responses: okResp(),
		},
	}
	paths["/v1/product/history/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "List Product change history",
			OperationID: "list_product_history",
			Parameters: []openAPIParam{
				pathParam("id", "Product ID"),
				storeParam(),
			},
			Security:  authSecurity,
			Responses: okResp(),
		},
	}
	paths["/v1/product/restore/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Restore deleted Product",
			OperationID: "restore_product",
			Parameters:  []openAPIParam{pathParam("id", "Product ID to restore")},
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}

	// ── Product Category ─────────────────────────
	paths["/v1/product-category"] = openAPIPathItem{
		Get:  listOp("product_category", "List / Search Product Categories", searchNameParam(), searchCodeParam()),
		Post: createOp("product_category", "Create Product Category"),
	}
	paths["/v1/product-category/{id}"] = openAPIPathItem{
		Get:    viewOp("product_category", "View Product Category"),
		Put:    updateOp("product_category", "Update Product Category"),
		Delete: deleteOp("product_category", "Delete Product Category"),
	}
	paths["/v1/product-category/restore/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Restore deleted Product Category", OperationID: "restore_product_category",
			Parameters: []openAPIParam{pathParam("id", "Product Category ID")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Product Brand ────────────────────────────
	paths["/v1/product-brand"] = openAPIPathItem{
		Get:  listOp("product_brand", "List / Search Product Brands", searchNameParam(), searchCodeParam()),
		Post: createOp("product_brand", "Create Product Brand"),
	}
	paths["/v1/product-brand/{id}"] = openAPIPathItem{
		Get:    viewOp("product_brand", "View Product Brand"),
		Put:    updateOp("product_brand", "Update Product Brand"),
		Delete: deleteOp("product_brand", "Delete Product Brand"),
	}
	paths["/v1/product-brand/restore/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Restore deleted Product Brand", OperationID: "restore_product_brand",
			Parameters: []openAPIParam{pathParam("id", "Product Brand ID")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Expense Category ─────────────────────────
	paths["/v1/expense-category"] = openAPIPathItem{
		Get:  listOp("expense_category", "List / Search Expense Categories", searchNameParam(), searchCodeParam()),
		Post: createOp("expense_category", "Create Expense Category"),
	}
	paths["/v1/expense-category/{id}"] = openAPIPathItem{
		Get:    viewOp("expense_category", "View Expense Category"),
		Put:    updateOp("expense_category", "Update Expense Category"),
		Delete: deleteOp("expense_category", "Delete Expense Category"),
	}

	// ── Expense ───────────────────────────────────
	paths["/v1/expense"] = openAPIPathItem{
		Get: listOp("expense", "List / Search Expenses",
			searchCodeParam(),
			qParam("search[category_id]", "Filter by expense category ID", false, nil),
			qParam("search[amount]", "Filter by amount", false, nil),
		),
		Post: createOp("expense", "Create Expense"),
	}
	paths["/v1/expense/{id}"] = openAPIPathItem{
		Get:    viewOp("expense", "View Expense"),
		Put:    updateOp("expense", "Update Expense"),
		Delete: deleteOp("expense", "Delete Expense"),
	}
	paths["/v1/expense/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View Expense by code", OperationID: "view_expense_by_code",
			Parameters: []openAPIParam{pathParam("code", "Expense code")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Customer Deposit ─────────────────────────
	paths["/v1/customer-deposit"] = openAPIPathItem{
		Get:  listOp("customer_deposit", "List / Search Customer Deposits", searchCodeParam()),
		Post: createOp("customer_deposit", "Create Customer Deposit"),
	}
	paths["/v1/customer-deposit/{id}"] = openAPIPathItem{
		Get:    viewOp("customer_deposit", "View Customer Deposit"),
		Put:    updateOp("customer_deposit", "Update Customer Deposit"),
		Delete: deleteOp("customer_deposit", "Delete Customer Deposit"),
	}
	paths["/v1/customer-deposit/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View Customer Deposit by code", OperationID: "view_customer_deposit_by_code",
			Parameters: []openAPIParam{pathParam("code", "Deposit code")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Customer Withdrawal ─────────────────────
	paths["/v1/customer-withdrawal"] = openAPIPathItem{
		Get:  listOp("customer_withdrawal", "List / Search Customer Withdrawals", searchCodeParam()),
		Post: createOp("customer_withdrawal", "Create Customer Withdrawal"),
	}
	paths["/v1/customer-withdrawal/{id}"] = openAPIPathItem{
		Get:    viewOp("customer_withdrawal", "View Customer Withdrawal"),
		Put:    updateOp("customer_withdrawal", "Update Customer Withdrawal"),
		Delete: deleteOp("customer_withdrawal", "Delete Customer Withdrawal"),
	}
	paths["/v1/customer-withdrawal/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View Customer Withdrawal by code", OperationID: "view_customer_withdrawal_by_code",
			Parameters: []openAPIParam{pathParam("code", "Withdrawal code")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Capital Withdrawal ──────────────────────
	paths["/v1/capital-withdrawal"] = openAPIPathItem{
		Get:  listOp("capital_withdrawal", "List / Search Capital Withdrawals", searchCodeParam()),
		Post: createOp("capital_withdrawal", "Create Capital Withdrawal"),
	}
	paths["/v1/capital-withdrawal/{id}"] = openAPIPathItem{
		Get:    viewOp("capital_withdrawal", "View Capital Withdrawal"),
		Put:    updateOp("capital_withdrawal", "Update Capital Withdrawal"),
		Delete: deleteOp("capital_withdrawal", "Delete Capital Withdrawal"),
	}
	paths["/v1/capital-withdrawal/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View Capital Withdrawal by code", OperationID: "view_capital_withdrawal_by_code",
			Parameters: []openAPIParam{pathParam("code", "Capital Withdrawal code")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Capital ──────────────────────────────────
	paths["/v1/capital"] = openAPIPathItem{
		Get:  listOp("capital", "List / Search Capitals", searchCodeParam()),
		Post: createOp("capital", "Create Capital"),
	}
	paths["/v1/capital/{id}"] = openAPIPathItem{
		Get:    viewOp("capital", "View Capital"),
		Put:    updateOp("capital", "Update Capital"),
		Delete: deleteOp("capital", "Delete Capital"),
	}
	paths["/v1/capital/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View Capital by code", OperationID: "view_capital_by_code",
			Parameters: []openAPIParam{pathParam("code", "Capital code")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Divident ─────────────────────────────────
	paths["/v1/divident"] = openAPIPathItem{
		Get:  listOp("divident", "List / Search Dividents", searchCodeParam()),
		Post: createOp("divident", "Create Divident"),
	}
	paths["/v1/divident/{id}"] = openAPIPathItem{
		Get:    viewOp("divident", "View Divident"),
		Put:    updateOp("divident", "Update Divident"),
		Delete: deleteOp("divident", "Delete Divident"),
	}
	paths["/v1/divident/code/{code}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View Divident by code", OperationID: "view_divident_by_code",
			Parameters: []openAPIParam{pathParam("code", "Divident code")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── User ─────────────────────────────────────
	paths["/v1/user"] = openAPIPathItem{
		Get: listOp("user", "List / Search Users",
			searchNameParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[role]", "Filter by role (Admin, Staff, etc.)", false, nil),
		),
		Post: createOp("user", "Create User"),
	}
	paths["/v1/user/{id}"] = openAPIPathItem{
		Get:    viewOp("user", "View User"),
		Put:    updateOp("user", "Update User"),
		Delete: deleteOp("user", "Delete User"),
	}

	// ── Signature ────────────────────────────────
	paths["/v1/signature"] = openAPIPathItem{
		Get:  listOp("signature", "List / Search Signatures"),
		Post: createOp("signature", "Create Signature"),
	}
	paths["/v1/signature/{id}"] = openAPIPathItem{
		Get:    viewOp("signature", "View Signature"),
		Put:    updateOp("signature", "Update Signature"),
		Delete: deleteOp("signature", "Delete Signature"),
	}

	// ── Quotation ────────────────────────────────
	paths["/v1/quotation"] = openAPIPathItem{
		Get: listOpDesc("quotation", "List / Search Quotations",
			"Available select fields: id,code,date,customer_id,customer_name,customer_name_arabic,net_total,total_with_vat,vat_price,discount,type,profit,loss,invoice_count,invoice_net_total,remarks,created_at. Numeric filter params support > < = operators.",
			searchCodeParam(),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[status]", "Filter by status", false, nil),
			qParam("search[type]", "Filter by quotation type", false, nil),
			qParam("search[payment_status]", "Filter by payment status", false, nil),
			qParam("search[payment_methods]", "Filter by payment methods (comma-separated)", false, nil),
			qParam("search[net_total]", "Filter by net total amount", false, nil),
			qParam("search[discount]", "Filter by discount amount", false, nil),
			qParam("search[order_code]", "Filter by related order code", false, nil),
			qParam("search[reported_to_zatca]", "Filter by ZATCA reporting status", false, nil),
			qParam("search[stats]", "Pass 1 to include stats in response", false, nil),
		),
		Post: createOp("quotation", "Create Quotation"),
	}
	paths["/v1/quotation/{id}"] = openAPIPathItem{
		Get:    viewOp("quotation", "View Quotation"),
		Put:    updateOp("quotation", "Update Quotation"),
		Delete: deleteOp("quotation", "Delete Quotation"),
	}
	paths["/v1/quotation/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Calculate Quotation Net Total",
			OperationID: "calculate_quotation_net_total",
			Parameters:  []openAPIParam{},
			RequestBody: jsonBody(),
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}
	paths["/v1/quotation/history"] = openAPIPathItem{
		Get: listOp("quotation_history", "List Quotation History",
			qParam("search[product_id]", "Filter by product ID", false, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[quotation_id]", "Filter by quotation ID", false, nil),
			qParam("search[quotation_code]", "Filter by quotation code", false, nil),
			qParam("search[type]", "Filter by quotation type", false, nil),
			qParam("search[payment_status]", "Filter by payment status", false, nil),
			qParam("search[quantity]", "Filter by quantity", false, nil),
			qParam("search[price]", "Filter by price", false, nil),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
	}

	// ── Delivery Note ────────────────────────────
	paths["/v1/delivery-note"] = openAPIPathItem{
		Get: listOp("delivery_note", "List / Search Delivery Notes",
			searchCodeParam(),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
		),
		Post: createOp("delivery_note", "Create Delivery Note"),
	}
	paths["/v1/delivery-note/{id}"] = openAPIPathItem{
		Get: viewOp("delivery_note", "View Delivery Note"),
		Put: updateOp("delivery_note", "Update Delivery Note"),
	}
	paths["/v1/delivery-note/history"] = openAPIPathItem{
		Get: listOp("delivery_note_history", "List Delivery Note History"),
	}

	// ── Stock Transfer ───────────────────────────
	paths["/v1/stock-transfer"] = openAPIPathItem{
		Get: listOp("stock_transfer", "List / Search Stock Transfers",
			searchCodeParam(),
			qParam("search[from_warehouse_id]", "Filter by source warehouse ID", false, nil),
			qParam("search[to_warehouse_id]", "Filter by destination warehouse ID", false, nil),
		),
		Post: createOp("stock_transfer", "Create Stock Transfer"),
	}
	paths["/v1/stock-transfer/{id}"] = openAPIPathItem{
		Get: viewOp("stock_transfer", "View Stock Transfer"),
		Put: updateOp("stock_transfer", "Update Stock Transfer"),
	}
	paths["/v1/stock-transfer/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Calculate Stock Transfer Net Total",
			OperationID: "calculate_stock_transfer_net_total",
			Parameters:  []openAPIParam{},
			RequestBody: jsonBody(),
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}
	paths["/v1/stock-transfer/history"] = openAPIPathItem{
		Get: listOp("stock_transfer_history", "List Stock Transfer History"),
	}
	paths["/v1/previous-stock-transfer/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View previous Stock Transfer", OperationID: "view_previous_stock_transfer",
			Parameters: []openAPIParam{pathParam("id", "Current Stock Transfer ID")}, Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/next-stock-transfer/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View next Stock Transfer", OperationID: "view_next_stock_transfer",
			Parameters: []openAPIParam{pathParam("id", "Current Stock Transfer ID")}, Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/last-stock-transfer"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View last Stock Transfer", OperationID: "view_last_stock_transfer",
			Parameters: []openAPIParam{storeParam()}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Sales (Orders) ───────────────────────────
	paths["/v1/order"] = openAPIPathItem{
		Get: listOpDesc("order", "List / Search Sales Orders",
			"Available select fields: id,code,date,customer_id,customer_name,customer_name_arabic,net_total,total_with_vat,vat_price,discount,payment_status,payment_methods,balance_amount,profit,loss,cash_sales,bank_account_sales,return_count,return_amount,remarks,created_at. Numeric filter params support > < = operators (e.g. search[net_total]=>1000).",
			searchCodeParam(),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[payment_method]", "Filter by payment method: cash, bank_account", false, nil),
			qParam("search[payment_methods]", "Filter by multiple payment methods (comma-separated)", false, nil),
			qParam("search[status]", "Filter by order status", false, nil),
			numParam("search[net_total]", "Filter by net total amount"),
			numParam("search[balance_amount]", "Filter by outstanding balance"),
			numParam("search[discount]", "Filter by discount amount"),
			numParam("search[return_count]", "Filter by number of returns"),
			numParam("search[return_amount]", "Filter by return amount"),
			qParam("search[delivered_by]", "Filter by delivered-by user ID", false, nil),
			qParam("search[stats]", "Pass 1 to include sales stats in response", false, nil),
			qParam("search[zatca.reporting_passed]", "ZATCA status: reported, reporting_failed, not_reported, compliance_passed, compliance_failed", false, nil),
		),
		Post: createOp("order", "Create Sales Order"),
	}
	paths["/v1/order/{id}"] = openAPIPathItem{
		Get: viewOp("order", "View Sales Order"),
		Put: updateOp("order", "Update Sales Order"),
	}
	paths["/v1/order/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary:     "Calculate Sales Net Total",
			OperationID: "calculate_sales_net_total",
			Parameters:  []openAPIParam{},
			RequestBody: jsonBody(),
			Security:    authSecurity,
			Responses:   okResp(),
		},
	}
	paths["/v1/previous-order/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View previous Order", OperationID: "view_previous_order",
			Parameters: []openAPIParam{pathParam("id", "Current Order ID")}, Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/next-order/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View next Order", OperationID: "view_next_order",
			Parameters: []openAPIParam{pathParam("id", "Current Order ID")}, Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/last-order"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary: "View last Order", OperationID: "view_last_order",
			Parameters: []openAPIParam{storeParam()}, Security: authSecurity, Responses: okResp(),
		},
	}
	// Sales Summary
	paths["/v1/sales/summary"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "Get Sales Summary / Statistics",
			Description: "Returns aggregated sales stats: net total, profit, VAT, discounts, paid/unpaid, cash/bank. Always pass the store_id obtained from GET /v1/store/list result[0].id at session start. Never ask the user to choose a store.",
			OperationID: "get_sales_summary",
			Parameters: []openAPIParam{
				{
					Name:        "search[store_id]",
					In:          "query",
					Required:    false,
					Description: "Store ID. Use result[0].id from GET /v1/store/list obtained at session start. Never ask the user to choose a store.",
					Schema:      map[string]interface{}{"type": "string"},
				},
				qParam("search[timezone_offset]", "Timezone offset. e.g. -3 for Saudi Arabia (UTC+3). Compute via JS: parseFloat(new Date().getTimezoneOffset()/60)", false, "-3"),
				qParam("search[date_str]", "Single day filter. Format: Jan 02 2006. e.g. Mar 29 2026", false, "Mar 29 2026"),
				qParam("search[from_date]", "Date range start. Format: Jan 02 2006", false, nil),
				qParam("search[to_date]", "Date range end. Format: Jan 02 2006", false, nil),
				qParam("search[customer_id]", "Filter by customer ID", false, nil),
				qParam("search[payment_status]", "Filter by payment status", false, nil),
				qParam("search[code]", "Filter by order code", false, nil),
				qParam("search[zatca.reporting_passed]", "ZATCA status filter", false, nil),
			},
			Security: authSecurity,
			Responses: map[string]openAPIResponse{
				"200": {
					Description: "Sales statistics",
					Content: map[string]openAPIMediaType{
						"application/json": {
							Schema: map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"status": map[string]interface{}{"type": "boolean"},
									"result": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"net_total":              map[string]interface{}{"type": "number"},
											"net_profit":             map[string]interface{}{"type": "number"},
											"net_loss":               map[string]interface{}{"type": "number"},
											"vat_price":              map[string]interface{}{"type": "number"},
											"discount":               map[string]interface{}{"type": "number"},
											"cash_discount":          map[string]interface{}{"type": "number"},
											"paid_sales":             map[string]interface{}{"type": "number"},
											"unpaid_sales":           map[string]interface{}{"type": "number"},
											"cash_sales":             map[string]interface{}{"type": "number"},
											"bank_account_sales":     map[string]interface{}{"type": "number"},
											"return_count":           map[string]interface{}{"type": "integer"},
											"return_amount":          map[string]interface{}{"type": "number"},
											"commission":             map[string]interface{}{"type": "number"},
											"shipping_handling_fees": map[string]interface{}{"type": "number"},
										},
									},
									"total_count": map[string]interface{}{"type": "integer"},
								},
							},
						},
					},
				},
				"401": {Description: "Unauthorized"},
			},
		},
	}
	paths["/v1/sales/history"] = openAPIPathItem{
		Get: listOp("sales_history", "List Sales History",
			qParam("search[product_id]", "Filter by product ID", false, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[order_id]", "Filter by sales order ID", false, nil),
			qParam("search[order_code]", "Filter by sales order code", false, nil),
			qParam("search[quantity]", "Filter by quantity", false, nil),
			qParam("search[price]", "Filter by price", false, nil),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			qParam("search[discount]", "Filter by discount", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
	}

	// ── Sales Return ─────────────────────────────
	paths["/v1/sales-return"] = openAPIPathItem{
		Get: listOpDesc("sales_return", "List / Search Sales Returns",
			"Available select fields: id,code,date,customer_id,customer_name,customer_name_arabic,net_total,total_with_vat,vat_price,discount,payment_status,payment_methods,balance_amount,profit,loss,cash_sales_return,bank_account_sales_return,remarks,created_at. Numeric filter params support > < = operators.",
			searchCodeParam(),
			qParam("search[order_id]", "Filter by original sales order ID", false, nil),
			qParam("search[order_code]", "Filter by original sales order code", false, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[payment_methods]", "Filter by payment methods (comma-separated)", false, nil),
			qParam("search[status]", "Filter by return status", false, nil),
			numParam("search[net_total]", "Filter by net total amount"),
			numParam("search[balance_amount]", "Filter by outstanding balance"),
			numParam("search[discount]", "Filter by discount amount"),
			qParam("search[received_by]", "Filter by received-by user ID", false, nil),
			qParam("search[zatca.reporting_passed]", "ZATCA reporting status filter", false, nil),
		),
		Post: createOp("sales_return", "Create Sales Return"),
	}
	paths["/v1/sales-return/{id}"] = openAPIPathItem{
		Get: viewOp("sales_return", "View Sales Return"),
		Put: updateOp("sales_return", "Update Sales Return"),
	}
	paths["/v1/sales-return/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Calculate Sales Return Net Total", OperationID: "calculate_sales_return_net_total",
			Parameters: []openAPIParam{}, RequestBody: jsonBody(), Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/sales-return/summary"] = openAPIPathItem{
		Get: listOp("sales_return_summary", "Get Sales Return Summary"),
	}
	paths["/v1/sales-return/history"] = openAPIPathItem{
		Get: listOp("sales_return_history", "List Sales Return History",
			qParam("search[product_id]", "Filter by product ID", false, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[sales_return_id]", "Filter by sales return ID", false, nil),
			qParam("search[sales_return_code]", "Filter by sales return code", false, nil),
			qParam("search[order_id]", "Filter by related sales order ID", false, nil),
			qParam("search[order_code]", "Filter by related sales order code", false, nil),
			qParam("search[quantity]", "Filter by quantity", false, nil),
			qParam("search[price]", "Filter by price", false, nil),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			qParam("search[discount]", "Filter by discount", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
	}

	// ── Quotation Sales Return ────────────────────
	paths["/v1/quotation-sales-return"] = openAPIPathItem{
		Get: listOpDesc("quotation_sales_return", "List / Search Quotation Sales Returns",
			"Available select fields: id,code,date,customer_id,customer_name,customer_name_arabic,net_total,total_with_vat,vat_price,discount,payment_status,payment_methods,balance_amount,profit,loss,created_at. Numeric filter params support > < = operators.",
			searchCodeParam(),
			qParam("search[quotation_id]", "Filter by quotation ID", false, nil),
			qParam("search[quotation_code]", "Filter by quotation code", false, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[payment_methods]", "Filter by payment methods (comma-separated)", false, nil),
			qParam("search[status]", "Filter by return status", false, nil),
			numParam("search[net_total]", "Filter by net total amount"),
			numParam("search[balance_amount]", "Filter by outstanding balance"),
			numParam("search[discount]", "Filter by discount amount"),
			qParam("search[zatca.reporting_passed]", "ZATCA reporting status filter", false, nil),
		),
		Post: createOp("quotation_sales_return", "Create Quotation Sales Return"),
	}
	paths["/v1/quotation-sales-return/{id}"] = openAPIPathItem{
		Get: viewOp("quotation_sales_return", "View Quotation Sales Return"),
		Put: updateOp("quotation_sales_return", "Update Quotation Sales Return"),
	}
	paths["/v1/quotation-sales-return/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Calculate Quotation Sales Return Net Total", OperationID: "calculate_quotation_sales_return_net_total",
			Parameters: []openAPIParam{}, RequestBody: jsonBody(), Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/quotation-sales-return/history"] = openAPIPathItem{
		Get: listOp("quotation_sales_return_history", "List Quotation Sales Return History"),
	}

	// ── Vendor ───────────────────────────────────
	paths["/v1/vendor"] = openAPIPathItem{
		Get: listOpDesc("vendor", "List / Search Vendors",
			"Available select fields: id,name,name_in_arabic,code,phone,phone2,email,vat_no,credit_balance,credit_limit,purchase_count,purchase_amount,purchase_balance_amount,purchase_not_paid_count,deleted. Numeric filter params support > < = operators.",
			searchNameParam(), searchCodeParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[phone]", "Filter by phone", false, nil),
			qParam("search[vat_no]", "Filter by VAT number", false, nil),
			qParam("search[query]", "General text search across name/code/email/phone", false, nil),
			numParam("search[credit_balance]", "Filter by credit balance amount"),
			numParam("search[credit_limit]", "Filter by credit limit"),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
		),
		Post: createOp("vendor", "Create Vendor"),
	}
	paths["/v1/vendor/{id}"] = openAPIPathItem{
		Get:    viewOp("vendor", "View Vendor"),
		Put:    updateOp("vendor", "Update Vendor"),
		Delete: deleteOp("vendor", "Delete Vendor"),
	}
	paths["/v1/vendor/restore/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Restore deleted Vendor", OperationID: "restore_vendor",
			Parameters: []openAPIParam{pathParam("id", "Vendor ID to restore")}, Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/vendor/vat_no/name"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "Find Vendor by VAT number or name",
			OperationID: "view_vendor_by_vat_or_name",
			Parameters: []openAPIParam{
				qParam("vat_no", "VAT number", false, nil),
				qParam("name", "Vendor name", false, nil),
			},
			Security:  authSecurity,
			Responses: okResp(),
		},
	}

	// ── Purchase ─────────────────────────────────
	paths["/v1/purchase"] = openAPIPathItem{
		Get: listOpDesc("purchase", "List / Search Purchases",
			"Available select fields: id,code,date,vendor_id,vendor_name,vendor_name_arabic,net_total,total_with_vat,vat_price,discount,payment_status,payment_methods,balance_amount,purchase_quantity,remarks,created_at. Numeric filter params support > < = operators.",
			searchCodeParam(),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[vendor_invoice_no]", "Filter by vendor invoice number", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[payment_methods]", "Filter by payment methods (comma-separated)", false, nil),
			qParam("search[status]", "Filter by purchase status", false, nil),
			numParam("search[net_total]", "Filter by net total amount"),
			numParam("search[balance_amount]", "Filter by outstanding balance"),
			numParam("search[discount]", "Filter by discount amount"),
			qParam("search[delivered_by]", "Filter by delivered-by user ID", false, nil),
			qParam("search[stats]", "Pass 1 to include purchase stats in response", false, nil),
		),
		Post: createOp("purchase", "Create Purchase"),
	}
	paths["/v1/purchase/{id}"] = openAPIPathItem{
		Get:    viewOp("purchase", "View Purchase"),
		Put:    updateOp("purchase", "Update Purchase"),
		Delete: deleteOp("purchase", "Delete Purchase"),
	}
	paths["/v1/purchase/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Calculate Purchase Net Total", OperationID: "calculate_purchase_net_total",
			Parameters: []openAPIParam{}, RequestBody: jsonBody(), Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/purchase/history"] = openAPIPathItem{
		Get: listOp("purchase_history", "List Purchase History",
			qParam("search[product_id]", "Filter by product ID", false, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_id]", "Filter by purchase ID", false, nil),
			qParam("search[purchase_code]", "Filter by purchase code", false, nil),
			qParam("search[quantity]", "Filter by quantity", false, nil),
			qParam("search[price]", "Filter by price", false, nil),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[net_price]", "Filter by net price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			qParam("search[discount]", "Filter by discount", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
	}

	// ── Purchase Return ──────────────────────────
	paths["/v1/purchase-return"] = openAPIPathItem{
		Get: listOpDesc("purchase_return", "List / Search Purchase Returns",
			"Available select fields: id,code,date,vendor_id,vendor_name,vendor_name_arabic,net_total,total_with_vat,vat_price,discount,payment_status,payment_methods,balance_amount,purchase_return_quantity,remarks,created_at. Numeric filter params support > < = operators.",
			searchCodeParam(),
			qParam("search[purchase_id]", "Filter by original purchase ID", false, nil),
			qParam("search[purchase_code]", "Filter by original purchase code", false, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[vendor_invoice_no]", "Filter by vendor invoice number", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[payment_methods]", "Filter by payment methods (comma-separated)", false, nil),
			qParam("search[status]", "Filter by return status", false, nil),
			numParam("search[net_total]", "Filter by net total amount"),
			numParam("search[balance_amount]", "Filter by outstanding balance"),
			numParam("search[discount]", "Filter by discount amount"),
		),
		Post: createOp("purchase_return", "Create Purchase Return"),
	}
	paths["/v1/purchase-return/{id}"] = openAPIPathItem{
		Get:    viewOp("purchase_return", "View Purchase Return"),
		Put:    updateOp("purchase_return", "Update Purchase Return"),
		Delete: deleteOp("purchase_return", "Delete Purchase Return"),
	}
	paths["/v1/purchase-return/calculate-net-total"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Calculate Purchase Return Net Total", OperationID: "calculate_purchase_return_net_total",
			Parameters: []openAPIParam{}, RequestBody: jsonBody(), Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/purchase-return/history"] = openAPIPathItem{
		Get: listOp("purchase_return_history", "List Purchase Return History",
			qParam("search[product_id]", "Filter by product ID", false, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_return_id]", "Filter by purchase return ID", false, nil),
			qParam("search[purchase_return_code]", "Filter by purchase return code", false, nil),
			qParam("search[purchase_id]", "Filter by original purchase ID", false, nil),
			qParam("search[purchase_code]", "Filter by original purchase code", false, nil),
			qParam("search[quantity]", "Filter by quantity", false, nil),
			qParam("search[price]", "Filter by price", false, nil),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[net_price]", "Filter by net price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			qParam("search[discount]", "Filter by discount", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
	}

	// ── Purchase Cash Discount ───────────────────
	paths["/v1/purchase-cash-discount"] = openAPIPathItem{
		Get:  listOp("purchase_cash_discount", "List / Search Purchase Cash Discounts", searchCodeParam()),
		Post: createOp("purchase_cash_discount", "Create Purchase Cash Discount"),
	}
	paths["/v1/purchase-cash-discount/{id}"] = openAPIPathItem{
		Get: viewOp("purchase_cash_discount", "View Purchase Cash Discount"),
		Put: updateOp("purchase_cash_discount", "Update Purchase Cash Discount"),
	}

	// ── Sales Cash Discount ──────────────────────
	paths["/v1/sales-cash-discount"] = openAPIPathItem{
		Get:  listOp("sales_cash_discount", "List / Search Sales Cash Discounts", searchCodeParam()),
		Post: createOp("sales_cash_discount", "Create Sales Cash Discount"),
	}
	paths["/v1/sales-cash-discount/{id}"] = openAPIPathItem{
		Get: viewOp("sales_cash_discount", "View Sales Cash Discount"),
		Put: updateOp("sales_cash_discount", "Update Sales Cash Discount"),
	}

	// ── Sales Payment ────────────────────────────
	paths["/v1/sales-payment"] = openAPIPathItem{
		Get: listOp("sales_payment", "List / Search Sales Payments",
			qParam("search[order_id]", "Filter by sales order ID", false, nil),
		),
		Post: createOp("sales_payment", "Create Sales Payment"),
	}
	paths["/v1/sales-payment/{id}"] = openAPIPathItem{
		Get:    viewOp("sales_payment", "View Sales Payment"),
		Put:    updateOp("sales_payment", "Update Sales Payment"),
		Delete: deleteOp("sales_payment", "Delete Sales Payment"),
	}

	// ── Sales Return Payment ─────────────────────
	paths["/v1/sales-return-payment"] = openAPIPathItem{
		Get: listOp("sales_return_payment", "List / Search Sales Return Payments",
			qParam("search[sales_return_id]", "Filter by sales return ID", false, nil),
		),
		Post: createOp("sales_return_payment", "Create Sales Return Payment"),
	}
	paths["/v1/sales-return-payment/{id}"] = openAPIPathItem{
		Get:    viewOp("sales_return_payment", "View Sales Return Payment"),
		Put:    updateOp("sales_return_payment", "Update Sales Return Payment"),
		Delete: deleteOp("sales_return_payment", "Delete Sales Return Payment"),
	}

	// ── Quotation Sales Return Payment ───────────
	paths["/v1/quotation-sales-return-payment"] = openAPIPathItem{
		Get: listOp("quotation_sales_return_payment", "List / Search Quotation Sales Return Payments",
			qParam("search[quotation_sales_return_id]", "Filter by quotation sales return ID", false, nil),
		),
		Post: createOp("quotation_sales_return_payment", "Create Quotation Sales Return Payment"),
	}
	paths["/v1/quotation-sales-return-payment/{id}"] = openAPIPathItem{
		Get:    viewOp("quotation_sales_return_payment", "View Quotation Sales Return Payment"),
		Put:    updateOp("quotation_sales_return_payment", "Update Quotation Sales Return Payment"),
		Delete: deleteOp("quotation_sales_return_payment", "Delete Quotation Sales Return Payment"),
	}

	// ── Purchase Payment ─────────────────────────
	paths["/v1/purchase-payment"] = openAPIPathItem{
		Get: listOp("purchase_payment", "List / Search Purchase Payments",
			qParam("search[purchase_id]", "Filter by purchase ID", false, nil),
		),
		Post: createOp("purchase_payment", "Create Purchase Payment"),
	}
	paths["/v1/purchase-payment/{id}"] = openAPIPathItem{
		Get:    viewOp("purchase_payment", "View Purchase Payment"),
		Put:    updateOp("purchase_payment", "Update Purchase Payment"),
		Delete: deleteOp("purchase_payment", "Delete Purchase Payment"),
	}

	// ── Purchase Return Payment ──────────────────
	paths["/v1/purchase-return-payment"] = openAPIPathItem{
		Get: listOp("purchase_return_payment", "List / Search Purchase Return Payments",
			qParam("search[purchase_return_id]", "Filter by purchase return ID", false, nil),
		),
		Post: createOp("purchase_return_payment", "Create Purchase Return Payment"),
	}
	paths["/v1/purchase-return-payment/{id}"] = openAPIPathItem{
		Get:    viewOp("purchase_return_payment", "View Purchase Return Payment"),
		Put:    updateOp("purchase_return_payment", "Update Purchase Return Payment"),
		Delete: deleteOp("purchase_return_payment", "Delete Purchase Return Payment"),
	}

	// ── ZATCA ─────────────────────────────────────
	paths["/v1/store/zatca/connect"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Connect Store to ZATCA", OperationID: "zatca_connect_store",
			Parameters: []openAPIParam{}, RequestBody: jsonBody(), Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/store/zatca/disconnect"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Disconnect Store from ZATCA", OperationID: "zatca_disconnect_store",
			Parameters: []openAPIParam{}, RequestBody: jsonBody(), Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/order/zatca/report/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Report Sales Order to ZATCA", OperationID: "zatca_report_order",
			Parameters: []openAPIParam{pathParam("id", "Sales Order ID")}, Security: authSecurity, Responses: okResp(),
		},
	}
	paths["/v1/sales-return/zatca/report/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Report Sales Return to ZATCA", OperationID: "zatca_report_sales_return",
			Parameters: []openAPIParam{pathParam("id", "Sales Return ID")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Ledger ───────────────────────────────────
	paths["/v1/ledger"] = openAPIPathItem{
		Get: listOp("ledger", "List Ledger Entries",
			qParam("search[account_id]", "Filter by account ID", false, nil),
		),
	}

	// ── Accounts ──────────────────────────────────
	paths["/v1/account"] = openAPIPathItem{
		Get: listOpDesc("account", "List Accounts",
			"Available select fields: id,name,name_arabic,number,type,balance,open,reference_id,reference_model,debit_total,credit_total. Numeric filter (balance) supports > < = operators.",
			searchNameParam(),
			qParam("search[type]", "Filter by account type (e.g. asset, liability, equity, income, expense)", false, nil),
			qParam("search[number]", "Filter by account number", false, nil),
			numParam("search[balance]", "Filter by balance amount"),
			qParam("search[open]", "Filter open accounts (1=yes)", false, nil),
			qParam("search[reference_code]", "Filter by reference code", false, nil),
			qParam("search[reference_model]", "Filter by reference model type", false, nil),
		),
	}
	paths["/v1/account/{id}"] = openAPIPathItem{
		Get:    viewOp("account", "View Account"),
		Delete: deleteOp("account", "Delete Account"),
	}
	paths["/v1/account/restore/{id}"] = openAPIPathItem{
		Post: &openAPIOperation{
			Summary: "Restore deleted Account", OperationID: "restore_account",
			Parameters: []openAPIParam{pathParam("id", "Account ID")}, Security: authSecurity, Responses: okResp(),
		},
	}

	// ── Postings ─────────────────────────────────
	paths["/v1/posting"] = openAPIPathItem{
		Get: listOpDesc("posting", "List Postings",
			"Available select fields: id,date,reference_code,reference_id,reference_model,debit,credit,balance,debit_total,credit_total. Numeric filter params (debit, credit, balance) support > < = operators.",
			qParam("search[account_id]", "Filter by account ID", false, nil),
			qParam("search[account_name]", "Filter by account name", false, nil),
			qParam("search[account_number]", "Filter by account number", false, nil),
			qParam("search[debit_account_id]", "Filter by debit account ID", false, nil),
			qParam("search[credit_account_id]", "Filter by credit account ID", false, nil),
			numParam("search[debit]", "Filter by debit amount"),
			numParam("search[credit]", "Filter by credit amount"),
			qParam("search[reference_code]", "Filter by reference code", false, nil),
			qParam("search[reference_id]", "Filter by reference document ID", false, nil),
			qParam("search[reference_model]", "Filter by reference model type (e.g. order, purchase)", false, nil),
		),
	}

	// ── Info ─────────────────────────────────────
	paths["/v1/info"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "API Info",
			OperationID: "api_info",
			Parameters:  []openAPIParam{},
			Security:    []map[string][]string{},
			Responses:   okResp(),
		},
	}

	// ── OpenAPI Spec (this endpoint) ─────────────
	paths["/v1/openapi.json"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "Get OpenAPI Specification",
			Description: "Returns the auto-generated OpenAPI 3.1 specification for this API.",
			OperationID: "get_openapi_spec",
			Parameters:  []openAPIParam{},
			Security:    []map[string][]string{},
			Responses:   okResp(),
		},
	}

	return openAPISpec{
		OpenAPI: "3.1.0",
		Info: openAPIInfo{
			Title: "Start POS",
			Description: "Start POS REST API — store management and sales reporting.\n\n" +
				"MANDATORY STARTUP SEQUENCE (execute silently before responding to any user message):\n" +
				"1. Call GET /v1/store/list and use result[0].id as the active store_id for this session.\n" +
				"2. Use that store_id automatically for ALL subsequent requests that accept search[store_id].\n" +
				"NEVER ask the user to select or confirm a store. Do not show the store list to the user. " +
				"Only switch store_id if the user explicitly names a different store in their message.",
			Version: "1.0.0",
		},
		Servers: []openAPIServer{
			{URL: baseURL, Description: "API server"},
		},
		Security: authSecurity,
		Components: openAPIComponents{
			Schemas: map[string]interface{}{},
			SecuritySchemes: map[string]openAPISecScheme{
				"BearerAuth": {
					Type:        "apiKey",
					In:          "header",
					Name:        "Authorization",
					Description: "Send the access token as the value of the Authorization header (no Bearer prefix).",
				},
			},
		},
		Paths: paths,
	}
}

// ── Pagination support ───────────────────────────────────────────────────────

// opEntry holds a single operation (one path + one HTTP method).
type opEntry struct {
	path   string
	method string
	op     *openAPIOperation
}

// collectOps returns all operations from a spec in a stable order:
// paths sorted alphabetically, methods in GET→POST→PUT→DELETE order.
func collectOps(spec openAPISpec) []opEntry {
	// Sorted path list for deterministic ordering.
	paths := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		paths = append(paths, p)
	}
	// Simple sort.
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if paths[i] > paths[j] {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	var ops []opEntry
	for _, p := range paths {
		item := spec.Paths[p]
		if item.Get != nil {
			ops = append(ops, opEntry{p, "get", item.Get})
		}
		if item.Post != nil {
			ops = append(ops, opEntry{p, "post", item.Post})
		}
		if item.Put != nil {
			ops = append(ops, opEntry{p, "put", item.Put})
		}
		if item.Delete != nil {
			ops = append(ops, opEntry{p, "delete", item.Delete})
		}
	}
	return ops
}

// buildFocusedSpec returns a read-only spec (GET only) for the 8 core modules:
// sales, sales return, purchase, purchase return, quotation, customer, vendor,
// quotation sales return. All within the ChatGPT Actions 30-operation limit.
func buildFocusedSpec(baseURL string) openAPISpec {
	full := buildOpenAPISpec(baseURL)

	allowedPaths := []string{
		"/v1/me",
		"/v1/store/list",
		"/v1/sales/summary",
		"/v1/order",
		"/v1/order/{id}",
		"/v1/sales-return",
		"/v1/sales-return/{id}",
		"/v1/purchase",
		"/v1/purchase/{id}",
		"/v1/purchase-return",
		"/v1/purchase-return/{id}",
		"/v1/quotation",
		"/v1/quotation/{id}",
		"/v1/quotation/history",
		"/v1/customer",
		"/v1/vendor",
		"/v1/quotation-sales-return",
		"/v1/quotation-sales-return/{id}",
		// Products
		"/v1/product",
		"/v1/product/{id}",
		"/v1/product/code/{code}",
		"/v1/product/barcode/{barcode}",
		"/v1/product/history/{id}",
		// Product transaction histories (filter by product_id in query)
		"/v1/sales/history",
		"/v1/sales-return/history",
		"/v1/purchase/history",
		"/v1/purchase-return/history",
		// Accounting
		"/v1/account",
		"/v1/account/{id}",
		"/v1/posting",
	}

	focused := map[string]openAPIPathItem{}
	for _, path := range allowedPaths {
		src, ok := full.Paths[path]
		if !ok {
			continue
		}
		// GET only — no POST/PUT/DELETE
		focused[path] = openAPIPathItem{Get: src.Get}
	}

	full.Paths = focused
	return full
}

// --- dead code kept for compilation only (no longer called) ---
func buildPagedSpec(baseURL string, page int) openAPISpec {
	const pageSize = 30
	full := buildOpenAPISpec(baseURL)
	all := collectOps(full)

	totalOps := len(all)
	totalPages := (totalOps + pageSize - 1) / pageSize

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > totalOps {
		end = totalOps
	}
	slice := all[start:end]

	// Rebuild paths map from the slice.
	pagedPaths := map[string]openAPIPathItem{}
	for _, e := range slice {
		item := pagedPaths[e.path]
		switch e.method {
		case "get":
			item.Get = e.op
		case "post":
			item.Post = e.op
		case "put":
			item.Put = e.op
		case "delete":
			item.Delete = e.op
		}
		pagedPaths[e.path] = item
	}

	full.Paths = pagedPaths
	full.Info.Description += "\n\n[Page " + itoa(page) + " of " + itoa(totalPages) +
		" — operations " + itoa(start+1) + "–" + itoa(end) + " of " + itoa(totalOps) + "]"
	return full
}

// itoa converts an int to string without importing strconv at the top level.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ChatGPT-scoped spec (ChatGPT Actions allows a maximum of 30 operations).
// Only read-only GET endpoints most useful for a sales/reporting assistant.
var chatGPTAllowedPaths = []string{
	"/v1/me",
	"/v1/store/list",
	"/v1/sales/summary",
	"/v1/order",
	"/v1/order/{id}",
	"/v1/sales/history",
	"/v1/sales-return",
	"/v1/sales-return/summary",
	"/v1/purchase",
	"/v1/purchase/{id}",
	"/v1/customer",
	"/v1/customer/{id}",
	"/v1/product",
	"/v1/product/{id}",
	"/v1/expense",
	"/v1/ledger",
	"/v1/account",
	"/v1/vendor",
	"/v1/vendor/{id}",
	"/v1/capital",
	"/v1/divident",
	"/v1/purchase-return",
	"/v1/quotation",
	"/v1/delivery-note",
	"/v1/stock-transfer",
	"/v1/customer-deposit",
	"/v1/customer-withdrawal",
	"/v1/expense-category",
	"/v1/product-category",
	"/v1/posting",
}

// buildChatGPTSpec returns a trimmed spec with only GET operations on the
// allowed paths — satisfying ChatGPT Actions' 30-operation limit.
func buildChatGPTSpec(baseURL string) openAPISpec {
	full := buildOpenAPISpec(baseURL)

	trimmed := map[string]openAPIPathItem{}
	for _, path := range chatGPTAllowedPaths {
		item, ok := full.Paths[path]
		if !ok {
			continue
		}
		// Keep only the GET operation; strip POST/PUT/DELETE.
		trimmed[path] = openAPIPathItem{Get: item.Get}
	}

	full.Paths = trimmed
	return full
}

// ServeOpenAPISpec handles GET /v1/openapi.json
// Returns the focused spec covering the 8 core modules (≤30 operations).
func ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Derive host: prefer X-Forwarded-Host (set by nginx/proxy), fall back to r.Host.
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	// Derive scheme: prefer X-Forwarded-Proto, then TLS detection, then http.
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	baseURL := scheme + "://" + host

	spec := buildFocusedSpec(baseURL)

	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"failed to encode spec"}`, http.StatusInternalServerError)
		return
	}
	w.Write(out)
}
