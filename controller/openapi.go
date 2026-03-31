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
	Schema  map[string]interface{} `json:"schema,omitempty"`
	Example interface{}            `json:"example,omitempty"`
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

// okRespWithExample returns a 200/401/500 response map with a sample response body.
func okRespWithExample(example interface{}) map[string]openAPIResponse {
	return map[string]openAPIResponse{
		"200": {Description: "Success", Content: map[string]openAPIMediaType{
			"application/json": {Example: example},
		}},
		"401": {Description: "Unauthorized — invalid or missing access token"},
		"500": {Description: "Server error"},
	}
}

// withExample wraps any operation and attaches a sample 200 response body example.
func withExample(op *openAPIOperation, example interface{}) *openAPIOperation {
	op.Responses = okRespWithExample(example)
	return op
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
			Responses: okRespWithExample(map[string]interface{}{
				"status": true,
				"total_count": 0,
				"result": map[string]interface{}{
					"id": "61fe8ce6a31c68a2a0ce3a28",
					"name": "Sirin k",
					"email": "sirinibin2006@gmail.com",
					"mob": "9633977699",
					"admin": true,
					"role": "Admin",
					"online": true,
					"store_ids": []interface{}{
						"67fea10a97457210a52a5eab",
					},
					"store_names": []interface{}{
						"MAABDI Trading Est - Jouhara",
					},
					"connected_computers": 2,
					"created_at": "2022-02-05T14:42:46.087Z",
					"updated_at": "2025-04-15T18:16:09.768Z",
				},
			}),
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
			op.Responses = okRespWithExample(map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 2,
					"select": map[string]interface{}{
						"id": 1,
						"name": 1,
						"code": 1,
						"branch_name": 1,
						"vat_no": 1,
						"country_code": 1,
						"country_name": 1,
					},
					"search_by": map[string]interface{}{
						"deleted": map[string]interface{}{
							"$ne": true,
						},
					},
				},
				"total_count": 16,
				"result": []interface{}{
					map[string]interface{}{
						"id": "61fe9179a31c68a2a0ce3a2b",
						"name": "GULF UNION OZONE",
						"code": "GUOJ",
						"branch_name": "UMLUJ",
						"vat_no": "399999999900003",
					},
					map[string]interface{}{
						"id": "65d079eee327423a24deb105",
						"name": "Store2",
						"code": "Str2",
						"branch_name": "",
						"vat_no": "123",
					},
				},
			})
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
		Get: withExample(listOpDesc("customer", "List / Search Customers",
			"Filter by name, code, phone, vat_no, deleted. Supports numeric comparison (> < =) on credit_balance, credit_limit, sales_count, sales_amount. Response includes a meta object with aggregated totals across all matching records.",
			searchNameParam(), searchCodeParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[phone]", "Filter by phone", false, nil),
			qParam("search[vat_no]", "Filter by VAT number", false, nil),
			qParam("search[query]", "General text search across name/code/email/phone", false, nil),
			numParam("search[credit_balance]", "Filter by credit balance amount"),
			numParam("search[credit_limit]", "Filter by credit limit"),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
		),
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 5,
			"result": []interface{}{
				map[string]interface{}{
					"id": "695e64bc4e491e90c1a25e7e",
					"code": "C-0761",
					"name": "C1",
					"name_in_arabic": "",
					"vat_no": "",
					"phone": "",
					"phone2": "",
					"credit_limit": 0,
					"credit_balance": 0,
					"account": map[string]interface{}{
						"id": "695e64d04e491e90c1a25e80",
						"store_id": "680f6c53e32076f2003a8934",
						"reference_id": "695e64bc4e491e90c1a25e7e",
						"reference_model": "customer",
						"type": "asset",
						"number": "1009",
						"name": "C1",
						"balance": 0,
						"debit_total": 45,
						"credit_total": 45,
						"deleted": false,
					},
					"deleted": false,
					"created_at": "2026-01-07T13:50:52.39Z",
					"created_by": "61fe8ce6a31c68a2a0ce3a28",
					"created_by_name": "Sirin k",
					"search_label": "#C-0761 C1",
					"store_id": "680f6c53e32076f2003a8934",
					"stores": map[string]interface{}{
						"680f6c53e32076f2003a8934": map[string]interface{}{
							"store_id": "680f6c53e32076f2003a8934",
							"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
							"sales_count": 1,
							"sales_amount": 45,
							"sales_paid_amount": 45,
							"sales_balance_amount": 0,
							"sales_profit": 0,
							"sales_loss": 0,
							"quotation_count": 1,
							"quotation_amount": 23,
							"quotation_profit": 13,
						},
					},
				},
				map[string]interface{}{
					"id": "697e7728bb03695e2229f587",
					"code": "C-0765",
					"name": "UNKNOWN",
					"name_in_arabic": "مجهول",
					"vat_no": "",
					"phone": "",
					"phone2": "",
					"credit_limit": 0,
					"credit_balance": 73.6,
					"account": map[string]interface{}{
						"id": "697e77da1a9798ef0af72af1",
						"store_id": "680f6c53e32076f2003a8934",
						"reference_id": "697e7728bb03695e2229f587",
						"reference_model": "customer",
						"type": "asset",
						"number": "1011",
						"name": "UNKNOWN",
						"balance": 73.6,
						"debit_total": 1516.85,
						"credit_total": 1443.25,
						"open": true,
						"deleted": false,
					},
					"deleted": false,
					"created_at": "2026-01-31T21:42:00.934Z",
					"created_by": "61fe8ce6a31c68a2a0ce3a28",
					"created_by_name": "Sirin k",
					"search_label": "#C-0765 UNKNOWN | مجهول",
					"store_id": "680f6c53e32076f2003a8934",
					"stores": map[string]interface{}{
						"680f6c53e32076f2003a8934": map[string]interface{}{
							"store_id": "680f6c53e32076f2003a8934",
							"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
							"sales_count": 6,
							"sales_amount": 1493.85,
							"sales_paid_amount": 1420.25,
							"sales_balance_amount": 73.6,
							"sales_profit": 1179.52,
							"sales_loss": 0,
							"sales_return_count": 1,
							"sales_return_amount": 23,
							"quotation_count": 2,
							"quotation_amount": 231,
							"quotation_profit": 190.37,
						},
					},
				},
			},
			"meta": map[string]interface{}{
				"sales": 0,
				"sales_count": 0,
				"sales_paid": 0,
				"sales_paid_count": 0,
				"sales_unpaid_count": 0,
				"sales_paid_partially_count": 0,
				"sales_credit_balance": 0,
				"credit_balance": 0,
				"sales_return": 0,
				"sales_return_count": 0,
				"quotation_sales": 0,
				"quotation_sales_count": 0,
				"quotation_sales_return": 0,
				"quotation_sales_return_count": 0,
				"delivery_note_count": 0,
			},
		}),
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
	paths["/v1/customer/summary"] = openAPIPathItem{
		Get: listOp("customer_summary", "Get Customer Summary",
			searchNameParam(), searchCodeParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[phone]", "Filter by phone", false, nil),
			qParam("search[vat_no]", "Filter by VAT number", false, nil),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
		),
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
		Get: withExample(listOpDesc("product", "List / Search Products",
			"Filter by name, code, barcode, category_id, deleted. Supports numeric comparison (> < =) on stock, retail_unit_price, wholesale_unit_price, profit. Response includes a meta object with aggregated totals across all matching records.",
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
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"select": map[string]interface{}{
						"id": 1,
						"name": 1,
						"item_code": 1,
						"unit": 1,
						"product_stores": 1,
						"deleted": 1,
					},
					"search_by": map[string]interface{}{
						"deleted": map[string]interface{}{
							"$ne": true,
						},
					},
				},
				"total_count": 2503,
				"result": []interface{}{
					map[string]interface{}{
						"id": "684064ac42999a68cb049541",
						"name": "PIPE 27NO. (13MM) 5/8 USA-27",
						"name_in_arabic": "لي بارد",
						"item_code": "00121/DA8504",
						"ean_12": "100000000002",
						"part_number": "00121/DA8504/SUC 6241",
						"unit": "Meter(s)",
						"product_stores": map[string]interface{}{
							"680f6c53e32076f2003a8934": map[string]interface{}{
								"purchase_unit_price": 10,
								"retail_unit_price": 200,
								"retail_unit_price_with_vat": 230,
								"stock": -1,
								"sales_count": 1,
								"sales": 230,
								"sales_profit": 190,
							},
						},
						"images": []interface{}{},
						"deleted": false,
						"is_set": true,
						"category_name": []interface{}{},
					},
				},
				"meta": map[string]interface{}{
					"purchase": 0,
					"purchase_return": 0,
					"purchase_stock_value": 0,
					"retail_stock_value": 0,
					"sales": 0,
					"sales_profit": 0,
					"sales_return": 0,
					"sales_return_profit": 0,
					"stock": 0,
					"wholesale_stock_value": 0,
				},
			}),
		Post: createOp("product", "Create Product"),
	}
	paths["/v1/product/summary"] = openAPIPathItem{
		Get: withExample(listOp("product_summary", "Get Product Summary",
			searchNameParam(), searchCodeParam(),
			qParam("search[category_id]", "Filter by category ID", false, nil),
			qParam("search[deleted]", "Pass 1 to include deleted", false, nil),
		), map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"deleted": map[string]interface{}{
							"$ne": true,
						},
					},
				},
				"total_count": 2503,
				"result": map[string]interface{}{
					"id": nil,
					"stock": -13,
					"retail_stock_value": -192,
					"wholesale_stock_value": -202,
					"purchase_stock_value": -11,
					"sales": 1791.11,
					"sales_profit": 1179.53,
					"sales_return": 23,
					"sales_return_profit": 13,
					"purchase": 316.25,
					"purchase_return": 0,
				},
			}),
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
			Responses: okRespWithExample(map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 1,
				"result": []interface{}{
					map[string]interface{}{
						"id": "69cc2916924653819c67a82d",
						"date": "2026-03-31T20:05:35.072Z",
						"store_id": "680f6c53e32076f2003a8934",
						"product_id": "684064ac42999a68cb049541",
						"reference_type": "sales",
						"reference_id": "69cc2916924653819c67a81f",
						"reference_code": "S-INV-20260331-005",
						"stock": -1,
						"quantity": 1,
						"purchase_unit_price": 10,
						"unit_price": 200,
						"unit": "Meter(s)",
						"price": 200,
						"net_price": 230,
						"profit": 190,
						"loss": 0,
						"vat_percent": 15,
						"vat_price": 30,
						"unit_price_with_vat": 230,
						"created_at": "2026-03-31T20:05:42.114Z",
					},
				},
				"meta": map[string]interface{}{
					"total_sales": 230,
					"total_sales_profit": 190,
					"total_sales_loss": 0,
					"total_sales_vat": 30,
					"total_purchase": 0,
					"total_purchase_profit": 0,
					"total_purchase_loss": 0,
					"total_purchase_vat": 0,
					"total_purchase_return": 0,
					"total_purchase_return_profit": 0,
					"total_purchase_return_loss": 0,
					"total_purchase_return_vat": 0,
					"total_sales_return": 0,
					"total_sales_return_profit": 0,
					"total_sales_return_loss": 0,
					"total_sales_return_vat": 0,
					"total_quotation": 0,
					"total_quotation_profit": 0,
					"total_quotation_loss": 0,
					"total_quotation_vat": 0,
					"total_delivery_note_quantity": 0,
				},
			}),
		},
	}
	paths["/v1/product/history/summary/{id}"] = openAPIPathItem{
		Get: &openAPIOperation{
			Summary:     "Get Product History Summary",
			OperationID: "get_product_history_summary",
			Parameters: []openAPIParam{
				pathParam("id", "Product ID"),
				storeParam(),
			},
			Security:  authSecurity,
			Responses: okRespWithExample(map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 1,
				"result": map[string]interface{}{
					"total_sales": 230,
					"total_sales_profit": 190,
					"total_sales_loss": 0,
					"total_sales_vat": 30,
					"total_sale_return": 0,
					"total_sales_return_profit": 0,
					"total_sales_return_loss": 0,
					"total_sales_return_vat": 0,
					"total_purchase": 0,
					"total_purchase_profit": 0,
					"total_purchase_loss": 0,
					"total_purchase_vat": 0,
					"total_purchase_return": 0,
					"total_purchase_return_profit": 0,
					"total_purchase_return_loss": 0,
					"total_purchase_return_vat": 0,
					"total_quotation": 0,
					"total_quotation_profit": 0,
					"total_quotation_loss": 0,
					"total_quotation_vat": 0,
					"total_quotation_sales": 0,
					"total_quotation_sales_profit": 0,
					"total_quotation_sales_loss": 0,
					"total_quotation_sales_vat": 0,
					"total_quotation_sales_return": 0,
					"total_quotation_sales_return_profit": 0,
					"total_quotation_sales_return_loss": 0,
					"total_quotation_sales_return_vat": 0,
					"total_delivery_note_quantity": 0,
				},
			}),
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
		Get: withExample(listOpDesc("expense", "List / Search Expenses", "The response includes a meta object with aggregated totals (e.g. total amount, paid, unpaid, VAT, profit/loss) across all matching records — not just the current page.",
			searchCodeParam(),
			qParam("search[category_id]", "Filter by expense category ID", false, nil),
			qParam("search[amount]", "Filter by amount", false, nil),
		),
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 2,
			"result": []interface{}{
				map[string]interface{}{
					"id": "698b3f784782b0b706464edd",
					"code": "EXP-20260210-001",
					"amount": 23,
					"description": "test",
					"date": "2026-02-10T14:23:21.664Z",
					"payment_method": "debit_card",
					"store_id": "680f6c53e32076f2003a8934",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"store_code": "MBDI",
					"category_id": []interface{}{
						"698b3f704782b0b706464edc",
					},
					"category_name": []interface{}{
						"BAKKALA",
					},
					"created_at": "2026-02-10T14:23:52.343Z",
					"updated_at": "2026-02-10T14:23:52.343Z",
					"created_by": "61fe8ce6a31c68a2a0ce3a28",
					"updated_by": "61fe8ce6a31c68a2a0ce3a28",
					"created_by_name": "Sirin k",
					"updated_by_name": "Sirin k",
					"deleted": false,
					"deleted_by": nil,
					"deleted_by_user": nil,
					"deleted_at": nil,
					"vendor_id": "698b3f634782b0b706464edb",
					"vendor_invoice_no": "",
					"taxable": false,
					"vat_percent": 15,
					"vat_price": 3,
					"vendor_name": "V1",
					"vendor_name_arabic": "",
				},
				map[string]interface{}{
					"id": "698b43c54782b0b706464ee7",
					"code": "EXP-20260210-002",
					"amount": 34,
					"description": "test",
					"date": "2026-02-10T14:41:57.253Z",
					"payment_method": "cash",
					"store_id": "680f6c53e32076f2003a8934",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"store_code": "MBDI",
					"category_id": []interface{}{
						"698b43be4782b0b706464ee6",
					},
					"category_name": []interface{}{
						"purchase",
					},
					"created_at": "2026-02-10T14:42:13.844Z",
					"updated_at": "2026-02-10T14:42:13.844Z",
					"created_by": "61fe8ce6a31c68a2a0ce3a28",
					"updated_by": "61fe8ce6a31c68a2a0ce3a28",
					"created_by_name": "Sirin k",
					"updated_by_name": "Sirin k",
					"deleted": false,
					"deleted_by": nil,
					"deleted_by_user": nil,
					"deleted_at": nil,
					"vendor_id": nil,
					"vendor_invoice_no": "",
					"taxable": false,
					"vat_percent": nil,
					"vat_price": 0,
					"vendor_name": "",
					"vendor_name_arabic": "",
				},
			},
			"meta": map[string]interface{}{
				"bank": 0,
				"cash": 0,
				"purchase_fund": 0,
				"total": 0,
				"vat": 0,
			},
		}),
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
	paths["/v1/expense/summary"] = openAPIPathItem{
		Get: listOp("expense_summary", "Get Expense Summary",
			searchCodeParam(),
			qParam("search[category_id]", "Filter by expense category ID", false, nil),
			qParam("search[payment_method]", "Filter by payment method", false, nil),
			qParam("search[created_by]", "Filter by creator user ID", false, nil),
		),
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
		Get: withExample(listOpDesc("quotation", "List / Search Quotations",
			"Filter by date, customer_id, type (quotation or invoice), status. Supports numeric comparison (> < =) on net_total, profit. Response includes a meta object with aggregated totals across all matching records.",
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
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 5,
			"result": []interface{}{
				map[string]interface{}{
					"id": "697fa3e7882854c9dd926477",
					"code": "QTN-20260201-01",
					"date": "2026-02-01T19:04:58.194Z",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "695e64bc4e491e90c1a25e7e",
					"customer_name": "C1",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6966a9bbb673ce08466f7c29",
							"name": "b123",
							"part_number": "7374163523",
							"quantity": 1,
							"unit_price": 10,
							"unit_price_with_vat": 11.5,
							"profit": 8,
							"loss": 0,
						},
						map[string]interface{}{
							"product_id": "6966a9aeb673ce08466f7c28",
							"name": "a123",
							"part_number": "3863918361",
							"quantity": 1,
							"unit_price": 10,
							"unit_price_with_vat": 11.5,
							"profit": 5,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "delivered",
					"total_quantity": 2,
					"vat_price": 3,
					"total": 20,
					"total_with_vat": 23,
					"net_total": 23,
					"cash_discount": 0,
					"payment_status": "",
					"payment_methods": nil,
					"total_payment_received": 0,
					"balance_amount": 0,
					"profit": 13,
					"net_profit": 13,
					"loss": 0,
					"net_loss": 0,
					"return_count": 0,
					"return_amount": 0,
					"created_at": "2026-02-01T19:05:11.202Z",
					"type": "quotation",
					"validity_days": 7,
					"delivery_days": 7,
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
				},
				map[string]interface{}{
					"id": "697fbdcd882854c9dd92648d",
					"code": "QTN-20260201-02",
					"date": "2026-02-01T20:55:26.291Z",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "695e64f84e491e90c1a25e8d",
					"customer_name": "C2",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6966a9bbb673ce08466f7c29",
							"name": "b123",
							"part_number": "7374163523",
							"quantity": 1,
							"unit_price": 20,
							"unit_price_with_vat": 23,
							"profit": 18,
							"loss": 0,
						},
						map[string]interface{}{
							"product_id": "6966a9aeb673ce08466f7c28",
							"name": "a123",
							"part_number": "3863918361",
							"quantity": 1,
							"unit_price": 10,
							"unit_price_with_vat": 11.5,
							"profit": 5,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "delivered",
					"total_quantity": 2,
					"vat_price": 4.5,
					"total": 30,
					"total_with_vat": 34.5,
					"net_total": 34.5,
					"cash_discount": 0,
					"payment_status": "paid",
					"payment_methods": []interface{}{
						"bank_card",
					},
					"total_payment_received": 34.5,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "697fbdcd882854c9dd926490",
							"quotation_id": "697fbdcd882854c9dd92648d",
							"quotation_code": "QTN-20260201-02",
							"amount": 34.5,
							"method": "bank_card",
						},
					},
					"payments_count": 1,
					"profit": 23,
					"net_profit": 23,
					"loss": 0,
					"net_loss": 0,
					"return_count": 1,
					"return_amount": 34.5,
					"created_at": "2026-02-01T20:55:41.507Z",
					"type": "invoice",
					"validity_days": 7,
					"delivery_days": 7,
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
				},
			},
			"meta": map[string]interface{}{
				"total_quotation": 0,
				"profit": 0,
				"loss": 0,
				"invoice_total_sales": 0,
				"invoice_paid_sales": 0,
				"invoice_unpaid_sales": 0,
				"invoice_vat_price": 0,
				"invoice_discount": 0,
				"invoice_net_profit": 0,
				"invoice_net_loss": 0,
			},
		}),
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
	paths["/v1/quotation/summary"] = openAPIPathItem{
		Get: withExample(listOp("quotation_summary", "Get Quotation Summary",
			searchCodeParam(),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[type]", "Filter by quotation type", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[status]", "Filter by status", false, nil),
			numParam("search[net_total]", "Filter by net total"),
			numParam("search[discount]", "Filter by discount"),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"date": map[string]interface{}{
							"$gte": "2026-02-28T21:00:00Z",
							"$lte": "2026-03-11T20:59:59Z",
						},
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"net_total": 0,
					"net_profit": 0,
					"loss": 0,
				},
			}),
	}
	paths["/v1/quotation/sales/summary"] = openAPIPathItem{
		Get: withExample(listOp("quotation_sales_summary", "Get Quotation Sales Summary",
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[type]", "Filter by quotation type", false, nil),
			qParam("search[payment_status]", "Filter by payment status", false, nil),
			qParam("search[status]", "Filter by status", false, nil),
			searchCodeParam(),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 3,
				"result": map[string]interface{}{
					"invoice_net_total": 34.5,
					"invoice_net_profit": 23,
					"invoice_net_loss": 0,
					"invoice_vat_price": 0,
					"invoice_discount": 0,
					"invoice_hipping_handling_fees": 0,
					"invoice_paid_sales": 34.5,
					"invoice_unpaid_sales": 0,
					"invoice_cash_sales": 0,
					"invoice_bank_account_sales": 34.5,
					"invoice_cash_discount": 0,
					"invoice_sales_return_sales": 0,
				},
			}),
	}
	paths["/v1/quotation/history"] = openAPIPathItem{
		Get: withExample(listOp("quotation_history", "List Quotation History",
			qParam("search[product_id]", "REQUIRED — Product ID to filter history by", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[quotation_id]", "Filter by quotation ID", false, nil),
			qParam("search[quotation_code]", "Filter by quotation code", false, nil),
			qParam("search[type]", "Filter by quotation type", false, nil),
			qParam("search[payment_status]", "Filter by payment status", false, nil),
			numParam("search[quantity]", "Filter by quantity"),
			numParam("search[price]", "Filter by price"),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 2,
				"result": []interface{}{
					map[string]interface{}{
						"id": "69cc52676a4f405cf5d4ed25",
						"date": "2026-03-31T23:01:52.923Z",
						"store_id": "680f6c53e32076f2003a8934",
						"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
						"product_id": "684064ac42999a68cb049541",
						"customer_id": "697e7728bb03695e2229f587",
						"customer_name": "UNKNOWN",
						"customer_name_arabic": "مجهول",
						"quotation_id": "69cc52676a4f405cf5d4ed24",
						"quotation_code": "QTN-20260401-02",
						"quantity": 1,
						"unit_price": 200,
						"unit_discount": 0,
						"discount": 0,
						"discount_percent": 0,
						"price": 200,
						"net_price": 230,
						"profit": 190,
						"loss": 0,
						"vat_percent": 15,
						"vat_price": 30,
						"unit": "Meter(s)",
						"unit_price_with_vat": 230,
						"created_at": "2026-03-31T23:01:59.794Z",
						"type": "invoice",
						"payment_status": "",
						"warehouse_code": "main_store",
					},
					map[string]interface{}{
						"id": "69cc525e6a4f405cf5d4ed21",
						"date": "2026-03-31T23:01:44.656Z",
						"store_id": "680f6c53e32076f2003a8934",
						"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
						"product_id": "684064ac42999a68cb049541",
						"customer_id": "697e7728bb03695e2229f587",
						"customer_name": "UNKNOWN",
						"customer_name_arabic": "مجهول",
						"quotation_id": "69cc525e6a4f405cf5d4ed20",
						"quotation_code": "QTN-20260401-01",
						"quantity": 1,
						"unit_price": 200,
						"unit_discount": 0,
						"discount": 0,
						"discount_percent": 0,
						"price": 200,
						"net_price": 230,
						"profit": 190,
						"loss": 0,
						"vat_percent": 15,
						"vat_price": 30,
						"unit": "Meter(s)",
						"unit_price_with_vat": 230,
						"created_at": "2026-03-31T23:01:50.424Z",
						"type": "quotation",
						"payment_status": "",
						"warehouse_code": "main_store",
					},
				},
				"meta": map[string]interface{}{
					"total_loss": 0,
					"total_profit": 380,
					"total_quantity": 2,
					"total_quotation": 460,
					"total_vat": 60,
				},
			}),
	}
	paths["/v1/quotation/history/summary"] = openAPIPathItem{
		Get: withExample(listOp("quotation_history_summary", "Get Quotation History Summary",
			qParam("search[product_id]", "REQUIRED — Product ID to get history summary for", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[type]", "Filter by quotation type", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"total_quotation": 0,
					"total_profit": 0,
					"total_loss": 0,
					"total_vat": 0,
					"total_quantity": 0,
				},
			}),
	}

	// ── Delivery Note ────────────────────────────────────────────
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
		Get: withExample(listOpDesc("order", "List / Search Sales Orders",
			"Filter by date, customer_id, payment_status, payment_methods, status. Supports numeric comparison (> < =) on net_total, balance_amount, profit. Response includes a meta object with aggregated totals across all matching records.",
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
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 4,
			"result": []interface{}{
				map[string]interface{}{
					"id": "69c2566b9a002ff1e95a080d",
					"date": "2026-03-11T09:21:00Z",
					"code": "S-INV-20260324-004",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "697e7728bb03695e2229f587",
					"customer": nil,
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "698b1f994782b0b706464ed8",
							"name": "COMP. AXOR 24V 11PK-2174",
							"part_number": "JP447220-8027",
							"quantity": 4,
							"unit_price": 0.86956522,
							"unit_price_with_vat": 1,
							"profit": 0,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"discount_percent": 0,
					"status": "delivered",
					"total_quantity": 4,
					"vat_price": 0.52,
					"total": 3.48,
					"total_with_vat": 4,
					"net_total": 4,
					"cash_discount": 0,
					"total_payment_received": 4,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "69c2566b9a002ff1e95a080e",
							"order_id": "69c2566b9a002ff1e95a080d",
							"order_code": "S-INV-20260324-004",
							"amount": 4,
							"method": "cash",
						},
					},
					"payments_count": 1,
					"payment_status": "paid",
					"payment_methods": []interface{}{
						"cash",
					},
					"profit": 0,
					"net_profit": 0,
					"loss": 0,
					"net_loss": 0,
					"return_count": 0,
					"return_amount": 0,
					"created_at": "2026-03-24T09:16:27.749Z",
					"customer_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
					"zatca": map[string]interface{}{
						"is_simplified": false,
						"compliance_passed": false,
						"reporting_passed": false,
					},
				},
				map[string]interface{}{
					"id": "69b133f3b859884b1a9d33db",
					"date": "2026-03-11T09:20:27.105Z",
					"code": "S-INV-20260311-003",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "697e7728bb03695e2229f587",
					"customer": nil,
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "698b1f994782b0b706464ed8",
							"name": "COMP. AXOR 24V 11PK-2174",
							"part_number": "JP447220-8027",
							"quantity": 3,
							"unit_price": 32,
							"unit_price_with_vat": 36.8,
							"profit": 90,
							"loss": 0,
						},
						map[string]interface{}{
							"product_id": "698b1f614782b0b706464ed7",
							"name": "COMP. AXOR VALEO",
							"part_number": "CHASH-VC8148",
							"quantity": 1,
							"unit_price": 23,
							"unit_price_with_vat": 26.45,
							"profit": 20,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"discount_percent": 0,
					"status": "delivered",
					"total_quantity": 4,
					"vat_price": 17.85,
					"total": 119,
					"total_with_vat": 136.85,
					"net_total": 136.85,
					"cash_discount": 0,
					"total_payment_received": 63.25,
					"balance_amount": 73.6,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "69b133f3b859884b1a9d33dc",
							"order_id": "69b133f3b859884b1a9d33db",
							"order_code": "S-INV-20260311-003",
							"amount": 53.25,
							"method": "credit_card",
						},
						map[string]interface{}{
							"id": "69b133f3b859884b1a9d33dd",
							"order_id": "69b133f3b859884b1a9d33db",
							"order_code": "S-INV-20260311-003",
							"amount": 10,
							"method": "cash",
						},
					},
					"payments_count": 2,
					"payment_status": "paid_partially",
					"payment_methods": []interface{}{
						"credit_card",
						"cash",
					},
					"profit": 110,
					"net_profit": 110,
					"loss": 0,
					"net_loss": 0,
					"return_count": 0,
					"return_amount": 0,
					"created_at": "2026-03-11T09:20:51.344Z",
					"customer_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
					"zatca": map[string]interface{}{
						"is_simplified": false,
						"compliance_passed": false,
						"reporting_passed": false,
					},
				},
			},
			"meta": map[string]interface{}{
				"cash_sales": 0,
				"bank_account_sales": 0,
				"total_sales": 0,
				"paid_sales": 0,
				"unpaid_sales": 0,
				"vat_price": 0,
				"discount": 0,
				"return_count": 0,
				"return_amount": 0,
				"net_profit": 0,
				"net_loss": 0,
			},
		}),
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
			Responses: okRespWithExample(map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"date": map[string]interface{}{
							"$gte": "2026-02-28T21:00:00Z",
							"$lte": "2026-03-11T20:59:59Z",
						},
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 4,
				"result": map[string]interface{}{
					"net_total": 1240.85,
					"net_profit": 976.52,
					"net_loss": 0,
					"vat_price": 161.85,
					"discount": 0,
					"shipping_handling_fees": 0,
					"paid_sales": 1167.25,
					"unpaid_sales": 73.6,
					"cash_sales": 14,
					"bank_account_sales": 1153.25,
					"cash_discount": 0,
					"return_count": 0,
					"return_amount": 0,
					"purchase_sales": 0,
					"sales_return_sales": 0,
					"commission": 0,
					"commission_paid_by_cash": 0,
					"commission_paid_by_bank": 0,
				},
			}),
		},
	}
	paths["/v1/sales/history"] = openAPIPathItem{
		Get: withExample(listOp("sales_history", "List Sales History",
			qParam("search[product_id]", "REQUIRED — Product ID to filter history by", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[order_id]", "Filter by sales order ID", false, nil),
			qParam("search[order_code]", "Filter by sales order code", false, nil),
			numParam("search[quantity]", "Filter by quantity"),
			numParam("search[price]", "Filter by price"),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			numParam("search[discount]", "Filter by discount"),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
			qParam("search[customer_name]", "Filter by customer name", false, nil),
			qParam("search[profit]", "Filter by profit", false, nil),
			qParam("search[loss]", "Filter by loss", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 1,
				"result": []interface{}{
					map[string]interface{}{
						"id": "69cc2916924653819c67a821",
						"date": "2026-03-31T20:05:35.072Z",
						"store_id": "680f6c53e32076f2003a8934",
						"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
						"product_id": "684064ac42999a68cb049541",
						"order_id": "69cc2916924653819c67a81f",
						"order_code": "S-INV-20260331-005",
						"quantity": 1,
						"purchase_unit_price": 10,
						"unit_price": 200,
						"unit": "Meter(s)",
						"discount": 0,
						"discount_percent": 0,
						"price": 200,
						"net_price": 230,
						"profit": 190,
						"loss": 0,
						"vat_percent": 15,
						"vat_price": 30,
						"unit_price_with_vat": 230,
						"created_at": "2026-03-31T20:05:42.114Z",
					},
				},
				"meta": map[string]interface{}{
					"total_loss": 0,
					"total_profit": 190,
					"total_quantity": 1,
					"total_sales": 230,
					"total_vat": 30,
				},
			}),
	}
	paths["/v1/sales/history/summary"] = openAPIPathItem{
		Get: withExample(listOp("sales_history_summary", "Get Sales History Summary",
			qParam("search[product_id]", "REQUIRED — Product ID to get sales history summary for", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[order_id]", "Filter by order ID", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 1,
				"result": map[string]interface{}{
					"total_sales": 230,
					"total_profit": 190,
					"total_loss": 0,
					"total_vat": 30,
					"total_quantity": 1,
				},
			}),
	}

	// ── Sales Return ─────────────────────────────────────────────
	paths["/v1/sales-return"] = openAPIPathItem{
		Get: withExample(listOpDesc("sales_return", "List / Search Sales Returns",
			"Filter by date, customer_id, order_id, payment_status. Supports numeric comparison (> < =) on net_total, balance_amount, profit. Response includes a meta object with aggregated totals across all matching records.",
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
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 1,
			"result": []interface{}{
				map[string]interface{}{
					"id": "697f9eac882854c9dd926465",
					"order_id": "697b270a0a1a0181914c7fdc",
					"order_code": "S-INV-20260129-004",
					"date": "2026-02-01T18:42:47.384Z",
					"code": "SR-INV-20260201-001",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "697e7728bb03695e2229f587",
					"customer": map[string]interface{}{
						"id": "697e7728bb03695e2229f587",
						"code": "C-0765",
						"name": "UNKNOWN",
						"name_in_arabic": "مجهول",
						"vat_no": "",
						"phone": "",
						"credit_limit": 0,
						"credit_balance": 0,
						"deleted": false,
					},
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6966a9bbb673ce08466f7c29",
							"name": "b123",
							"part_number": "7374163523",
							"quantity": 1,
							"unit_price": 10,
							"unit_price_with_vat": 11.5,
							"purchase_unit_price": 2,
							"profit": 8,
							"loss": 0,
						},
						map[string]interface{}{
							"product_id": "6966a9aeb673ce08466f7c28",
							"name": "a123",
							"part_number": "3863918361",
							"quantity": 1,
							"unit_price": 10,
							"unit_price_with_vat": 11.5,
							"purchase_unit_price": 5,
							"profit": 5,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"discount_percent": 0,
					"status": "received",
					"total_quantity": 2,
					"vat_price": 3,
					"total": 20,
					"total_with_vat": 23,
					"net_total": 23,
					"cash_discount": 0,
					"payment_status": "paid",
					"payment_methods": []interface{}{
						"credit_card",
					},
					"total_payment_paid": 23,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "697f9eac882854c9dd926468",
							"sales_return_id": "697f9eac882854c9dd926465",
							"sales_return_code": "SR-INV-20260201-001",
							"order_id": "697b270a0a1a0181914c7fdc",
							"amount": 23,
							"method": "credit_card",
						},
					},
					"payments_count": 1,
					"profit": 13,
					"net_profit": 13,
					"loss": 0,
					"net_loss": 0,
					"created_at": "2026-02-01T18:42:51.99Z",
					"customer_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
					"zatca": map[string]interface{}{
						"is_simplified": false,
						"compliance_passed": false,
						"reporting_passed": false,
					},
				},
			},
			"meta": map[string]interface{}{
				"cash_sales_return": 0,
				"bank_account_sales_return": 0,
				"total_sales_return": 0,
				"paid_sales_return": 0,
				"unpaid_sales_return": 0,
				"vat_price": 0,
				"discount": 0,
				"net_profit": 0,
				"net_loss": 0,
			},
		}),
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
		Get: withExample(listOp("sales_return_summary", "Get Sales Return Summary"),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"date": map[string]interface{}{
							"$gte": "2026-02-28T21:00:00Z",
							"$lte": "2026-03-11T20:59:59Z",
						},
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"net_total": 0,
					"vat_price": 0,
					"discount": 0,
					"cash_discount": 0,
					"net_profit": 0,
					"net_loss": 0,
					"paid_sales_return": 0,
					"unpaid_sales_return": 0,
					"cash_sales_return": 0,
					"bank_account_sales_return": 0,
					"shipping_handling_fees": 0,
					"sales_return_count": 0,
					"sales_sales_return": 0,
					"commission": 0,
					"commission_paid_by_cash": 0,
					"commission_paid_by_bank": 0,
				},
			}),
	}
	paths["/v1/sales-return/history"] = openAPIPathItem{
		Get: withExample(listOp("sales_return_history", "List Sales Return History",
			qParam("search[product_id]", "REQUIRED — Product ID to filter history by", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[sales_return_id]", "Filter by sales return ID", false, nil),
			qParam("search[sales_return_code]", "Filter by sales return code", false, nil),
			qParam("search[order_id]", "Filter by related sales order ID", false, nil),
			qParam("search[order_code]", "Filter by related sales order code", false, nil),
			numParam("search[quantity]", "Filter by quantity"),
			numParam("search[price]", "Filter by price"),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			numParam("search[discount]", "Filter by discount"),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
			qParam("search[customer_name]", "Filter by customer name", false, nil),
			qParam("search[profit]", "Filter by profit", false, nil),
			qParam("search[loss]", "Filter by loss", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 0,
				"result": []interface{}{},
				"meta": map[string]interface{}{
					"total_loss": 0,
					"total_profit": 0,
					"total_quantity": 0,
					"total_sales_return": 0,
					"total_vat_return": 0,
				},
			}),
	}
	paths["/v1/sales-return/history/summary"] = openAPIPathItem{
		Get: withExample(listOp("sales_return_history_summary", "Get Sales Return History Summary",
			qParam("search[product_id]", "REQUIRED — Product ID to get sales return history summary for", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[order_id]", "Filter by order ID", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"total_sales_return": 0,
					"total_profit": 0,
					"total_loss": 0,
					"total_vat_return": 0,
					"total_quantity": 0,
				},
			}),
	}

	// ── Quotation Sales Return ────────────────────────────────────────────
	paths["/v1/quotation-sales-return"] = openAPIPathItem{
		Get: withExample(listOpDesc("quotation_sales_return", "List / Search Quotation Sales Returns",
			"Filter by date, customer_id, quotation_id, payment_status. Supports numeric comparison (> < =) on net_total, balance_amount, profit. Response includes a meta object with aggregated totals across all matching records.",
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
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 2,
			"result": []interface{}{
				map[string]interface{}{
					"id": "697fc13d81b333988887131f",
					"quotation_id": "697fbdcd882854c9dd92648d",
					"quotation_code": "QTN-20260201-02",
					"date": "2026-02-01T21:10:16.939Z",
					"code": "QTN-SR-20260202-01",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "695e64f84e491e90c1a25e8d",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6966a9bbb673ce08466f7c29",
							"name": "b123",
							"part_number": "7374163523",
							"quantity": 1,
							"unit_price": 20,
							"unit_price_with_vat": 23,
							"profit": 18,
							"loss": 0,
						},
						map[string]interface{}{
							"product_id": "6966a9aeb673ce08466f7c28",
							"name": "a123",
							"part_number": "3863918361",
							"quantity": 1,
							"unit_price": 10,
							"unit_price_with_vat": 11.5,
							"profit": 5,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "received",
					"total_quantity": 2,
					"vat_price": 4.5,
					"total": 30,
					"total_with_vat": 34.5,
					"net_total": 34.5,
					"cash_discount": 0,
					"payment_status": "paid",
					"payment_methods": []interface{}{
						"credit_card",
					},
					"total_payment_paid": 34.5,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "697fc13d81b3339888871322",
							"quotation_sales_return_id": "697fc13d81b333988887131f",
							"quotation_id": "697fbdcd882854c9dd92648d",
							"amount": 34.5,
							"method": "credit_card",
						},
					},
					"payments_count": 1,
					"profit": 23,
					"net_profit": 23,
					"loss": 0,
					"net_loss": 0,
					"created_at": "2026-02-01T21:10:21.406Z",
					"customer_name": "C2",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
					"zatca": map[string]interface{}{
						"is_simplified": false,
						"compliance_passed": false,
						"reporting_passed": false,
					},
				},
				map[string]interface{}{
					"id": "69cc52706a4f405cf5d4ed28",
					"quotation_id": "69cc52676a4f405cf5d4ed24",
					"quotation_code": "QTN-20260401-02",
					"date": "2026-03-31T23:02:04.228Z",
					"code": "QTN-SR-20260401-01",
					"store_id": "680f6c53e32076f2003a8934",
					"customer_id": "697e7728bb03695e2229f587",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "684064ac42999a68cb049541",
							"name": "PIPE 27NO. (13MM) 5/8 USA-27",
							"part_number": "00121/DA8504/SUC 6241",
							"quantity": 1,
							"unit_price": 200,
							"unit_price_with_vat": 230,
							"profit": 190,
							"loss": 0,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "received",
					"total_quantity": 1,
					"vat_price": 30,
					"total": 200,
					"total_with_vat": 230,
					"net_total": 230,
					"cash_discount": 0,
					"payment_status": "paid",
					"payment_methods": []interface{}{
						"bank_transfer",
					},
					"total_payment_paid": 230,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "69cc52706a4f405cf5d4ed2a",
							"quotation_sales_return_id": "69cc52706a4f405cf5d4ed28",
							"quotation_id": "69cc52676a4f405cf5d4ed24",
							"amount": 230,
							"method": "bank_transfer",
						},
					},
					"payments_count": 1,
					"profit": 190,
					"net_profit": 190,
					"loss": 0,
					"net_loss": 0,
					"created_at": "2026-03-31T23:02:08.595Z",
					"customer_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
					"zatca": map[string]interface{}{
						"is_simplified": false,
						"compliance_passed": false,
						"reporting_passed": false,
					},
				},
			},
			"meta": map[string]interface{}{
				"total_quotation_sales_return": 0,
				"paid_quotation_sales_return": 0,
				"unpaid_quotation_sales_return": 0,
				"cash_quotation_sales_return": 0,
				"bank_account_quotation_sales_return": 0,
				"vat_price": 0,
				"discount": 0,
				"net_profit": 0,
				"net_loss": 0,
			},
		}),
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
	paths["/v1/quotation-sales-return/summary"] = openAPIPathItem{
		Get: withExample(listOp("quotation_sales_return_summary", "Get Quotation Sales Return Summary",
			searchCodeParam(),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[quotation_id]", "Filter by quotation ID", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[status]", "Filter by status", false, nil),
			numParam("search[net_total]", "Filter by net total"),
			numParam("search[discount]", "Filter by discount"),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 2,
				"result": map[string]interface{}{
					"net_total": 264.5,
					"vat_price": 34.5,
					"discount": 0,
					"cash_discount": 0,
					"net_profit": 213,
					"net_loss": 0,
					"paid_quotation_sales_return": 264.5,
					"unpaid_quotation_sales_return": 0,
					"cash_quotation_sales_return": 0,
					"bank_account_quotation_sales_return": 264.5,
					"shipping_handling_fees": 0,
					"quotation_sales_return_count": 0,
					"quotation_sales_quotation_sales_return": 0,
				},
			}),
	}
	paths["/v1/quotation-sales-return/history"] = openAPIPathItem{
		Get: listOp("quotation_sales_return_history", "List Quotation Sales Return History",
			qParam("search[product_id]", "REQUIRED — Product ID to filter history by", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[quotation_sales_return_id]", "Filter by quotation sales return ID", false, nil),
			qParam("search[quotation_sales_return_code]", "Filter by quotation sales return code", false, nil),
			qParam("search[quotation_id]", "Filter by quotation ID", false, nil),
			qParam("search[quotation_code]", "Filter by quotation code", false, nil),
			numParam("search[quantity]", "Filter by quantity"),
			numParam("search[price]", "Filter by price"),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			numParam("search[discount]", "Filter by discount"),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
			qParam("search[customer_name]", "Filter by customer name", false, nil),
		),
	}
	paths["/v1/quotation-sales-return/history/summary"] = openAPIPathItem{
		Get: listOp("quotation_sales_return_history_summary", "Get Quotation Sales Return History Summary",
			qParam("search[product_id]", "REQUIRED — Product ID to get quotation sales return history summary for", true, nil),
			qParam("search[customer_id]", "Filter by customer ID", false, nil),
			qParam("search[quotation_id]", "Filter by quotation ID", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
	}

	// ── Vendor ─────────────────────────────────────────────
	paths["/v1/vendor"] = openAPIPathItem{
		Get: withExample(listOpDesc("vendor", "List / Search Vendors",
			"Filter by name, code, phone, vat_no, deleted. Supports numeric comparison (> < =) on credit_balance, purchase_count, purchase_amount. Response includes a meta object with aggregated totals across all matching records.",
			searchNameParam(), searchCodeParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[phone]", "Filter by phone", false, nil),
			qParam("search[vat_no]", "Filter by VAT number", false, nil),
			qParam("search[query]", "General text search across name/code/email/phone", false, nil),
			numParam("search[credit_balance]", "Filter by credit balance amount"),
			numParam("search[credit_limit]", "Filter by credit limit"),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
		),
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 2,
			"result": []interface{}{
				map[string]interface{}{
					"id": "697e7c79c79aa3d157e6f121",
					"code": "V-0056",
					"name": "UNKNOWN",
					"name_in_arabic": "مجهول",
					"email": "",
					"phone": "",
					"phone2": "",
					"vat_no": "",
					"credit_limit": 0,
					"credit_balance": 0,
					"account": map[string]interface{}{
						"id": "697e7c79c79aa3d157e6f126",
						"store_id": "680f6c53e32076f2003a8934",
						"reference_id": "697e7c79c79aa3d157e6f121",
						"reference_model": "vendor",
						"type": "liability",
						"number": "1012",
						"name": "UNKNOWN",
						"balance": 0,
						"debit_total": 290.95,
						"credit_total": 290.95,
						"deleted": false,
					},
					"deleted": false,
					"created_at": "2026-01-31T22:04:41.285Z",
					"created_by_name": "Sirin k",
					"search_label": "#V-0056 UNKNOWN / مجهول",
					"store_id": "680f6c53e32076f2003a8934",
					"stores": map[string]interface{}{
						"680f6c53e32076f2003a8934": map[string]interface{}{
							"store_id": "680f6c53e32076f2003a8934",
							"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
							"purchase_count": 4,
							"purchase_amount": 327.75,
							"purchase_paid_amount": 327.75,
							"purchase_balance_amount": 0,
							"purchase_return_count": 0,
							"purchase_return_amount": 0,
						},
					},
				},
				map[string]interface{}{
					"id": "698b3f634782b0b706464edb",
					"code": "V-0057",
					"name": "V1",
					"name_in_arabic": "",
					"email": "",
					"phone": "",
					"phone2": "",
					"vat_no": "",
					"vat_percent": 15,
					"credit_limit": 0,
					"credit_balance": 0,
					"account": nil,
					"deleted": false,
					"created_at": "2026-02-10T14:23:31.75Z",
					"created_by_name": "Sirin k",
					"search_label": "#V-0057 V1",
					"store_id": "680f6c53e32076f2003a8934",
					"stores": nil,
				},
			},
			"meta": map[string]interface{}{
				"purchase": 0,
				"purchase_count": 0,
				"purchase_paid": 0,
				"purchase_paid_count": 0,
				"purchase_unpaid_count": 0,
				"purchase_paid_partially_count": 0,
				"purchase_credit_balance": 0,
				"credit_balance": 0,
				"purchase_return": 0,
				"purchase_return_count": 0,
			},
		}),
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
	paths["/v1/vendor/summary"] = openAPIPathItem{
		Get: listOp("vendor_summary", "Get Vendor Summary",
			searchNameParam(), searchCodeParam(),
			qParam("search[email]", "Filter by email", false, nil),
			qParam("search[phone]", "Filter by phone", false, nil),
			qParam("search[vat_no]", "Filter by VAT number", false, nil),
			qParam("search[deleted]", "Pass 1 to include deleted records", false, nil),
		),
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
		Get: withExample(listOpDesc("purchase", "List / Search Purchases",
			"Filter by date, vendor_id, payment_status. Supports numeric comparison (> < =) on net_total, balance_amount. Response includes a meta object with aggregated totals across all matching records.",
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
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 4,
			"result": []interface{}{
				map[string]interface{}{
					"id": "695d2d524e491e90c1a25e35",
					"date": "2026-01-06T15:41:45.062Z",
					"code": "P-INV-20260106-001",
					"store_id": "680f6c53e32076f2003a8934",
					"vendor_id": "697e7c79c79aa3d157e6f121",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6934285a8d5448cff62ac8d9",
							"name": "BLOWER FAN TOYOTA RAV-4 NEW 2018-24",
							"part_number": "CXA-2942",
							"quantity": 1,
							"quantity_returned": 1,
							"purchase_unit_price": 230,
							"purchase_unit_price_with_vat": 264.5,
						},
						map[string]interface{}{
							"product_id": "69443ec5831cb214ae340d6c",
							"name": "EVAPORATOR SCANIA OLD MODEL",
							"part_number": "CXA-5221B",
							"quantity": 1,
							"quantity_returned": 1,
							"purchase_unit_price": 23,
							"purchase_unit_price_with_vat": 26.45,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "delivered",
					"total_quantity": 2,
					"vat_price": 37.95,
					"total": 253,
					"total_with_vat": 290.95,
					"net_total": 290.95,
					"cash_discount": 0,
					"payment_status": "paid",
					"return_count": 1,
					"return_amount": 290.95,
					"total_payment_paid": 290.95,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "695d2d524e491e90c1a25e38",
							"purchase_id": "695d2d524e491e90c1a25e35",
							"purchase_code": "P-INV-20260106-001",
							"amount": 290.95,
							"method": "bank_transfer",
						},
					},
					"payments_count": 1,
					"payment_methods": []interface{}{
						"bank_transfer",
					},
					"created_at": "2026-01-06T15:42:10.482Z",
					"vendor_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
				},
				map[string]interface{}{
					"id": "697fbd3b882854c9dd926489",
					"date": "2026-02-01T20:53:00.814Z",
					"code": "P-INV-20260201-001",
					"store_id": "680f6c53e32076f2003a8934",
					"vendor_id": "697e7c79c79aa3d157e6f121",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6966a9bbb673ce08466f7c29",
							"name": "b123",
							"part_number": "7374163523",
							"quantity": 1,
							"purchase_unit_price": 2,
							"purchase_unit_price_with_vat": 2.3,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "delivered",
					"total_quantity": 1,
					"vat_price": 0.3,
					"total": 2,
					"total_with_vat": 2.3,
					"net_total": 2.3,
					"cash_discount": 0,
					"payment_status": "paid",
					"return_count": 0,
					"return_amount": 0,
					"total_payment_paid": 2.3,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "697fbd3b882854c9dd92648b",
							"purchase_id": "697fbd3b882854c9dd926489",
							"purchase_code": "P-INV-20260201-001",
							"amount": 2.3,
							"method": "cash",
						},
					},
					"payments_count": 1,
					"payment_methods": []interface{}{
						"cash",
					},
					"created_at": "2026-02-01T20:53:14.996Z",
					"vendor_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
				},
			},
			"meta": map[string]interface{}{
				"cash_purchase": 0,
				"bank_account_purchase": 0,
				"total_purchase": 0,
				"paid_purchase": 0,
				"unpaid_purchase": 0,
				"vat_price": 0,
				"discount": 0,
				"return_count": 0,
				"return_amount": 0,
			},
		}),
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
	paths["/v1/purchase/summary"] = openAPIPathItem{
		Get: withExample(listOp("purchase_summary", "Get Purchase Summary",
			searchCodeParam(),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[status]", "Filter by status", false, nil),
			numParam("search[net_total]", "Filter by net total"),
			numParam("search[balance_amount]", "Filter by balance amount"),
			numParam("search[discount]", "Filter by discount"),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"date": map[string]interface{}{
							"$gte": "2026-02-28T21:00:00Z",
							"$lte": "2026-03-11T20:59:59Z",
						},
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"net_total": 0,
					"vat_price": 0,
					"discount": 0,
					"cash_discount": 0,
					"shipping_handling_fees": 0,
					"net_retail_net_profit": 0,
					"net_wholesale_profit": 0,
					"paid_purchase": 0,
					"unpaid_purchase": 0,
					"cash_purchase": 0,
					"bank_account_purchase": 0,
					"return_count": 0,
					"return_amount": 0,
					"sales_purchase": 0,
					"purchase_return_purchase": 0,
				},
			}),
	}
	paths["/v1/purchase/history"] = openAPIPathItem{
		Get: withExample(listOp("purchase_history", "List Purchase History",
			qParam("search[product_id]", "REQUIRED — Product ID to filter history by", true, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_id]", "Filter by purchase ID", false, nil),
			qParam("search[purchase_code]", "Filter by purchase code", false, nil),
			numParam("search[quantity]", "Filter by quantity"),
			numParam("search[price]", "Filter by price"),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[net_price]", "Filter by net price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			numParam("search[discount]", "Filter by discount"),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
			qParam("search[vendor_name]", "Filter by vendor name", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 1,
				"result": []interface{}{
					map[string]interface{}{
						"id": "69cc519c6a4f405cf5d4ed19",
						"date": "2026-03-31T22:58:29.871Z",
						"store_id": "680f6c53e32076f2003a8934",
						"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
						"product_id": "684064ac42999a68cb049541",
						"vendor_id": "697e7c79c79aa3d157e6f121",
						"vendor_name": "UNKNOWN",
						"vendor_name_arabic": "",
						"purchase_id": "69cc519c6a4f405cf5d4ed18",
						"purchase_code": "P-INV-20260401-001",
						"quantity": 1,
						"unit_price": 10,
						"unit_discount": 0,
						"discount": 0,
						"discount_percent": 0,
						"price": 10,
						"net_price": 11.5,
						"retail_profit": 0,
						"wholesale_profit": 0,
						"retail_loss": 0,
						"wholesale_loss": 0,
						"vat_percent": 15,
						"vat_price": 1.5,
						"unit": "Meter(s)",
						"unit_price_with_vat": 11.5,
						"created_at": "2026-03-31T22:58:36.025Z",
						"updated_at": "2026-03-31T22:58:36.025Z",
						"warehouse_code": "main_store",
					},
				},
				"meta": map[string]interface{}{
					"total_purchase": 11.5,
					"total_quantity": 1,
					"total_retail_loss": 0,
					"total_retail_profit": 0,
					"total_vat": 1.5,
					"total_wholesale_loss": 0,
					"total_wholesale_profit": 0,
				},
			}),
	}
	paths["/v1/purchase/history/summary"] = openAPIPathItem{
		Get: withExample(listOp("purchase_history_summary", "Get Purchase History Summary",
			qParam("search[product_id]", "REQUIRED — Product ID to get purchase history summary for", true, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_id]", "Filter by purchase ID", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"total_purchase": 0,
					"total_retail_profit": 0,
					"total_wholesale_profit": 0,
					"total_retail_loss": 0,
					"total_wholesale_loss": 0,
					"total_vat": 0,
					"total_quantity": 0,
				},
			}),
	}

	// ── Purchase Return ────────────────────────────────────────────
	paths["/v1/purchase-return"] = openAPIPathItem{
		Get: withExample(listOpDesc("purchase_return", "List / Search Purchase Returns",
			"Filter by date, vendor_id, purchase_id, payment_status. Supports numeric comparison (> < =) on net_total, balance_amount. Response includes a meta object with aggregated totals across all matching records.",
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
		map[string]interface{}{
			"status": true,
			"criterias": map[string]interface{}{
				"page": 1,
				"size": 10,
				"search_by": map[string]interface{}{
					"deleted": map[string]interface{}{
						"$ne": true,
					},
					"store_id": "680f6c53e32076f2003a8934",
				},
			},
			"total_count": 2,
			"result": []interface{}{
				map[string]interface{}{
					"id": "697fbc50882854c9dd926483",
					"purchase_id": "695d2d524e491e90c1a25e35",
					"purchase_code": "P-INV-20260106-001",
					"date": "2026-02-01T20:49:16.469Z",
					"code": "PR-INV-20260201-001",
					"store_id": "680f6c53e32076f2003a8934",
					"vendor_id": "697e7c79c79aa3d157e6f121",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "6934285a8d5448cff62ac8d9",
							"name": "BLOWER FAN TOYOTA RAV-4 NEW 2018-24",
							"part_number": "CXA-2942",
							"quantity": 1,
							"purchasereturn_unit_price": 230,
							"purchasereturn_unit_price_with_vat": 264.5,
						},
						map[string]interface{}{
							"product_id": "69443ec5831cb214ae340d6c",
							"name": "EVAPORATOR SCANIA OLD MODEL",
							"part_number": "CXA-5221B",
							"quantity": 1,
							"purchasereturn_unit_price": 23,
							"purchasereturn_unit_price_with_vat": 26.45,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "delivered",
					"total_quantity": 2,
					"vat_price": 37.95,
					"total": 253,
					"total_with_vat": 290.95,
					"net_total": 290.95,
					"cash_discount": 0,
					"payment_status": "paid",
					"total_payment_paid": 290.95,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "697fbc50882854c9dd926486",
							"purchase_return_id": "697fbc50882854c9dd926483",
							"purchase_return_code": "PR-INV-20260201-001",
							"purchase_id": "695d2d524e491e90c1a25e35",
							"amount": 290.95,
							"method": "bank_card",
						},
					},
					"payments_count": 1,
					"payment_methods": []interface{}{
						"bank_card",
					},
					"created_at": "2026-02-01T20:49:20.62Z",
					"vendor_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
				},
				map[string]interface{}{
					"id": "69cc51a46a4f405cf5d4ed1c",
					"purchase_id": "69cc519c6a4f405cf5d4ed18",
					"purchase_code": "P-INV-20260401-001",
					"date": "2026-03-31T22:58:38.975Z",
					"code": "PR-INV-20260401-001",
					"store_id": "680f6c53e32076f2003a8934",
					"vendor_id": "697e7c79c79aa3d157e6f121",
					"products": []interface{}{
						map[string]interface{}{
							"product_id": "684064ac42999a68cb049541",
							"name": "PIPE 27NO. (13MM) 5/8 USA-27",
							"part_number": "00121/DA8504/SUC 6241",
							"quantity": 1,
							"purchasereturn_unit_price": 10,
							"purchasereturn_unit_price_with_vat": 11.5,
						},
					},
					"vat_percent": 15,
					"discount": 0,
					"status": "delivered",
					"total_quantity": 1,
					"vat_price": 1.5,
					"total": 10,
					"total_with_vat": 11.5,
					"net_total": 11.5,
					"cash_discount": 0,
					"payment_status": "paid",
					"total_payment_paid": 11.5,
					"balance_amount": 0,
					"payments": []interface{}{
						map[string]interface{}{
							"id": "69cc51a46a4f405cf5d4ed1e",
							"purchase_return_id": "69cc51a46a4f405cf5d4ed1c",
							"purchase_return_code": "PR-INV-20260401-001",
							"amount": 11.5,
							"method": "bank_transfer",
						},
					},
					"payments_count": 1,
					"payment_methods": []interface{}{
						"bank_transfer",
					},
					"created_at": "2026-03-31T22:58:44.261Z",
					"vendor_name": "UNKNOWN",
					"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
					"created_by_name": "Sirin k",
				},
			},
			"meta": map[string]interface{}{
				"cash_purchase_return": 0,
				"bank_account_purchase_return": 0,
				"total_purchase_return": 0,
				"paid_purchase_return": 0,
				"unpaid_purchase_return": 0,
				"vat_price": 0,
				"discount": 0,
			},
		}),
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
	paths["/v1/purchase-return/summary"] = openAPIPathItem{
		Get: withExample(listOp("purchase_return_summary", "Get Purchase Return Summary",
			searchCodeParam(),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_id]", "Filter by original purchase ID", false, nil),
			qParam("search[payment_status]", "Filter by payment status: paid, not_paid, paid_partially", false, nil),
			qParam("search[status]", "Filter by status", false, nil),
			numParam("search[net_total]", "Filter by net total"),
			numParam("search[balance_amount]", "Filter by balance amount"),
			numParam("search[discount]", "Filter by discount"),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"date": map[string]interface{}{
							"$gte": "2026-02-28T21:00:00Z",
							"$lte": "2026-03-11T20:59:59Z",
						},
						"deleted": map[string]interface{}{
							"$ne": true,
						},
						"store_id": "680f6c53e32076f2003a8934",
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"net_total": 0,
					"vat_price": 0,
					"discount": 0,
					"cash_discount": 0,
					"paid_purchase_return": 0,
					"purchase_return_count": 0,
					"unpaid_purchase_return": 0,
					"cash_purchase_return": 0,
					"bank_account_purchase_return": 0,
					"shipping_handling_fees": 0,
					"purchase_purchase_return": 0,
				},
			}),
	}
	paths["/v1/purchase-return/history"] = openAPIPathItem{
		Get: withExample(listOp("purchase_return_history", "List Purchase Return History",
			qParam("search[product_id]", "REQUIRED — Product ID to filter history by", true, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_return_id]", "Filter by purchase return ID", false, nil),
			qParam("search[purchase_return_code]", "Filter by purchase return code", false, nil),
			qParam("search[purchase_id]", "Filter by original purchase ID", false, nil),
			qParam("search[purchase_code]", "Filter by original purchase code", false, nil),
			numParam("search[quantity]", "Filter by quantity"),
			numParam("search[price]", "Filter by price"),
			qParam("search[unit_price]", "Filter by unit price", false, nil),
			qParam("search[net_price]", "Filter by net price", false, nil),
			qParam("search[vat_price]", "Filter by VAT amount", false, nil),
			numParam("search[discount]", "Filter by discount"),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
			qParam("search[vendor_name]", "Filter by vendor name", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 1,
				"result": []interface{}{
					map[string]interface{}{
						"id": "69cc51a46a4f405cf5d4ed1d",
						"date": "2026-03-31T22:58:38.975Z",
						"store_id": "680f6c53e32076f2003a8934",
						"store_name": "Ghali Jabr Musleh Noimi Al-Ma'bady Trading Establishment",
						"product_id": "684064ac42999a68cb049541",
						"vendor_id": "697e7c79c79aa3d157e6f121",
						"vendor_name": "UNKNOWN",
						"vendor_name_arabic": "",
						"purchase_return_id": "69cc51a46a4f405cf5d4ed1c",
						"purchase_return_code": "PR-INV-20260401-001",
						"purchase_id": "69cc519c6a4f405cf5d4ed18",
						"purchase_code": "P-INV-20260401-001",
						"quantity": 1,
						"unit_price": 10,
						"unit_discount": 0,
						"discount": 0,
						"discount_percent": 0,
						"price": 10,
						"net_price": 11.5,
						"vat_percent": 15,
						"vat_price": 1.5,
						"unit": "Meter(s)",
						"unit_price_with_vat": 11.5,
						"created_at": "2026-03-31T22:58:44.261Z",
						"updated_at": "2026-03-31T22:58:44.261Z",
						"warehouse_code": "main_store",
					},
				},
				"meta": map[string]interface{}{
					"total_purchase_return": 11.5,
					"total_quantity": 1,
					"total_vat_return": 1.5,
				},
			}),
	}
	paths["/v1/purchase-return/history/summary"] = openAPIPathItem{
		Get: withExample(listOp("purchase_return_history_summary", "Get Purchase Return History Summary",
			qParam("search[product_id]", "REQUIRED — Product ID to get purchase return history summary for", true, nil),
			qParam("search[vendor_id]", "Filter by vendor ID", false, nil),
			qParam("search[purchase_return_id]", "Filter by purchase return ID", false, nil),
			qParam("search[warehouse_code]", "Filter by warehouse code", false, nil),
		),
			map[string]interface{}{
				"status": true,
				"criterias": map[string]interface{}{
					"page": 1,
					"size": 10,
					"search_by": map[string]interface{}{
						"product_id": "684064ac42999a68cb049541",
						"store_id": "680f6c53e32076f2003a8934",
					},
					"sort_by": map[string]interface{}{
						"created_at": -1,
					},
				},
				"total_count": 0,
				"result": map[string]interface{}{
					"total_purchase_return": 0,
					"total_vat_return": 0,
					"total_quantity": 0,
				},
			}),
	}

	// ── Purchase Cash Discount ────────────────────────────────────────────
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
		// Core
		"/v1/me",
		"/v1/store/list",
		// Sales
		//"/v1/sales/summary",
		"/v1/order",
		"/v1/sales/history",
		//"/v1/sales/history/summary",
		// Sales Return
		"/v1/sales-return",
		"/v1/sales-return/history",
		//"/v1/sales-return/history/summary",
		// Purchase
		"/v1/purchase",
		//"/v1/purchase/summary",
		"/v1/purchase/history",
		//"/v1/purchase/history/summary",
		// Purchase Return
		"/v1/purchase-return",
		//"/v1/purchase-return/summary",
		"/v1/purchase-return/history",
		//"/v1/purchase-return/history/summary",
		// Quotation
		"/v1/quotation",
		//"/v1/quotation/summary",
		//"/v1/quotation/sales/summary",
		"/v1/quotation/history",
		//"/v1/quotation/history/summary",
		// Quotation Sales Return
		"/v1/quotation-sales-return",
		"/v1/quotation-sales-return/summary",
		"/v1/quotation-sales-return/history",
		"/v1/quotation-sales-return/history/summary",
		// Products
		"/v1/product",
		"/v1/product/summary",
		"/v1/product/history/{id}",
		"/v1/product/history/summary/{id}",
		// Customers & Vendors
		"/v1/customer",
		"/v1/customer/summary",
		"/v1/vendor",
		"/v1/vendor/summary",
		// Expense
		"/v1/expense",
		"/v1/expense/summary",
		// Accounting
		"/v1/account",
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
