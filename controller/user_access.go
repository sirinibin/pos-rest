package controller

import (
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func isAdminUser(u *models.User) bool {
	return u != nil && (u.Admin || u.Role == "Admin")
}

func shareAStore(a, b []*primitive.ObjectID) bool {
	for _, x := range a {
		for _, y := range b {
			if x != nil && y != nil && *x == *y {
				return true
			}
		}
	}
	return false
}

// manageUserError says why the requesting user may not change or delete the
// target user, or "" when they may. It is the rule ChangePassword and
// ToggleUserStatus already applied: admins manage anyone, everyone manages
// themselves, and a Manager manages Managers and SalesMen who share one of
// their stores. Update and delete used to let any signed-in user change or
// delete any user, admins included.
func manageUserError(requesting, target *models.User) string {
	if isAdminUser(requesting) || requesting.ID == target.ID {
		return ""
	}
	if requesting.Role != "Manager" {
		return "Only Admin or Manager can manage other users"
	}
	if target.Role != "Manager" && target.Role != "SalesMan" {
		return "Manager can only manage Manager or SalesMan users"
	}
	if !shareAStore(requesting.StoreIDs, target.StoreIDs) {
		return "Target user is not in your store"
	}
	return ""
}

// viewUserAllowed mirrors the user list: non-admins see themselves, users
// they created and users who share one of their stores.
func viewUserAllowed(requesting, target *models.User) bool {
	if isAdminUser(requesting) || requesting.ID == target.ID {
		return true
	}
	if target.CreatedBy != nil && *target.CreatedBy == requesting.ID {
		return true
	}
	return shareAStore(requesting.StoreIDs, target.StoreIDs)
}

// grantErrors checks what a non-admin puts on a user they create or edit:
// no admin flag, and only stores they can use themselves (plus, on an edit,
// stores the user already had). A user limited to some stores may not give
// out an empty store list, which means every store.
func grantErrors(requesting *models.User, admin bool, storeIDs, alreadyHad []*primitive.ObjectID) map[string]string {
	errs := map[string]string{}
	if isAdminUser(requesting) {
		return errs
	}
	if admin {
		errs["admin"] = "Only admins can grant admin access"
	}
	if len(requesting.StoreIDs) == 0 {
		return errs
	}
	if len(storeIDs) == 0 {
		errs["store_ids"] = "Choose at least one store"
		return errs
	}
	for _, s := range storeIDs {
		if s == nil {
			continue
		}
		one := []*primitive.ObjectID{s}
		if !shareAStore(one, requesting.StoreIDs) && !shareAStore(one, alreadyHad) {
			errs["store_ids"] = "You can only give access to your own stores"
			break
		}
	}
	return errs
}
