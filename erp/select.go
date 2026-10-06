package erp

import (
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// Field selection (?select=) for list and get, same syntax as the legacy v1
// API (models.ParseSelectString):
//
//	select=code,nameEn      only these fields (+ the envelope keys below)
//	select=-history         every field except these
//
// Includes win when both forms are mixed. The envelope keys every client
// needs to keep records apart (id, storeId, version, deleted) are always
// returned in include mode.
type fieldSelect struct {
	include map[string]bool
	exclude map[string]bool
}

var alwaysSelected = []string{"id", "storeId", "version", "deleted"}

var reSelectField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseSelect parses a select parameter; bad names give a 400 field error.
func parseSelect(s string) (fieldSelect, error) {
	fs := fieldSelect{}
	s = strings.TrimSpace(s)
	if s == "" {
		return fs, nil
	}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		neg := strings.HasPrefix(f, "-")
		name := strings.TrimPrefix(f, "-")
		if !reSelectField.MatchString(name) {
			return fieldSelect{}, errBadRequest("Invalid select.", map[string]string{"select": "unknown field name " + f})
		}
		if neg {
			if fs.exclude == nil {
				fs.exclude = map[string]bool{}
			}
			fs.exclude[name] = true
		} else {
			if fs.include == nil {
				fs.include = map[string]bool{}
			}
			fs.include[name] = true
		}
	}
	return fs, nil
}

func (fs fieldSelect) empty() bool { return len(fs.include) == 0 && len(fs.exclude) == 0 }

// wants reports whether field f is part of the response.
func (fs fieldSelect) wants(f string) bool {
	if len(fs.include) > 0 {
		if fs.include[f] {
			return true
		}
		for _, k := range alwaysSelected {
			if k == f {
				return true
			}
		}
		return false
	}
	return !fs.exclude[f]
}

// apply returns rec with only the selected fields (rec itself when no select).
func (fs fieldSelect) apply(rec M) M {
	if fs.empty() || rec == nil {
		return rec
	}
	out := make(M, len(rec))
	for k, v := range rec {
		if fs.wants(k) {
			out[k] = v
		}
	}
	return out
}

func (fs fieldSelect) applyAll(rows []M) []M {
	if fs.empty() {
		return rows
	}
	for i, r := range rows {
		rows[i] = fs.apply(r)
	}
	return rows
}

// dbProjection drops stored data the response will not carry, so it is not
// read from MongoDB at all. Only the change history is mapped (it is by far the
// largest part of a record and maps to one stored key); every other field is
// trimmed after mapping, because contract fields are computed from several
// legacy fields. historyKey is where the backend stores history ("erp.h" for
// legacy collections, "history" for native ones).
func (fs fieldSelect) dbProjection(historyKey string) bson.M {
	if fs.empty() || fs.wants("history") || historyKey == "" {
		return nil
	}
	return bson.M{historyKey: 0}
}
