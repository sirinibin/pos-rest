// UI e2e: the StartERP UI driven against the pos-rest /v1/erp adapter, with
// a legacy-shaped fixture seeded into a test DB (see ../run.sh).
// Verifies through the OLD v1 endpoints that what the UI wrote is readable by
// the existing app.
import { test, expect } from "@playwright/test";
import fs from "node:fs";

const fx = JSON.parse(fs.readFileSync(process.env.E2E_FIXTURE || "fixture.json", "utf8"));
const API = process.env.E2E_API || "http://localhost:2010";
const EMAIL = fx.managerEmail.toLowerCase(); // stored mixed-case: login is case-insensitive

// ---------- tiny API clients ----------
async function erpLogin() {
  const r = await fetch(API + "/v1/erp/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email: EMAIL, password: fx.password }) });
  expect(r.status).toBe(200);
  return (await r.json()).accessToken;
}
async function erpGet(tok, path) {
  const r = await fetch(API + "/v1/erp" + path, { headers: { Authorization: "Bearer " + tok } });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}
async function legacyId(tok, path, id) {
  let rec;
  await expect.poll(async () => (rec = await erpGet(tok, "/" + path + "/" + encodeURIComponent(id))).status).toBe(200);
  expect(rec.body.id).toMatch(/^[0-9a-f]{24}$/);
  return rec.body.id;
}
// legacy v1 (old app) — same access token works for both APIs
async function v1Get(tok, path) {
  const sep = path.includes("?") ? "&" : "?";
  const r = await fetch(API + "/v1" + path + sep + "search[store_id]=" + fx.storeA, { headers: { Authorization: "Bearer " + tok } });
  return r.json();
}

// ---------- UI helpers (same flows as the StartERP REST suite) ----------
const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const primary = (page, name) => page.getByRole("button", { name, exact: true }).filter({ visible: true }).first();
const idFromUrl = (url, base) => {
  const m = new RegExp("#" + base + "/([^/?#]+)$").exec(url);
  return m && m[1] !== "new" ? decodeURIComponent(m[1]) : null;
};
async function go(page, hash) {
  await page.evaluate((h) => (location.hash = h), hash);
  await page.waitForFunction((h) => location.hash === h, hash);
}
async function open(page) {
  await page.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.fulfill({ status: 200, contentType: "text/css", body: "" }));
  const q = `/?provider=rest&apiBase=${encodeURIComponent(process.env.E2E_BASE || "http://localhost:5180")}`;
  await page.goto(q + "#/login");
  await page.evaluate(() => {
    for (const k of Object.keys(localStorage)) if (!/^starterp-(provider|api-base)$/.test(k)) localStorage.removeItem(k);
    sessionStorage.clear();
  });
  await page.goto(q + "#/login");
}
async function uiLogin(page) {
  await page.getByLabel(/^(Work email|Email)/).fill(EMAIL);
  await page.getByLabel(/^Password/).filter({ visible: true }).first().fill(fx.password);
  await primary(page, "Sign in").click();
  await page.waitForURL(/#\/app\//, { timeout: 30_000 });
  const tips = page.getByRole("button", { name: "Hide all tips" });
  if (await tips.isVisible().catch(() => false)) await tips.click();
}

test.describe.configure({ mode: "serial" });

test("UI against the adapter: legacy data, customer, product, sale with payment, return", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const apiErrors = [];
  page.on("response", async (r) => {
    if (r.url().includes("/v1/") && r.status() >= 500) apiErrors.push(`${r.status()} ${r.request().method()} ${r.url()}`);
  });
  await open(page);
  await uiLogin(page);
  const tok = await erpLogin();
  const stamp = Date.now().toString(36);

  // 1) legacy records written by the OLD app are visible in the UI
  await go(page, "#/app/sales");
  await expect(page.locator(".app-main")).toContainText("S-INV-001");
  await go(page, "#/app/customers");
  await expect(page.locator(".app-main")).toContainText("Riyadh Motors");
  await go(page, "#/app/sales/" + fx.orderA2);
  await expect(page.locator(".app-main")).toContainText("S-INV-000"); // old-shape invoice (no embedded payments)

  // 2) create a customer
  const custName = "E2E Customer " + stamp;
  await go(page, "#/app/customers/new");
  await page.getByLabel(/^Name \(English\)/).fill(custName);
  await page.getByLabel(/^Name \(Arabic\)/).fill("عميل الاختبار");
  await page.getByLabel(/^VAT number/).fill("310000000000003");
  await page.getByLabel(/^Phone$/).fill("0551234567");
  await primary(page, "Save").click();
  await page.waitForURL(/#\/app\/customers\/(?!new)[^/]+$/);
  // the UI may keep its client id (contract clientIds); the adapter maps it to the legacy ObjectID
  const custId = await legacyId(tok, "customers", idFromUrl(page.url(), "/app/customers"));

  // 3) create a product
  const prodName = "E2E Product " + stamp;
  await go(page, "#/app/products/new");
  await page.getByLabel(/^Name \(EN\)/).fill(prodName);
  await page.getByRole("tab", { name: "Pricing" }).click();
  await page.getByLabel("Purchase price ex. VAT").fill("50");
  await page.getByLabel("Retail price ex. VAT").fill("80");
  await primary(page, "Create").click();
  await page.waitForURL(/#\/app\/products\/(?!new)[^/]+$/);
  const prodId = await legacyId(tok, "products", idFromUrl(page.url(), "/app/products"));

  // 4) sale with payment (legacy product with stock, new customer)
  await go(page, "#/app/sales/new");
  const cbox = page.getByPlaceholder("Search name, phone, code or VAT no.").first();
  await cbox.click();
  await cbox.fill(custName);
  await page.getByRole("option", { name: new RegExp(escapeRe(custName), "i") }).first().click();
  const pbox = page.getByPlaceholder(/Search product by name, part no\. or barcode/);
  await pbox.fill("Oil Filter");
  await pbox.press("Enter");
  await expect(page.locator(".docs-items-wrap")).toContainText("Oil Filter");
  await page.getByRole("button", { name: "Paid in full" }).click();
  await primary(page, "Create").click();
  await page.waitForURL(/#\/app\/sales\/(?!new)[^/]+$/, { timeout: 30_000 });
  const saleId = await legacyId(tok, "sales", idFromUrl(page.url(), "/app/sales"));
  const sale = (await erpGet(tok, "/sales/" + saleId)).body;
  expect(sale.payments.length).toBeGreaterThan(0);

  // 5) sales return of that invoice via the UI
  await go(page, "#/app/sales-returns/new");
  const ibox = page.getByLabel(/^Original invoice/);
  await ibox.click();
  await ibox.fill(sale.code);
  await page.getByRole("option", { name: new RegExp(escapeRe(sale.code)) }).first().click();
  const lines = page.getByRole("checkbox", { name: "Return this item" });
  await expect(lines.first()).toBeVisible();
  for (const cb of await lines.all()) await cb.check();
  await primary(page, "Create").click();
  await page.waitForURL(/#\/app\/sales-returns\/(?!new)[^/]+$/, { timeout: 30_000 });
  const retId = await legacyId(tok, "sales-returns", idFromUrl(page.url(), "/app/sales-returns"));

  // 6) the OLD app (v1 endpoints) reads everything the UI wrote
  const c = await v1Get(tok, "/customer/" + custId);
  expect(c.status, JSON.stringify(c)).toBe(true);
  expect(c.result.name.toLowerCase()).toBe(custName.toLowerCase()); // legacy upper-cases customer names
  const p = await v1Get(tok, "/product/" + prodId);
  expect(p.result?.name, JSON.stringify(p).slice(0, 300)).toBe(prodName);
  const o = await v1Get(tok, "/order/" + saleId);
  expect(o.status).toBe(true);
  expect(o.result.code).toBe(sale.code);
  expect(o.result.customer_id).toBe(custId);
  expect(o.result.products[0].product_id).toBe(fx.productA2);
  await expect.poll(async () => (await v1Get(tok, "/sales-payment?search[order_id]=" + saleId)).result?.length || 0).toBeGreaterThan(0);
  const r = await v1Get(tok, "/sales-return/" + retId);
  expect(r.status).toBe(true);
  expect(r.result.order_id).toBe(saleId);
  await expect.poll(async () => (await v1Get(tok, "/order/" + saleId)).result.products[0].quantity_returned).toBe(o.result.products[0].quantity);

  expect(errors, "no uncaught page errors").toEqual([]);
  expect(apiErrors, "no 5xx from the adapter").toEqual([]);
});
