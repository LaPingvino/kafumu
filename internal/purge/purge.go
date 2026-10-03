// Package purge deletes what Kafumu promised to forget (see /privacy):
// expired meetups, mailboxes and slots, and accounts nobody uses. It runs
// daily from App Engine cron, in bounded batches of keys-only queries.
package purge

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
)

const batch = 500

// Rules for accounts: nameless ones go after 30 idle days, named ones
// after a year (or their own KeepDays).
const (
	AnonymousIdle = 30 * 24 * time.Hour
	NamedIdle     = 365 * 24 * time.Hour
)

// Result counts what was deleted.
type Result struct{ Meetups, Boxes, Slots, ATSessions, Users, Usernames int }

func (r Result) String() string {
	return fmt.Sprintf("meetups=%d boxes=%d slots=%d atproto=%d users=%d usernames=%d", r.Meetups, r.Boxes, r.Slots, r.ATSessions, r.Users, r.Usernames)
}

// Run deletes expired entities and idle accounts as of now.
func Run(ctx context.Context, db *datastore.Client, now time.Time) (Result, error) {
	var r Result
	var err error
	for _, k := range []struct {
		kind string
		n    *int
	}{{"Meetup", &r.Meetups}, {"Box", &r.Boxes}, {"Slot", &r.Slots}, {"ATSession", &r.ATSessions}, {"ATAuthRequest", &r.ATSessions}, {"PushSub", &r.Boxes}} {
		q := datastore.NewQuery(k.kind).FilterField("expires_at", "<", now).KeysOnly().Limit(batch)
		n, err := deleteAll(ctx, db, q)
		if err != nil {
			return r, fmt.Errorf("%s: %w", k.kind, err)
		}
		*k.n += n
	}
	// Accounts: everything idle for 30 days is a candidate; named ones are
	// kept for a year.
	q := datastore.NewQuery("User").FilterField("last_seen_at", "<", now.Add(-AnonymousIdle)).Limit(batch)
	var us []struct {
		Username   string    `datastore:"username"`
		LastSeenAt time.Time `datastore:"last_seen_at"`
		KeepDays   int       `datastore:"keep_days,noindex"`
	}
	keys, err := db.GetAll(ctx, q, &us)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			return r, fmt.Errorf("users: %w", err)
		}
	}
	var del, names []*datastore.Key
	for i, k := range keys {
		u := us[i]
		idle := now.Sub(u.LastSeenAt)
		keep := AnonymousIdle
		if u.Username != "" {
			keep = NamedIdle
			if u.KeepDays > 0 {
				keep = time.Duration(u.KeepDays) * 24 * time.Hour
			}
		}
		if u.KeepDays == -1 || idle < keep {
			continue
		}
		del = append(del, k)
		if u.Username != "" {
			names = append(names, datastore.NameKey("Username", u.Username, nil))
		}
	}
	if len(del) > 0 {
		if err := db.DeleteMulti(ctx, del); err != nil {
			return r, fmt.Errorf("users: %w", err)
		}
		r.Users = len(del)
	}
	if len(names) > 0 {
		if err := db.DeleteMulti(ctx, names); err == nil {
			r.Usernames = len(names)
		}
	}
	return r, nil
}

func deleteAll(ctx context.Context, db *datastore.Client, q *datastore.Query) (int, error) {
	keys, err := db.GetAll(ctx, q, nil)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	return len(keys), db.DeleteMulti(ctx, keys)
}
