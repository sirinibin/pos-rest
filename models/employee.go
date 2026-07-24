package models

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Employee : Employee structure
type Employee struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Code         string             `bson:"code,omitempty" json:"code,omitempty"`
	Name         string             `bson:"name" json:"name"`
	NameInArabic string             `bson:"name_in_arabic" json:"name_in_arabic"`
	Position     string             `bson:"position,omitempty" json:"position,omitempty"`
	Mob1         string             `bson:"mob1" json:"mob1"`
	Mob2         string             `bson:"mob2" json:"mob2"`
	IqamaNo      string             `bson:"iqama_no" json:"iqama_no"`
	Address      string             `bson:"address" json:"address"`
	Salary       float64            `bson:"salary" json:"salary"`
	SalaryDay    int                `bson:"salary_day" json:"salary_day"` // 1–28
	JoiningDate  *time.Time         `bson:"joining_date,omitempty" json:"joining_date,omitempty"`
	// OpeningBalance/OpeningBalanceDate support migrating employees from an
	// existing (external) payroll system: OpeningBalance is the amount already
	// owed to the employee as of OpeningBalanceDate ("cutover" day), posted as a
	// single one-time ledger entry. Automated salary-due accrual never looks
	// further back than this date, regardless of how far in the past JoiningDate
	// is — that history is assumed to already be settled/tracked by the old system.
	OpeningBalance       float64             `bson:"opening_balance" json:"opening_balance"`
	OpeningBalanceDate   *time.Time          `bson:"opening_balance_date,omitempty" json:"opening_balance_date,omitempty"`
	OpeningBalancePosted bool                `bson:"opening_balance_posted" json:"opening_balance_posted"`
	IsActive             bool                `bson:"is_active" json:"is_active"`
	Account              *Account            `json:"account" bson:"account"`
	StoreID              *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName            string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	Deleted              bool                `bson:"deleted" json:"deleted"`
	DeletedBy            *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt            *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt            *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt            *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy            *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy            *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName        string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName        string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName        string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
}

// EmployeeSalaryPayment records a salary payment to an employee.
type EmployeeSalaryPayment struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code          string              `bson:"code,omitempty" json:"code,omitempty"`
	EmployeeID    *primitive.ObjectID `json:"employee_id" bson:"employee_id"`
	EmployeeName  string              `json:"employee_name" bson:"employee_name"`
	StoreID       *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
	Amount        float64             `bson:"amount" json:"amount"`
	PaymentMethod string              `json:"payment_method" bson:"payment_method"` // "cash" or "bank_transfer"
	Month         int                 `bson:"month" json:"month"`
	Year          int                 `bson:"year" json:"year"`
	Description   string              `bson:"description" json:"description"`
	Deleted       bool                `bson:"deleted" json:"deleted"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt     *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
}

// SalaryDueEntry is an idempotency record used by the background worker to avoid
// creating duplicate salary accrual ledger entries.
type SalaryDueEntry struct {
	ID         primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	EmployeeID *primitive.ObjectID `json:"employee_id" bson:"employee_id"`
	StoreID    *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Month      int                 `bson:"month" json:"month"`
	Year       int                 `bson:"year" json:"year"`
	Amount     float64             `bson:"amount" json:"amount"`
	CreatedAt  *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
}

// EmployeeStats holds aggregate totals for the employee list.
type EmployeeStats struct {
	ID                   *primitive.ObjectID `json:"id" bson:"_id"`
	TotalEmployees       int64               `json:"total_employees" bson:"total_employees"`
	TotalSalary          float64             `json:"total_salary" bson:"total_salary"`
	TotalOwedToEmployees float64             `json:"total_owed_to_employees" bson:"total_owed_to_employees"`
	TotalEmployeesOwe    float64             `json:"total_employees_owe" bson:"total_employees_owe"`
	TotalSalaryPaid      float64             `json:"total_salary_paid" bson:"total_salary_paid"`
}

// GetEmployeeStats aggregates salary, balance, and payment totals for the
// given employee filter. Account balances are joined live from the account
// collection (which is always updated by SetAccountBalances on every
// payment create/update/delete) — never from the stale cached snapshot
// embedded in the employee document. Salary payments are also joined live
// so deleted payments are excluded immediately.
func (store *Store) GetEmployeeStats(filter map[string]interface{}) (stats EmployeeStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$match": filter},
		// Join the live account balance (updated by SetAccountBalances on every
		// transaction) instead of the stale embedded snapshot.
		{
			"$lookup": bson.M{
				"from": "account",
				"let":  bson.M{"emp_id": "$_id"},
				"pipeline": []bson.M{
					{"$match": bson.M{"$expr": bson.M{"$and": []interface{}{
						bson.M{"$eq": []interface{}{"$reference_id", "$$emp_id"}},
						bson.M{"$eq": []interface{}{"$reference_model", "employee"}},
					}}}},
				},
				"as": "live_accounts",
			},
		},
		// Join non-deleted salary payments for this employee.
		{
			"$lookup": bson.M{
				"from": "employee_salary_payment",
				"let":  bson.M{"emp_id": "$_id"},
				"pipeline": []bson.M{
					{"$match": bson.M{"$expr": bson.M{"$and": []interface{}{
						bson.M{"$eq": []interface{}{"$employee_id", "$$emp_id"}},
						bson.M{"$ne": []interface{}{"$deleted", true}},
					}}}},
				},
				"as": "salary_payments",
			},
		},
		// Promote the first (and only expected) account to a top-level field
		// to avoid $unwind inflating the employee count when an account is missing.
		{"$addFields": bson.M{
			"live_account": bson.M{"$arrayElemAt": []interface{}{"$live_accounts", 0}},
		}},
		{"$group": bson.M{
			"_id":             nil,
			"total_employees": bson.M{"$sum": 1},
			"total_salary":    bson.M{"$sum": "$salary"},
			"total_owed_to_employees": bson.M{"$sum": bson.M{
				"$cond": bson.A{
					bson.M{"$eq": bson.A{"$live_account.debit_or_credit_balance", "credit_balance"}},
					"$live_account.balance",
					0,
				},
			}},
			"total_employees_owe": bson.M{"$sum": bson.M{
				"$cond": bson.A{
					bson.M{"$eq": bson.A{"$live_account.debit_or_credit_balance", "debit_balance"}},
					"$live_account.balance",
					0,
				},
			}},
			"total_salary_paid": bson.M{"$sum": bson.M{"$sum": "$salary_payments.amount"}},
		}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		if err := cur.Decode(&stats); err != nil {
			return stats, err
		}
		stats.TotalSalary = RoundFloat(stats.TotalSalary, 2)
		stats.TotalOwedToEmployees = RoundFloat(stats.TotalOwedToEmployees, 2)
		stats.TotalEmployeesOwe = RoundFloat(stats.TotalEmployeesOwe, 2)
		stats.TotalSalaryPaid = RoundFloat(stats.TotalSalaryPaid, 2)
	}
	return stats, nil
}

// ──────────────────────────────────────────────────────────
// Employee CRUD
// ──────────────────────────────────────────────────────────

func (employee *Employee) UpdateForeignLabelFields() error {
	if employee.CreatedBy != nil {
		u, err := FindUserByID(employee.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding created_by user:" + err.Error())
		}
		employee.CreatedByName = u.Name
	}

	if employee.UpdatedBy != nil {
		u, err := FindUserByID(employee.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding updated_by user:" + err.Error())
		}
		employee.UpdatedByName = u.Name
	}

	if employee.StoreID != nil {
		store, err := FindStoreByID(employee.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		employee.StoreName = store.Name
	}

	return nil
}

func (employee *Employee) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	if employee.StoreID == nil || employee.StoreID.IsZero() {
		errs["store_id"] = "Store ID is required"
	}

	if employee.Name == "" {
		errs["name"] = "Name is required"
	}

	if employee.Salary < 0 {
		errs["salary"] = "Salary cannot be negative"
	}

	if employee.SalaryDay < 1 || employee.SalaryDay > 28 {
		errs["salary_day"] = "Salary day must be between 1 and 28"
	}

	if employee.JoiningDate == nil {
		errs["joining_date"] = "Joining date is required"
	}

	if employee.OpeningBalance < 0 {
		errs["opening_balance"] = "Opening balance cannot be negative"
	}
	if employee.OpeningBalance != 0 && employee.OpeningBalanceDate == nil {
		errs["opening_balance_date"] = "Opening balance date is required when an opening balance is entered"
	}

	if strings.TrimSpace(employee.Mob1) != "" && employee.StoreID != nil && !employee.StoreID.IsZero() {
		exists, err := employee.IsMob1Exists()
		if err != nil {
			errs["mob1"] = err.Error()
		} else if exists {
			errs["mob1"] = "Mobile 1 already exists"
		}
	}

	return errs
}

// IsMob1Exists checks whether another employee (in the same store) already has this Mob1 number.
func (employee *Employee) IsMob1Exists() (exists bool, err error) {
	collection := db.GetDB("store_" + employee.StoreID.Hex()).Collection("employee")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if employee.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"mob1":     employee.Mob1,
			"store_id": employee.StoreID,
			"deleted":  bson.M{"$ne": true},
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"mob1":     employee.Mob1,
			"store_id": employee.StoreID,
			"deleted":  bson.M{"$ne": true},
			"_id":      bson.M{"$ne": employee.ID},
		})
	}

	return (count > 0), err
}

func (employee *Employee) Insert() error {
	collection := db.GetDB("store_" + employee.StoreID.Hex()).Collection("employee")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	employee.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &employee)
	return err
}

func (employee *Employee) Update() error {
	collection := db.GetDB("store_" + employee.StoreID.Hex()).Collection("employee")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	employee.UpdatedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": employee.ID},
		bson.M{"$set": employee},
		options.Update().SetUpsert(false),
	)
	return err
}

func (store *Store) FindEmployeeByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (employee *Employee, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx, bson.M{
		"_id":      ID,
		"store_id": store.ID,
	}, findOneOptions).Decode(&employee)
	if err != nil {
		return nil, err
	}

	return employee, nil
}

func (store *Store) SearchEmployee(w http.ResponseWriter, r *http.Request) (employees []Employee, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var keys []string
	var ok bool

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return employees, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[search]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"mob1": bson.M{"$regex": keys[0], "$options": "i"}},
			{"iqama_no": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	ParseTextSearch(r, &criterias, "search[name]", "name")
	ParseTextSearch(r, &criterias, "search[iqama_no]", "iqama_no")
	ParseTextSearch(r, &criterias, "search[mob1]", "mob1")

	if err = ParseFloatWithOperatorFilter(r, &criterias, "search[salary]", "salary"); err != nil {
		return employees, criterias, err
	}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return employees, criterias, err
	}
	if err = ParseDateRangeFilter(r, &criterias, "search[joining_date_from]", "search[joining_date_to]", "joining_date", timeZoneOffset); err != nil {
		return employees, criterias, err
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
	}
	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return employees, criterias, errors.New("Error fetching employees:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		if err := cur.Err(); err != nil {
			return employees, criterias, errors.New("Cursor error:" + err.Error())
		}
		emp := Employee{}
		if err := cur.Decode(&emp); err != nil {
			return employees, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		employees = append(employees, emp)
	}

	// Refresh each employee's liability account balance live from the ledger,
	// so list views stay in sync with the single-employee view (which always
	// recalculates). The employee document only stores a cached snapshot that
	// can go stale after payment edits/deletes. Skip this when a restrictive
	// projection was requested that doesn't need the account (e.g. dropdowns).
	needsAccount := criterias.Select == nil
	if !needsAccount {
		if v, ok := criterias.Select["account"]; ok && v == 1 {
			needsAccount = true
		}
	}
	if needsAccount {
		for i := range employees {
			if employees[i].ID.IsZero() {
				continue
			}
			account, err := employees[i].GetOrCreateLiabilityAccount(store)
			if err != nil || account == nil {
				continue
			}
			if err := account.CalculateBalance(nil, nil); err != nil {
				continue
			}
			employees[i].Account = account
		}
	}

	return employees, criterias, nil
}

func (employee *Employee) Delete(tokenClaims TokenClaims) error {
	collection := db.GetDB("store_" + employee.StoreID.Hex()).Collection("employee")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	now := time.Now()
	employee.Deleted = true
	employee.DeletedBy = &userID
	employee.DeletedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": employee.ID},
		bson.M{"$set": employee},
		options.Update().SetUpsert(false),
	)
	return err
}

// HardDelete permanently removes this employee and every trace of them from the
// accounting records: every salary/advance payment (and its ledger/postings),
// every automatic salary-due accrual entry and the opening-balance entry (and
// their ledgers/postings), the salary-due idempotency records, the employee's
// own liability account, and finally the employee document itself. Every OTHER
// account that was on the other side of one of these journals (Cash, Bank,
// SALARY EXPENSE, etc.) has its balance and posting running-balance chain
// rebuilt afterward, since the employee's own account/postings are gone and
// can no longer be relied on to self-correct via the normal update/undo flow.
func (employee *Employee) HardDelete() error {
	store, err := FindStoreByID(employee.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Accounts affected by the removed journals, other than the employee's own
	// (which is deleted outright below, so its balance doesn't need recalculating).
	otherAccounts := map[string]Account{}

	empAccount, err := employee.GetOrCreateLiabilityAccount(store)
	if err != nil {
		return err
	}

	collectOtherAccounts := func(accounts map[string]Account) {
		for id, acc := range accounts {
			if empAccount != nil && id == empAccount.ID.Hex() {
				continue
			}
			otherAccounts[id] = acc
		}
	}

	// 1. Every salary/advance payment: remove its ledger+postings, then the
	// payment record itself (including any already-soft-deleted ones).
	paymentCollection := db.GetDB("store_" + store.ID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cur, err := paymentCollection.Find(ctx, bson.M{"employee_id": employee.ID})
	if err != nil {
		return errors.New("error finding salary payments: " + err.Error())
	}
	var paymentIDs []primitive.ObjectID
	for cur.Next(ctx) {
		var p EmployeeSalaryPayment
		if err := cur.Decode(&p); err != nil {
			cur.Close(ctx)
			return errors.New("error decoding salary payment: " + err.Error())
		}
		paymentIDs = append(paymentIDs, p.ID)
	}
	cur.Close(ctx)

	for _, paymentID := range paymentIDs {
		ledger, err := store.FindLedgerByReferenceID(paymentID, store.ID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return err
		}
		if ledger != nil {
			related, err := ledger.GetRelatedAccounts()
			if err != nil {
				return err
			}
			collectOtherAccounts(related)
		}
		if err := store.RemoveLedgerByReferenceID(paymentID); err != nil {
			return err
		}
		if err := store.RemovePostingsByReferenceID(paymentID); err != nil {
			return err
		}
	}
	if _, err := paymentCollection.DeleteMany(ctx, bson.M{"employee_id": employee.ID}); err != nil {
		return errors.New("error deleting salary payments: " + err.Error())
	}

	// 2. Salary-due accrual entries + the opening-balance entry: both are
	// ledgered with ReferenceID = employee.ID (different ReferenceModels), so
	// a single reference-ID-based lookup/removal covers all of them at once.
	empLedgers, err := store.FindLedgersByReferenceID(employee.ID, store.ID, bson.M{})
	if err != nil {
		return err
	}
	for _, ledger := range empLedgers {
		related, err := ledger.GetRelatedAccounts()
		if err != nil {
			return err
		}
		collectOtherAccounts(related)
	}
	if err := store.RemoveLedgerByReferenceID(employee.ID); err != nil {
		return err
	}
	if err := store.RemovePostingsByReferenceID(employee.ID); err != nil {
		return err
	}

	// 3. Salary-due idempotency records (not ledgered, just bookkeeping for the
	// accrual cron/recalculation to avoid double-posting).
	dueCollection := db.GetDB("store_" + store.ID.Hex()).Collection("salary_due")
	if _, err := dueCollection.DeleteMany(ctx, bson.M{"employee_id": employee.ID}); err != nil {
		return errors.New("error deleting salary due records: " + err.Error())
	}

	// 4. The employee's own liability account.
	if empAccount != nil {
		accountCollection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
		if _, err := accountCollection.DeleteOne(ctx, bson.M{"_id": empAccount.ID}); err != nil {
			return errors.New("error deleting employee account: " + err.Error())
		}
	}

	// 5. The employee document itself.
	employeeCollection := db.GetDB("store_" + store.ID.Hex()).Collection("employee")
	if _, err := employeeCollection.DeleteOne(ctx, bson.M{"_id": employee.ID}); err != nil {
		return errors.New("error deleting employee: " + err.Error())
	}

	// 6. Rebuild every other account touched by the removed journals: its
	// current balance and the chronological running-balance chain of its
	// remaining postings.
	for _, account := range otherAccounts {
		if err := SetAccountBalances(map[string]Account{account.ID.Hex(): account}); err != nil {
			return err
		}
		if err := RebuildAccountPostingBalances(store, account.ID); err != nil {
			return err
		}
	}

	return nil
}

// ──────────────────────────────────────────────────────────
// Employee Accounting
// ──────────────────────────────────────────────────────────

// employeeAccountName returns the canonical liability account name for this employee.
func (employee *Employee) employeeAccountName() string {
	return "EMP: " + strings.ToUpper(strings.TrimSpace(employee.Name))
}

// GetOrCreateLiabilityAccount returns (or creates) the liability ledger account for this employee.
func (employee *Employee) GetOrCreateLiabilityAccount(store *Store) (*Account, error) {
	referenceModel := "employee"

	account, err := store.FindAccountByReferenceID(employee.ID, store.ID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	if account == nil {
		account, err = store.CreateAccountIfNotExists(
			employee.StoreID,
			&employee.ID,
			&referenceModel,
			employee.employeeAccountName(),
			nil,
			nil,
		)
		if err != nil {
			return nil, err
		}
	}

	expectedName := employee.employeeAccountName()
	if account != nil && account.Name != expectedName {
		account.Name = expectedName
	}
	if account != nil && account.ReferenceModel == nil {
		account.ReferenceModel = &referenceModel
	}
	if account != nil && account.ReferenceID == nil {
		account.ReferenceID = &employee.ID
	}
	if account != nil && account.Type == "" {
		account.Type = "liability"
	}
	if account != nil {
		if err := account.Update(); err != nil {
			return nil, err
		}
	}

	return account, nil
}

// EffectiveAccrualStartDate returns the date from which automated salary-due
// accrual should start counting for this employee: the later of JoiningDate and
// OpeningBalanceDate. This ensures migrated employees (real JoiningDate from
// years ago, but an OpeningBalanceDate set to their cutover day into this system)
// never get retroactive accrual entries for periods already settled under their
// old payroll system — that history is captured by the one-time opening balance
// instead. New hires with no OpeningBalanceDate simply start from JoiningDate.
//
// OpeningBalanceDate always acts as this cutover whenever it is set, regardless
// of whether OpeningBalance itself is zero or non-zero — the date represents
// "when this employee's records started being tracked in this system", and
// automated accrual must never reach back before it.
func (employee *Employee) EffectiveAccrualStartDate() *time.Time {
	var start *time.Time
	if employee.JoiningDate != nil {
		start = employee.JoiningDate
	}
	if employee.OpeningBalanceDate != nil {
		if start == nil || employee.OpeningBalanceDate.After(*start) {
			start = employee.OpeningBalanceDate
		}
	}
	return start
}

// ProratedSalaryForMonth returns the salary amount owed for the given month/year,
// prorating the employee's monthly salary by days worked if their effective
// accrual start date (see EffectiveAccrualStartDate) falls partway through that
// month. Returns the full monthly salary for periods entirely after that date,
// and 0 for periods before it. Employees with no JoiningDate/OpeningBalanceDate
// set (legacy records) always get the full salary.
func (employee *Employee) ProratedSalaryForMonth(month, year int) float64 {
	startDate := employee.EffectiveAccrualStartDate()
	if startDate == nil {
		return employee.Salary
	}

	periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0) // exclusive: first day of next month

	sd := *startDate
	joinDate := time.Date(sd.Year(), sd.Month(), sd.Day(), 0, 0, 0, 0, time.UTC)

	if !joinDate.Before(periodEnd) {
		// Accrual starts on/after the end of this period — nothing owed yet.
		return 0
	}
	if joinDate.Before(periodStart) {
		// Accrual already started before this period — full month's salary.
		return employee.Salary
	}

	// Accrual starts partway through this period: prorate by calendar days.
	daysInMonth := periodEnd.AddDate(0, 0, -1).Day()
	daysWorked := daysInMonth - joinDate.Day() + 1
	if daysWorked <= 0 {
		return 0
	}
	if daysWorked >= daysInMonth {
		return employee.Salary
	}

	dailyRate := employee.Salary / float64(daysInMonth)
	return RoundFloat(dailyRate*float64(daysWorked), 2)
}

// GetOutstandingSalaryAmount returns the amount to be credited to the employee's
// account by the automatic due-date accrual entry: always the employee's FULL
// monthly Salary (exactly as saved on the employee record) for any period that
// is eligible for accrual (see isSalaryDuePeriodValid) — never prorated by days
// worked and never reduced by payments (advance or otherwise) already made this
// period. Payments always debit the employee's account directly (see
// EmployeeSalaryPayment.CreateLedger), so the accrual credit and any
// earlier/later payment debits net together automatically to produce the
// correct running balance — the accrual amount itself is never changed.
func (employee *Employee) GetOutstandingSalaryAmount(store *Store, month, year int) (float64, error) {
	if !employee.isSalaryDuePeriodValid(month, year) {
		return 0, nil
	}

	return RoundFloat(employee.Salary, 2), nil
}

// IsSalaryDueRecorded checks whether a salary accrual entry already exists for the given month/year.
func (store *Store) IsSalaryDueRecorded(employeeID *primitive.ObjectID, month, year int) (bool, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salary_due")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, bson.M{
		"employee_id": employeeID,
		"month":       month,
		"year":        year,
	})
	return count > 0, err
}

// RecordSalaryDue inserts a SalaryDueEntry for idempotency tracking.
func (store *Store) RecordSalaryDue(employeeID *primitive.ObjectID, month, year int, amount float64) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salary_due")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	entry := SalaryDueEntry{
		ID:         primitive.NewObjectID(),
		EmployeeID: employeeID,
		StoreID:    &store.ID,
		Month:      month,
		Year:       year,
		Amount:     amount,
		CreatedAt:  &now,
	}
	_, err := collection.InsertOne(ctx, &entry)
	return err
}

// DoSalaryDueAccounting creates the accrual ledger entry for an unpaid salary period.
// DEBIT  SALARY EXPENSE (real, permanent P&L expense)
// CREDIT EMP: NAME       (liability — increases what the store owes the employee)
func (employee *Employee) DoSalaryDueAccounting(store *Store, month, year int) error {
	// Never post an automatic accrual entry for a period whose salary due date
	// (derived from SalaryDay) falls before the migration cutover date
	// (OpeningBalanceDate/"As Of Date"). That period is assumed to already be
	// settled under the old payroll system — the one-time Opening Balance entry
	// captures whatever was owed as of that date instead. This matters most for
	// the transition month itself: e.g. SalaryDay=1 but OpeningBalanceDate set
	// to the 15th means this month's computed due date (the 1st) is earlier
	// than the cutover, so it must be skipped rather than partially accrued.
	if !employee.isSalaryDuePeriodValid(month, year) {
		return nil
	}

	outstandingAmount, err := employee.GetOutstandingSalaryAmount(store, month, year)
	if err != nil {
		return err
	}
	if outstandingAmount <= 0 {
		return nil
	}

	already, err := store.IsSalaryDueRecorded(&employee.ID, month, year)
	if err != nil {
		return err
	}
	if already {
		// Check whether the ledger actually exists (two-phase idempotency).
		// If not, recreate it.
		ledgerExists, _ := store.HasSalaryDueLedger(&employee.ID, month, year)
		if ledgerExists {
			return nil
		}
	}

	// Record idempotency entry first.
	if !already {
		if err := store.RecordSalaryDue(&employee.ID, month, year, outstandingAmount); err != nil {
			return errors.New("error recording salary due entry: " + err.Error())
		}
	}

	ledger, err := employee.CreateSalaryDueLedger(store, month, year, outstandingAmount)
	if err != nil {
		return err
	}

	if _, err := ledger.CreatePostings(); err != nil {
		return err
	}

	// The accrual is dated to the salary due day, which can be earlier than
	// other postings already on the affected accounts (e.g. an advance
	// payment made earlier in the same month, dated after this accrual's
	// due date, or another employee's accrual/payment sharing the
	// company-wide SALARY EXPENSE account).
	// Rebuild the running balance chain in true chronological order on every
	// account this ledger touched so all balance sheets read correctly
	// regardless of insertion order.
	relatedAccounts, err := ledger.GetRelatedAccounts()
	if err != nil {
		return err
	}
	for _, account := range relatedAccounts {
		if err := RebuildAccountPostingBalances(store, account.ID); err != nil {
			return err
		}
	}
	return nil
}

// HasSalaryDueLedger checks whether a salary accrual ledger entry exists for this employee/month/year.
func (store *Store) HasSalaryDueLedger(employeeID *primitive.ObjectID, month, year int) (bool, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, bson.M{
		"reference_id":    employeeID,
		"reference_model": "salary_due",
		"store_id":        store.ID,
		"journals.0":      bson.M{"$exists": true},
		// Encode month/year in the reference_code field for lookup.
		"reference_code": buildSalaryDueCode(month, year),
	})
	return count > 0, err
}

// ClearSalaryDueForPeriod removes any previously-posted automatic salary-due
// accrual ledger, its postings, and its idempotency record for one employee/
// month/year, so DoSalaryDueAccounting can safely recreate it (e.g. after the
// employee's Salary or SalaryDay changes and the old figures are stale).
func (store *Store) ClearSalaryDueForPeriod(employeeID *primitive.ObjectID, month, year int) error {
	code := buildSalaryDueCode(month, year)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"store_id":        store.ID,
		"reference_id":    employeeID,
		"reference_model": "salary_due",
		"reference_code":  code,
	}

	// Find the accounts this ledger touched (EMP:NAME, SALARY EXPENSE)
	// BEFORE deleting it, so they can be rebuilt afterward — once the
	// ledger is gone, GetRelatedAccounts has nothing left to look up.
	ledgerCollection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	relatedAccounts := map[string]Account{}
	var existingLedger Ledger
	if err := ledgerCollection.FindOne(ctx, filter).Decode(&existingLedger); err == nil {
		relatedAccounts, err = existingLedger.GetRelatedAccounts()
		if err != nil {
			return err
		}
	} else if err != mongo.ErrNoDocuments {
		return errors.New("error finding stale salary due ledger: " + err.Error())
	}

	ledgerDeleteResult, err := ledgerCollection.DeleteMany(ctx, filter)
	if err != nil {
		return errors.New("error clearing stale salary due ledger: " + err.Error())
	}

	postingCollection := db.GetDB("store_" + store.ID.Hex()).Collection("posting")
	postingDeleteResult, err := postingCollection.DeleteMany(ctx, filter)
	if err != nil {
		return errors.New("error clearing stale salary due postings: " + err.Error())
	}

	dueCollection := db.GetDB("store_" + store.ID.Hex()).Collection("salary_due")
	if _, err := dueCollection.DeleteMany(ctx, bson.M{
		"store_id":    store.ID,
		"employee_id": employeeID,
		"month":       month,
		"year":        year,
	}); err != nil {
		return errors.New("error clearing stale salary due record: " + err.Error())
	}

	// Only rebuild if something was actually removed — removing a posting
	// leaves the balances cached on the account's OTHER, still-existing
	// postings stale (they were computed including this now-deleted entry's
	// amount), so the running balance chain needs recomputing.
	if ledgerDeleteResult.DeletedCount > 0 || postingDeleteResult.DeletedCount > 0 {
		if err := SetAccountBalances(relatedAccounts); err != nil {
			return err
		}
		for _, account := range relatedAccounts {
			if err := RebuildAccountPostingBalances(store, account.ID); err != nil {
				return err
			}
		}

		// Fall back to the employee's own account too, in case the deleted
		// ledger predated this function's account tracking (e.g. very old
		// data) and GetRelatedAccounts came back empty.
		emp, err := store.FindEmployeeByID(employeeID, bson.M{})
		if err != nil {
			return errors.New("error finding employee for balance rebuild: " + err.Error())
		}
		if emp != nil {
			empAccount, err := emp.GetOrCreateLiabilityAccount(store)
			if err != nil {
				return err
			}
			if err := RebuildAccountPostingBalances(store, empAccount.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// removeInvalidSalaryDueEntries scans EVERY previously-posted automatic
// salary-due accrual entry for this employee (across all months/years, not
// just the ones RecalculateSalaryDueForEmployee's loop would touch) and
// removes any that are no longer valid under the employee's current
// Salary/SalaryDay/JoiningDate/OpeningBalanceDate. This matters in
// particular when OpeningBalanceDate (or JoiningDate) is pushed LATER on an
// edit: periods between the old and new effective start date may already
// have an accrual entry posted from before the edit, which is now "wrong"
// (it predates the migration cutover) and must be cleared rather than left
// stranded in the ledger/balance sheet.
func (employee *Employee) removeInvalidSalaryDueEntries(store *Store) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := collection.Find(ctx, bson.M{
		"store_id":        store.ID,
		"reference_id":    employee.ID,
		"reference_model": "salary_due",
	})
	if err != nil {
		return errors.New("error finding salary due ledgers: " + err.Error())
	}
	defer cur.Close(ctx)

	type periodKey struct {
		month int
		year  int
	}
	var invalidPeriods []periodKey

	for cur.Next(ctx) {
		var ledger Ledger
		if err := cur.Decode(&ledger); err != nil {
			continue
		}

		month, year, ok := parseSalaryDueCode(ledger.ReferenceCode)
		if !ok {
			continue
		}

		if !employee.isSalaryDuePeriodValid(month, year) {
			invalidPeriods = append(invalidPeriods, periodKey{month: month, year: year})
		}
	}

	for _, p := range invalidPeriods {
		if err := store.ClearSalaryDueForPeriod(&employee.ID, p.month, p.year); err != nil {
			return err
		}
	}

	return nil
}

// isSalaryDuePeriodValid reports whether an automatic accrual entry is
// allowed to exist for the given month/year under the employee's current
// settings: the period's computed due date must not fall before
// OpeningBalanceDate (migration cutover — always enforced whenever
// OpeningBalanceDate is set, regardless of the OpeningBalance amount), and
// the period must not be entirely before the employee's effective accrual
// start date.
func (employee *Employee) isSalaryDuePeriodValid(month, year int) bool {
	if employee.OpeningBalanceDate != nil {
		ob := *employee.OpeningBalanceDate
		obDate := time.Date(ob.Year(), ob.Month(), ob.Day(), 0, 0, 0, 0, time.UTC)
		dueDate := time.Date(year, time.Month(month), employee.SalaryDay, 0, 0, 0, 0, time.UTC)
		if dueDate.Before(obDate) {
			return false
		}
	}

	return employee.ProratedSalaryForMonth(month, year) > 0
}

// parseSalaryDueCode reverses buildSalaryDueCode ("SALARY-{year}-{month}"),
// returning ok=false if the code isn't in the expected format.
func parseSalaryDueCode(code string) (month, year int, ok bool) {
	parts := strings.Split(code, "-")
	if len(parts) != 3 || parts[0] != "SALARY" {
		return 0, 0, false
	}
	y, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	m, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, false
	}
	return m, y, true
}

// RecalculateSalaryDueForEmployee clears every previously-posted automatic
// salary-due accrual entry for this employee and immediately regenerates them
// (from EffectiveAccrualStartDate through the current month) using the
// employee's up-to-date Salary/SalaryDay. Call this whenever Salary or
// SalaryDay is edited, so the corrected accrual is visible right away instead
// of waiting for the nightly cron (ProcessSalaryDueDatesForAllStores) to
// eventually recompute it — useful both for correctness and for same-day
// testing after an edit.
func (employee *Employee) RecalculateSalaryDueForEmployee(store *Store) error {
	// First, sweep away any existing accrual entries that are now invalid
	// (e.g. OpeningBalanceDate/JoiningDate moved later than a period that
	// already has an entry posted from before this edit).
	if err := employee.removeInvalidSalaryDueEntries(store); err != nil {
		return err
	}

	startDate := employee.EffectiveAccrualStartDate()
	if startDate == nil {
		startDate = employee.CreatedAt
	}
	if startDate == nil {
		return nil
	}

	now := time.Now()
	tzOffset := CountryTimezoneOffset(store.CountryCode)
	// Convert the current UTC instant to the store's local wall-clock date so
	// "today" (used to gate the current month/salary-day check below) reflects
	// the store's own timezone rather than the server's. tzOffset is negative
	// for east-of-UTC countries (e.g. -3 for Saudi Arabia/UTC+3), so local =
	// UTC - tzOffset (i.e. UTC + 3h for KSA) — matching the same UTC→local
	// pattern used elsewhere (e.g. models/sales.go, controller/mcp_bi.go).
	localNow := now.Add(-time.Duration(tzOffset * float64(time.Hour)))

	for y := startDate.Year(); y <= localNow.Year(); y++ {
		startMonth := 1
		endMonth := 12
		if y == startDate.Year() {
			startMonth = int(startDate.Month())
		}
		if y == localNow.Year() {
			endMonth = int(localNow.Month())
		}

		for m := startMonth; m <= endMonth; m++ {
			// Always clear the stale entry first — the old figures (based on
			// the previous Salary/SalaryDay) are no longer valid regardless
			// of whether this period is still due.
			if err := store.ClearSalaryDueForPeriod(&employee.ID, m, y); err != nil {
				return err
			}

			// Skip recreating the current month's entry if the (possibly
			// updated) salary day hasn't arrived yet this month.
			if y == localNow.Year() && m == int(localNow.Month()) && localNow.Day() < employee.SalaryDay {
				continue
			}

			if err := employee.DoSalaryDueAccounting(store, m, y); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildSalaryDueCode(month, year int) string {
	return "SALARY-" + strconv.Itoa(year) + "-" + strconv.Itoa(month)
}

// CreateSalaryDueLedger builds the accrual double-entry journal:
//
//	DR SALARY EXPENSE  /  CR EMP: NAME
//
// Recognizes the cost once (SALARY EXPENSE is a true P&L expense that is
// never reduced later, same as any other accrued expense) and increases
// what the store owes this specific employee. There is no separate
// "Pending Salary" account — the employee's own liability account (EMP:
// NAME) already IS the live "currently owed to this employee" figure, and
// the Dashboard's Salary Balance sums these across all employees for the
// company-wide total.
//
// Always posts the employee's FULL monthly Salary for the period (see
// GetOutstandingSalaryAmount), regardless of any payment (advance or
// regular) already made this period. Every salary payment always debits
// the employee's own account directly (see EmployeeSalaryPayment.CreateLedger),
// so that debit nets against this credit automatically to produce the
// correct final running balance.
func (employee *Employee) CreateSalaryDueLedger(store *Store, month, year int, amount float64) (*Ledger, error) {
	salaryExpenseAccount, err := store.CreateAccountIfNotExists(employee.StoreID, nil, nil, "SALARY EXPENSE", nil, nil)
	if err != nil {
		return nil, errors.New("error creating SALARY EXPENSE account: " + err.Error())
	}

	empAccount, err := employee.GetOrCreateLiabilityAccount(store)
	if err != nil {
		return nil, errors.New("error creating employee account: " + err.Error())
	}

	now := time.Now()
	// Date the accrual to the FIRST MINUTE of the salary day in the store's own
	// local timezone (not the server's UTC clock) — construct local midnight,
	// then convert to its UTC equivalent for storage, matching the same
	// local-midnight→UTC pattern used elsewhere (e.g. models/bi_monthly_pl.go).
	// Without this conversion, a naive UTC-midnight timestamp displays several
	// hours into the day for any store east of UTC (e.g. 3am for Saudi Arabia).
	localMidnight := time.Date(year, time.Month(month), employee.SalaryDay, 0, 0, 0, 0, time.UTC)
	tzOffset := CountryTimezoneOffset(store.CountryCode)
	dueDate := ConvertTimeZoneToUTC(tzOffset, localMidnight)

	refModel := "salary_due"
	refCode := buildSalaryDueCode(month, year)
	groupID := primitive.NewObjectID()

	journals := []Journal{
		{
			Date:          &dueDate,
			AccountID:     salaryExpenseAccount.ID,
			AccountName:   salaryExpenseAccount.Name,
			AccountNumber: salaryExpenseAccount.Number,
			DebitOrCredit: "debit",
			Debit:         amount,
			GroupID:       groupID,
		},
		{
			Date:          &dueDate,
			AccountID:     empAccount.ID,
			AccountName:   empAccount.Name,
			AccountNumber: empAccount.Number,
			DebitOrCredit: "credit",
			Credit:        amount,
			GroupID:       groupID,
		},
	}

	ledger := &Ledger{
		StoreID:        employee.StoreID,
		ReferenceID:    employee.ID,
		ReferenceModel: refModel,
		ReferenceCode:  refCode,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if err := ledger.Insert(); err != nil {
		return nil, errors.New("error inserting salary due ledger: " + err.Error())
	}

	// Keep employee account up to date.
	empAccount.CalculateBalance(nil, nil)
	empAccount.Update()
	employee.Account = empAccount
	employee.Update()

	return ledger, nil
}

// ──────────────────────────────────────────────────────────
// Employee Opening Balance (migration cutover) Accounting
// ──────────────────────────────────────────────────────────

const employeeOpeningBalanceReferenceModel = "employee_opening_balance"
const employeeOpeningBalanceReferenceCode = "OPENING-BALANCE"

// HasOpeningBalanceLedger checks whether the one-time opening-balance ledger
// entry has already been posted for this employee.
func (store *Store) HasOpeningBalanceLedger(employeeID *primitive.ObjectID) (bool, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, bson.M{
		"reference_id":    employeeID,
		"reference_model": employeeOpeningBalanceReferenceModel,
		"store_id":        store.ID,
		"journals.0":      bson.M{"$exists": true},
	})
	return count > 0, err
}

// RemoveOpeningBalanceLedgerAndPostings deletes the opening-balance ledger entry
// (and its postings) for this employee, scoped strictly by reference_model so it
// never touches the employee's other ledger entries (e.g. salary_due accruals)
// that happen to share the same reference_id.
func (store *Store) RemoveOpeningBalanceLedgerAndPostings(employeeID *primitive.ObjectID) error {
	ctx := context.Background()

	ledgerCollection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	if _, err := ledgerCollection.DeleteMany(ctx, bson.M{
		"reference_id":    employeeID,
		"reference_model": employeeOpeningBalanceReferenceModel,
	}); err != nil {
		return err
	}

	postingCollection := db.GetDB("store_" + store.ID.Hex()).Collection("posting")
	_, err := postingCollection.DeleteMany(ctx, bson.M{
		"reference_id":    employeeID,
		"reference_model": employeeOpeningBalanceReferenceModel,
	})
	return err
}

// CreateOpeningBalanceLedger posts the one-time migration cutover entry:
// DR OPENING BALANCE EQUITY  /  CR EMP: NAME
// This represents what the store already owed the employee (under a previous,
// external payroll system) as of the opening balance date.
func (employee *Employee) CreateOpeningBalanceLedger(store *Store, amount float64, date *time.Time) (*Ledger, error) {
	openingBalanceAccount, err := store.CreateAccountIfNotExists(employee.StoreID, nil, nil, "OPENING BALANCE EQUITY", nil, nil)
	if err != nil {
		return nil, errors.New("error creating OPENING BALANCE EQUITY account: " + err.Error())
	}

	empAccount, err := employee.GetOrCreateLiabilityAccount(store)
	if err != nil {
		return nil, errors.New("error creating employee account: " + err.Error())
	}

	now := time.Now()
	entryDate := date
	if entryDate == nil {
		entryDate = &now
	}

	groupID := primitive.NewObjectID()
	journals := []Journal{
		{
			Date:          entryDate,
			AccountID:     openingBalanceAccount.ID,
			AccountName:   openingBalanceAccount.Name,
			AccountNumber: openingBalanceAccount.Number,
			DebitOrCredit: "debit",
			Debit:         amount,
			GroupID:       groupID,
		},
		{
			Date:          entryDate,
			AccountID:     empAccount.ID,
			AccountName:   empAccount.Name,
			AccountNumber: empAccount.Number,
			DebitOrCredit: "credit",
			Credit:        amount,
			GroupID:       groupID,
		},
	}

	ledger := &Ledger{
		StoreID:        employee.StoreID,
		ReferenceID:    employee.ID,
		ReferenceModel: employeeOpeningBalanceReferenceModel,
		ReferenceCode:  employeeOpeningBalanceReferenceCode,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if err := ledger.Insert(); err != nil {
		return nil, errors.New("error inserting opening balance ledger: " + err.Error())
	}

	empAccount.CalculateBalance(nil, nil)
	empAccount.Update()
	employee.Account = empAccount

	return ledger, nil
}

// PostOpeningBalanceIfNeeded (re-)posts the one-time opening-balance ledger entry
// so it always matches the employee's current OpeningBalance/OpeningBalanceDate.
// Safe to call on every create/update: it removes any previously posted entry
// first (idempotent — no duplicate postings), then re-creates it if the current
// OpeningBalance is non-zero, or leaves it removed if the balance was cleared to
// zero. Updates employee.OpeningBalancePosted and employee.Account accordingly;
// the caller is responsible for persisting the employee record afterward.
func (employee *Employee) PostOpeningBalanceIfNeeded(store *Store) error {
	if err := store.RemoveOpeningBalanceLedgerAndPostings(&employee.ID); err != nil {
		return errors.New("error removing existing opening balance ledger: " + err.Error())
	}

	if employee.OpeningBalance == 0 {
		employee.OpeningBalancePosted = false
		if empAccount, err := employee.GetOrCreateLiabilityAccount(store); err == nil && empAccount != nil {
			empAccount.CalculateBalance(nil, nil)
			empAccount.Update()
			employee.Account = empAccount
		}
		return nil
	}

	ledger, err := employee.CreateOpeningBalanceLedger(store, employee.OpeningBalance, employee.OpeningBalanceDate)
	if err != nil {
		return err
	}
	if _, err := ledger.CreatePostings(); err != nil {
		return errors.New("error posting opening balance ledger: " + err.Error())
	}

	employee.OpeningBalancePosted = true
	return nil
}

// ──────────────────────────────────────────────────────────
// EmployeeSalaryPayment Accounting
// ──────────────────────────────────────────────────────────

func (payment *EmployeeSalaryPayment) UpdateForeignLabelFields() error {
	if payment.CreatedBy != nil {
		u, err := FindUserByID(payment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		payment.CreatedByName = u.Name
	}

	if payment.UpdatedBy != nil {
		u, err := FindUserByID(payment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		payment.UpdatedByName = u.Name
	}

	if payment.EmployeeID != nil && !payment.EmployeeID.IsZero() {
		store, err := FindStoreByID(payment.StoreID, bson.M{})
		if err != nil {
			return err
		}
		emp, err := store.FindEmployeeByID(payment.EmployeeID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		payment.EmployeeName = emp.Name
	}

	return nil
}

func (payment *EmployeeSalaryPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	if payment.StoreID == nil || payment.StoreID.IsZero() {
		errs["store_id"] = "Store ID is required"
	}
	if payment.EmployeeID == nil || payment.EmployeeID.IsZero() {
		errs["employee_id"] = "Employee is required"
	}
	if payment.Amount <= 0 {
		errs["amount"] = "Amount must be greater than 0"
	}
	if payment.PaymentMethod == "" {
		errs["payment_method"] = "Payment method is required"
	}
	if payment.Month < 1 || payment.Month > 12 {
		errs["month"] = "Month must be between 1 and 12"
	}
	if payment.Year < 2000 {
		errs["year"] = "Invalid year"
	}

	// Note: no cap is enforced against the employee's outstanding/accrued salary
	// balance here. Salary advances (paying an employee ahead of what they've
	// accrued so far) are a legitimate, common use case, so any positive amount
	// is accepted — the resulting (possibly negative/"employee owes store")
	// balance is simply reflected on the employee's ledger/balance sheet.

	return errs
}

func (payment *EmployeeSalaryPayment) Insert() error {
	collection := db.GetDB("store_" + payment.StoreID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payment.ID = primitive.NewObjectID()
	payment.ensureCode()
	_, err := collection.InsertOne(ctx, &payment)
	return err
}

// ensureCode backfills Code for legacy payments (created before Code existed)
// as well as new ones, using a short, always-unique code derived from the
// payment's own ObjectID. This is what the ledger's ReferenceCode is set to,
// so the Employee Balance Sheet's "ID" column has something to display/link to.
func (payment *EmployeeSalaryPayment) ensureCode() {
	if payment.Code != "" || payment.ID.IsZero() {
		return
	}
	hex := payment.ID.Hex()
	payment.Code = "SAL-" + strings.ToUpper(hex[len(hex)-6:])
}

func (payment *EmployeeSalaryPayment) Update() error {
	collection := db.GetDB("store_" + payment.StoreID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Backfill Code on legacy payments so editing an older payment (that never
	// got a code) also gets one, and the Employee Balance Sheet's ID column
	// starts showing a link for it going forward.
	payment.ensureCode()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": payment.ID},
		bson.M{"$set": payment},
		options.Update().SetUpsert(false),
	)
	return err
}

func (payment *EmployeeSalaryPayment) Delete(tokenClaims TokenClaims) error {
	collection := db.GetDB("store_" + payment.StoreID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	now := time.Now()
	payment.Deleted = true
	payment.DeletedBy = &userID
	payment.DeletedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": payment.ID},
		bson.M{"$set": payment},
		options.Update().SetUpsert(false),
	)
	return err
}

func (store *Store) FindEmployeeSalaryPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (payment *EmployeeSalaryPayment, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx, bson.M{
		"_id":      ID,
		"store_id": store.ID,
	}, findOneOptions).Decode(&payment)
	return payment, err
}

func (store *Store) SearchEmployeeSalaryPayment(w http.ResponseWriter, r *http.Request) (payments []EmployeeSalaryPayment, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var keys []string
	var ok bool

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return payments, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[employee_id]"]
	if ok && len(keys[0]) >= 1 {
		empID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return payments, criterias, err
		}
		criterias.SearchBy["employee_id"] = empID
	}

	keys, ok = r.URL.Query()["search[month]"]
	if ok && len(keys[0]) >= 1 {
		m, _ := strconv.Atoi(keys[0])
		criterias.SearchBy["month"] = m
	}

	keys, ok = r.URL.Query()["search[year]"]
	if ok && len(keys[0]) >= 1 {
		y, _ := strconv.Atoi(keys[0])
		criterias.SearchBy["year"] = y
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee_salary_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
	}
	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return payments, criterias, errors.New("Error fetching salary payments:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		if err := cur.Err(); err != nil {
			return payments, criterias, errors.New("Cursor error:" + err.Error())
		}
		p := EmployeeSalaryPayment{}
		if err := cur.Decode(&p); err != nil {
			return payments, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		payments = append(payments, p)
	}

	return payments, criterias, nil
}

// DoAccounting creates the payment ledger (see CreateLedger: DR EMP: NAME /
// CR CASH-or-BANK), then rebuilds the running-balance chain on every account
// it touched — CASH/BANK are shared across every employee (and every other
// transaction in the store), so a backdated payment could otherwise leave
// stale out-of-order balances on those accounts.
func (payment *EmployeeSalaryPayment) DoAccounting() error {
	ledger, err := payment.CreateLedger()
	if err != nil {
		return err
	}

	if _, err := ledger.CreatePostings(); err != nil {
		return err
	}

	store, err := FindStoreByID(payment.StoreID, bson.M{})
	if err != nil {
		return err
	}
	relatedAccounts, err := ledger.GetRelatedAccounts()
	if err != nil {
		return err
	}
	for _, account := range relatedAccounts {
		if err := RebuildAccountPostingBalances(store, account.ID); err != nil {
			return err
		}
	}
	return nil
}

// UndoAccounting deletes all ledger/posting records for this payment, recalculates
// each affected account's current balance, and rebuilds the chronological
// running-balance chain for the employee's own account (the payment's postings
// may not be the most recent ones on that account by date, so simply removing
// them can leave the remaining postings' per-row Balance values stale/out of
// order on the Employee Balance Sheet — RebuildAccountPostingBalances fixes that).
func (payment *EmployeeSalaryPayment) UndoAccounting() error {
	store, err := FindStoreByID(payment.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(payment.ID, *payment.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	ledgerAccounts := map[string]Account{}
	if ledger != nil {
		ledgerAccounts, err = ledger.GetRelatedAccounts()
		if err != nil {
			return err
		}
	}

	if err := store.RemoveLedgerByReferenceID(payment.ID); err != nil {
		return err
	}
	if err := store.RemovePostingsByReferenceID(payment.ID); err != nil {
		return err
	}

	if err := SetAccountBalances(ledgerAccounts); err != nil {
		return err
	}

	for _, account := range ledgerAccounts {
		if err := RebuildAccountPostingBalances(store, account.ID); err != nil {
			return err
		}
	}

	return nil
}

// CreateLedger builds the journal for a salary payment:
//
//	DR EMP: NAME  /  CR CASH or BANK
//
// Decreases what the store owes this specific employee and reflects the
// actual cash/bank outflow — regardless of whether it's an advance or a
// regular salary payment, and regardless of whether the due-date accrual
// for that period has been posted yet. The automatic accrual entry (see
// DoSalaryDueAccounting) always credits the FULL monthly salary to this
// same account, so payments made before or after that accrual simply net
// together in the employee's account balance to produce the correct
// running "amount owed" at all times.
func (payment *EmployeeSalaryPayment) CreateLedger() (*Ledger, error) {
	store, err := FindStoreByID(payment.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	emp, err := store.FindEmployeeByID(payment.EmployeeID, bson.M{})
	if err != nil {
		return nil, errors.New("employee not found: " + err.Error())
	}

	empAccount, err := emp.GetOrCreateLiabilityAccount(store)
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(payment.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}
	bankAccount, err := store.CreateAccountIfNotExists(payment.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	var payingAccount *Account
	if payment.PaymentMethod == "cash" {
		payingAccount = cashAccount
	} else if slices.Contains(BANK_PAYMENT_METHODS, payment.PaymentMethod) {
		payingAccount = bankAccount
	} else {
		return nil, errors.New("unsupported payment method: " + payment.PaymentMethod)
	}

	now := time.Now()
	date := payment.Date
	if date == nil {
		date = &now
	}

	groupID := primitive.NewObjectID()
	journals := []Journal{
		{
			Date:          date,
			AccountID:     empAccount.ID,
			AccountName:   empAccount.Name,
			AccountNumber: empAccount.Number,
			DebitOrCredit: "debit",
			Debit:         payment.Amount,
			GroupID:       groupID,
		},
		{
			Date:          date,
			AccountID:     payingAccount.ID,
			AccountName:   payingAccount.Name,
			AccountNumber: payingAccount.Number,
			DebitOrCredit: "credit",
			Credit:        payment.Amount,
			GroupID:       groupID,
		},
	}

	ledger := &Ledger{
		StoreID:        payment.StoreID,
		ReferenceID:    payment.ID,
		ReferenceModel: "employee_salary_payment",
		ReferenceCode:  payment.Code,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if err := ledger.Insert(); err != nil {
		return nil, errors.New("error inserting salary payment ledger: " + err.Error())
	}

	// Refresh account balance on employee record.
	empAccount.CalculateBalance(nil, nil)
	empAccount.Update()
	emp.Account = empAccount
	emp.Update()

	return ledger, nil
}

func (payment *EmployeeSalaryPayment) RegenerateSalaryDueIfNeeded() error {
	if payment.EmployeeID == nil || payment.EmployeeID.IsZero() {
		return nil
	}

	return regenerateSalaryDueForPeriod(payment.StoreID, payment.EmployeeID, payment.Month, payment.Year)
}

// RegenerateSalaryDueForEmployeePeriod is the exported entry point used by
// controllers to re-check/accrue an employee's salary for a specific month/year
// (e.g. after a salary payment that used to cover that period is edited/moved).
func RegenerateSalaryDueForEmployeePeriod(store *Store, employeeID *primitive.ObjectID, month, year int) error {
	return regenerateSalaryDueForPeriod(&store.ID, employeeID, month, year)
}

// regenerateSalaryDueForPeriod re-checks whether an employee's salary for a given
// month/year is still outstanding after their salary day, and (re-)creates the
// accrual ledger entry if so. Used both by RegenerateSalaryDueIfNeeded (payment
// delete/update) and by the background cron job.
func regenerateSalaryDueForPeriod(storeID, employeeID *primitive.ObjectID, month, year int) error {
	if employeeID == nil || employeeID.IsZero() || month < 1 || month > 12 || year < 2000 {
		return nil
	}

	store, err := FindStoreByID(storeID, bson.M{})
	if err != nil {
		return err
	}

	hasSalaryDueLedger, err := store.HasSalaryDueLedger(employeeID, month, year)
	if err != nil {
		return err
	}
	if hasSalaryDueLedger {
		return nil
	}

	employee, err := store.FindEmployeeByID(employeeID, bson.M{})
	if err != nil {
		return err
	}

	outstandingAmount, err := employee.GetOutstandingSalaryAmount(store, month, year)
	if err != nil {
		return err
	}
	if outstandingAmount <= 0 {
		return nil
	}

	// Compare in the store's own local time so the due date isn't off by
	// several hours relative to the server's UTC clock (same reasoning as
	// RecalculateSalaryDueForEmployee's current-month gate).
	tzOffset := CountryTimezoneOffset(store.CountryCode)
	localNow := time.Now().Add(-time.Duration(tzOffset * float64(time.Hour)))
	dueDate := time.Date(year, time.Month(month), employee.SalaryDay, 0, 0, 0, 0, time.UTC)
	if localNow.Before(dueDate) {
		return nil
	}

	return employee.DoSalaryDueAccounting(store, month, year)
}

// ──────────────────────────────────────────────────────────
// Background cron: auto-accrue unpaid salaries
// ──────────────────────────────────────────────────────────

// ProcessSalaryDueDatesForAllStores runs nightly and creates accrual entries
// for any employees whose salary day has passed in prior months without a record.
func ProcessSalaryDueDatesForAllStores() error {
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	now := time.Now()

	for _, store := range stores {
		if !store.Settings.EnableEmployeeModule {
			continue
		}

		if err := processSalaryDuesForStore(&store, now); err != nil {
			log.Printf("[salary-cron] store %s: %v", store.ID.Hex(), err)
		}
	}

	return nil
}

func processSalaryDuesForStore(store *Store, now time.Time) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee")
	ctx := context.Background()

	cur, err := collection.Find(ctx, bson.M{
		"deleted": bson.M{"$ne": true},
		"salary":  bson.M{"$gt": 0},
	}, options.Find().SetNoCursorTimeout(true))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	tzOffset := CountryTimezoneOffset(store.CountryCode)
	// See RecalculateSalaryDueForEmployee for why this subtracts (not adds)
	// tzOffset to go from the UTC instant to the store's local wall-clock date.
	localNow := now.Add(-time.Duration(tzOffset * float64(time.Hour)))

	for cur.Next(ctx) {
		emp := Employee{}
		if err := cur.Decode(&emp); err != nil {
			continue
		}

		// Check every month from the employee's effective accrual start date
		// (the later of JoiningDate and OpeningBalanceDate — see
		// EffectiveAccrualStartDate) up to now. For legacy records with neither
		// set, fall back to record creation date.
		startDate := emp.EffectiveAccrualStartDate()
		if startDate == nil {
			startDate = emp.CreatedAt
		}
		if startDate == nil {
			t := now.AddDate(-1, 0, 0)
			startDate = &t
		}

		for y := startDate.Year(); y <= localNow.Year(); y++ {
			startMonth := 1
			endMonth := 12
			if y == startDate.Year() {
				startMonth = int(startDate.Month())
			}
			if y == localNow.Year() {
				endMonth = int(localNow.Month())
			}

			for m := startMonth; m <= endMonth; m++ {
				// Skip current month if salary day hasn't arrived yet.
				if y == localNow.Year() && m == int(localNow.Month()) && localNow.Day() < emp.SalaryDay {
					continue
				}

				if err := emp.DoSalaryDueAccounting(store, m, y); err != nil {
					log.Printf("[salary-cron] emp %s month %d/%d: %v", emp.ID.Hex(), m, y, err)
				}
			}
		}
	}

	return nil
}
