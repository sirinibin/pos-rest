package models

import (
	"github.com/sirinibin/startpos/backend/db"
)

// Document numbers come from Redis counters (INCR is atomic). Two rules keep
// them unique under concurrent creates:
//
//   - A counter is seeded with SETNX (seedCounter): when two creates find the
//     counter missing at the same moment, the first seed wins and the second
//     does not reset it after numbers were already handed out.
//   - A create that fails after taking a number gives it back only while it
//     is still the counter's latest value (counterClaims.release, one atomic
//     Redis script). Once someone else took a later number the failed one is
//     left as a gap: it is never handed out twice.

// seedCounter sets a missing counter; an existing counter is left alone.
func seedCounter(key string, value int64) error {
	return db.RedisClient.SetNX(key, value, 0).Err()
}

// counterClaims are the counter values a document took while being numbered
// (counter key → value). Kept on the document in an unexported field, so it
// is never stored or sent.
type counterClaims map[string]int64

// incr takes the next value of a counter and remembers it.
func (c *counterClaims) incr(key string) (int64, error) {
	n, err := db.RedisClient.Incr(key).Result()
	if err != nil {
		return 0, err
	}
	if *c == nil {
		*c = counterClaims{}
	}
	(*c)[key] = n
	return n, nil
}

// releaseCounterScript decrements KEYS[1] only when it still equals ARGV[1].
const releaseCounterScript = `
local cur = redis.call('GET', KEYS[1])
if cur and tonumber(cur) == tonumber(ARGV[1]) then
  return redis.call('DECR', KEYS[1])
end
return -1`

// release gives back every number this document took that nobody has
// followed yet, and forgets them (a second release is a no-op).
func (c *counterClaims) release() error {
	var firstErr error
	for key, n := range *c {
		if err := db.RedisClient.Eval(releaseCounterScript, []string{key}, n).Err(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	*c = nil
	return firstErr
}
