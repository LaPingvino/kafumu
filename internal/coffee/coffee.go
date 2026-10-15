// Package coffee is Kafumu's side of CoffeeBucks (K$, VISION §4): a client
// for the LiquidityPub service "lp" (~/liquiditypub/gae), where every group is
// its own K$ node with its own dividend. Kafumu knows who is signed in; the lp
// service only knows pseudonymous member ids and checks Kafumu's admin token.
package coffee

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

// Client talks to the lp service's admin API.
type Client struct {
	Base string // the lp service, e.g. https://lp-dot-lokumo.ew.r.appspot.com
	DB   *datastore.Client
	HTTP *http.Client

	// For tests: set both and DB is not needed.
	Token string
	Key   []byte

	mu sync.Mutex
}

// ErrOff: no lp service configured (local runs without it).
var ErrOff = errors.New("coffeebucks: not configured")

func (c *Client) secrets(ctx context.Context) (token string, key []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Token != "" && c.Key != nil {
		return c.Token, c.Key, nil
	}
	if c.DB == nil || c.Base == "" {
		return "", nil, ErrOff
	}
	// The admin token: made by the lp service, in its own namespace.
	tk := datastore.NameKey("LPConfig", "coffeebucks", nil)
	tk.Namespace = "lp"
	var cfg struct {
		AdminToken string `datastore:"admin_token,noindex"`
	}
	if err := c.DB.Get(ctx, tk, &cfg); err != nil {
		return "", nil, fmt.Errorf("coffeebucks: admin token: %w", err)
	}
	// Kafumu's own key for member ids and invites: made once, kept here.
	kk := datastore.NameKey("Config", "coffee", nil)
	var k struct {
		Key []byte `datastore:"key,noindex"`
	}
	_, err = c.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		if err := tx.Get(kk, &k); err == nil && len(k.Key) == 32 {
			return nil
		} else if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		k.Key = make([]byte, 32)
		if _, err := rand.Read(k.Key); err != nil {
			return err
		}
		_, err := tx.Put(kk, &k)
		return err
	})
	if err != nil {
		return "", nil, err
	}
	c.Token, c.Key = cfg.AdminToken, k.Key
	return c.Token, c.Key, nil
}

func mac(key []byte, parts ...string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(m.Sum(nil))
}

// MemberID is a Kafumu account's name in every group's ledger: "u" + 16 hex
// of a keyed hash of the user id — pseudonymous, and nothing stored maps it
// back (only this server can compute it, from the signed-in user).
func (c *Client) MemberID(ctx context.Context, userID string) (string, error) {
	_, key, err := c.secrets(ctx)
	if err != nil {
		return "", err
	}
	return "u" + mac(key, "member", userID)[:16], nil
}

// InviteToken: the proof in an invite link that member `by` of group gid
// invited (vouched); nothing is stored, the server recomputes it.
func (c *Client) InviteToken(ctx context.Context, gid, by string) (string, error) {
	_, key, err := c.secrets(ctx)
	if err != nil {
		return "", err
	}
	return mac(key, "invite", gid, by)[:20], nil
}

// ValidInvite checks an invite link's token.
func (c *Client) ValidInvite(ctx context.Context, gid, by, token string) bool {
	want, err := c.InviteToken(ctx, gid, by)
	return err == nil && hmac.Equal([]byte(want), []byte(token))
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// NewGroupID makes a group id from its name: a readable slug and 4 random
// hex, e.g. "barreiro-coffee-3fa9".
func NewGroupID(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 24 {
		s = strings.TrimRight(s[:24], "-")
	}
	if s == "" {
		s = "group"
	}
	b := make([]byte, 2)
	rand.Read(b)
	return s + "-" + hex.EncodeToString(b)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	token, _, err := c.secrets(ctx)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	if in != nil {
		json.NewEncoder(&body).Encode(in)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct{ Error string }
		json.NewDecoder(res.Body).Decode(&e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

// CreateGroup starts a group (its own K$ node and dividend).
func (c *Client) CreateGroup(ctx context.Context, gid, name string) error {
	return c.do(ctx, "POST", "/lp/admin/group", map[string]string{"gid": gid, "name": name}, nil)
}

// AddMember adds a member to a group (the dividend reaches them from now on).
func (c *Client) AddMember(ctx context.Context, gid, member string) error {
	return c.do(ctx, "POST", "/lp/admin/member", map[string]string{"gid": gid, "member": member}, nil)
}

// Wallet is a member's view of one group.
type Wallet struct {
	Member      string `json:"member"`
	Active      bool   `json:"active"`
	Balance     int64  `json:"balance"`    // micro-units
	CurrentUD   int64  `json:"current_ud"` // today's dividend, micro-units
	Members     int64  `json:"members"`
	MoneySupply int64  `json:"money_supply"`
}

// Balance reads a member's wallet in a group.
func (c *Client) Balance(ctx context.Context, gid, member string) (Wallet, error) {
	var w Wallet
	err := c.do(ctx, "GET", "/lp/admin/balance?gid="+gid+"&member="+member, nil, &w)
	return w, err
}

// Pay moves micro-units between two members of a group.
func (c *Client) Pay(ctx context.Context, gid, from, to string, amount int64) error {
	return c.do(ctx, "POST", "/lp/admin/pay", map[string]any{"gid": gid, "from": from, "to": to, "amount": amount}, nil)
}

var names = struct {
	sync.Mutex
	m map[string]nameEntry
}{m: map[string]nameEntry{}}

type nameEntry struct {
	name string
	at   time.Time
}

// GroupName reads a group's name from its public identity document (cached
// ten minutes per instance); "" if the group doesn't exist.
func (c *Client) GroupName(ctx context.Context, gid string) string {
	names.Lock()
	e, ok := names.m[gid]
	names.Unlock()
	if ok && time.Since(e.at) < 10*time.Minute {
		return e.name
	}
	if c.Base == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.Base+"/lp/g/"+gid+"/.well-known/liquiditypub", nil)
	if err != nil {
		return ""
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var doc struct {
		Node struct{ Name string } `json:"node"`
	}
	if res.StatusCode == http.StatusOK {
		json.NewDecoder(res.Body).Decode(&doc)
	}
	names.Lock()
	if len(names.m) > 1000 {
		names.m = map[string]nameEntry{}
	}
	names.m[gid] = nameEntry{doc.Node.Name, time.Now()}
	names.Unlock()
	return doc.Node.Name
}

// Money formats micro-units as "12.34" (two decimals, floored).
func Money(micro int64) string {
	sign := ""
	if micro < 0 {
		sign, micro = "-", -micro
	}
	return fmt.Sprintf("%s%d.%02d", sign, micro/1_000_000, micro%1_000_000/10_000)
}
