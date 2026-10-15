package coffee

import (
	"context"
	"slices"
	"sync"

	"cloud.google.com/go/datastore"
)

// Members keeps who is in each group (97c), as account ids — shown as
// @names to the group's members only. Updated on start, join, leave and
// account deletion; a member who opens Groups is added if missing (members
// from before this existed).
type Members interface {
	Add(ctx context.Context, gid, uid string) error
	Remove(ctx context.Context, gid, uid string) error
	List(ctx context.Context, gid string) ([]string, error)
}

type groupMembers struct {
	Users []string `datastore:"users,noindex"`
}

// DatastoreMembers: one GroupMembers entity per group.
type DatastoreMembers struct{ DB *datastore.Client }

func membersKey(gid string) *datastore.Key { return datastore.NameKey("GroupMembers", gid, nil) }

func (s *DatastoreMembers) change(ctx context.Context, gid string, f func([]string) []string) error {
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var g groupMembers
		if err := tx.Get(membersKey(gid), &g); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		g.Users = f(g.Users)
		if len(g.Users) == 0 {
			return tx.Delete(membersKey(gid))
		}
		_, err := tx.Put(membersKey(gid), &g)
		return err
	})
	return err
}

func (s *DatastoreMembers) Add(ctx context.Context, gid, uid string) error {
	return s.change(ctx, gid, func(u []string) []string {
		if slices.Contains(u, uid) {
			return u
		}
		return append(u, uid)
	})
}

func (s *DatastoreMembers) Remove(ctx context.Context, gid, uid string) error {
	return s.change(ctx, gid, func(u []string) []string { return slices.DeleteFunc(u, func(x string) bool { return x == uid }) })
}

func (s *DatastoreMembers) List(ctx context.Context, gid string) ([]string, error) {
	var g groupMembers
	if err := s.DB.Get(ctx, membersKey(gid), &g); err != nil && err != datastore.ErrNoSuchEntity {
		return nil, err
	}
	return g.Users, nil
}

// MemoryMembers is Members in memory (tests, local runs).
type MemoryMembers struct {
	mu sync.Mutex
	m  map[string][]string
}

func (s *MemoryMembers) Add(_ context.Context, gid, uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string][]string{}
	}
	if !slices.Contains(s.m[gid], uid) {
		s.m[gid] = append(s.m[gid], uid)
	}
	return nil
}

func (s *MemoryMembers) Remove(_ context.Context, gid, uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[gid] = slices.DeleteFunc(s.m[gid], func(x string) bool { return x == uid })
	return nil
}

func (s *MemoryMembers) List(_ context.Context, gid string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.m[gid]), nil
}
