// Package kv persists the small in-memory maps that stores fall back to
// without Datastore (businesses, named links, short codes…), so a
// self-hosted Kafumu keeps them across restarts. With no Default set
// (local runs, tests) it does nothing and the maps stay memory-only.
//
// Values are gob-encoded: every exported field is kept, including the
// ones a type hides from its public JSON.
package kv

import (
	"bytes"
	"encoding/gob"
	"log"
)

// Persister stores encoded values by kind and id (sqlstore.KV).
type Persister interface {
	LoadKind(kind string) (map[string][]byte, error)
	SaveOne(kind, id string, data []byte) error
	DeleteOne(kind, id string) error
}

// Default is set by main when a persistent store is configured.
var Default Persister

// Load fills into with everything stored under kind.
func Load[T any](kind string, into map[string]T) {
	if Default == nil {
		return
	}
	all, err := Default.LoadKind(kind)
	if err != nil {
		log.Printf("kv: load %s: %v", kind, err)
		return
	}
	for id, data := range all {
		var v T
		if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&v); err != nil {
			log.Printf("kv: decode %s/%s: %v", kind, id, err)
			continue
		}
		into[id] = v
	}
}

// Save stores v under kind/id.
func Save(kind, id string, v any) {
	if Default == nil {
		return
	}
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(v); err != nil {
		log.Printf("kv: encode %s/%s: %v", kind, id, err)
		return
	}
	if err := Default.SaveOne(kind, id, b.Bytes()); err != nil {
		log.Printf("kv: save %s/%s: %v", kind, id, err)
	}
}

// Delete removes kind/id.
func Delete(kind, id string) {
	if Default == nil {
		return
	}
	if err := Default.DeleteOne(kind, id); err != nil {
		log.Printf("kv: delete %s/%s: %v", kind, id, err)
	}
}
