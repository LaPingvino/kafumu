package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/coffee"
)

// Coffee is CoffeeBucks on Kafumu's side (VISION §4): start a group, invite,
// join. The money itself lives in the lp service; this knows who is signed in.
type Coffee struct {
	Home     *Home
	Accounts *account.Service
	C        *coffee.Client
	Asks     coffee.Asks // link requests (96n)
}

// MaxGroups bounds how many groups one account is in.
const MaxGroups = 5

// mayStart: like adding a source (81c), starting a group needs a named
// account with a passkey or a role — someone who can't be made by the dozen.
func mayStart(u *account.User) bool {
	return u != nil && u.Username != "" && (len(u.WebAuthnCredentials()) > 0 || u.Role != "")
}

// StartGroup handles POST /coffee/group (name): a new group, its own K$ node
// with its own dividend, the starter its first member.
func (c *Coffee) StartGroup(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if !mayStart(u) {
		http.Error(w, "a named account with a passkey starts a group", http.StatusForbidden)
		return
	}
	if len(u.Groups) >= MaxGroups {
		http.Error(w, "in too many groups", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len([]rune(name)) > 60 {
		http.Error(w, "a name, up to 60 characters", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	gid := coffee.NewGroupID(name)
	me, err := c.C.MemberID(ctx, u.ID)
	if err == nil {
		err = c.C.CreateGroup(ctx, gid, name)
	}
	if err == nil {
		err = c.C.AddMember(ctx, gid, me)
	}
	if err == nil {
		err = c.Accounts.Update(ctx, u, func(x *account.User) { x.Groups = append(x.Groups, gid) })
	}
	if err != nil {
		log.Printf("coffee: start group: %v", err)
		http.Error(w, "could not start the group", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/coffee?g="+gid, http.StatusSeeOther)
}

// Invite handles GET /coffee/invite?g=: a member's invite link for a group
// (the link carries who vouched; nothing is stored).
func (c *Coffee) Invite(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	gid := r.FormValue("g")
	if u == nil || !slices.Contains(u.Groups, gid) {
		http.Error(w, "not a member", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	me, err := c.C.MemberID(ctx, u.ID)
	var t string
	if err == nil {
		t, err = c.C.InviteToken(ctx, gid, me)
	}
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	q := url.Values{"g": {gid}, "by": {me}, "t": {t}}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{"link": c.Home.Cfg.Origin + "/coffee/join?" + q.Encode()})
}

// Join handles POST /coffee/join (g, by, t): a named account joins a group
// through a member's invite.
func (c *Coffee) Join(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || u.Username == "" {
		http.Redirect(w, r, "/account?next="+url.QueryEscape("/coffee/join?"+r.Form.Encode()), http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	gid, by := r.FormValue("g"), r.FormValue("by")
	if !c.C.ValidInvite(ctx, gid, by, r.FormValue("t")) {
		http.Error(w, "this invite isn't valid", http.StatusForbidden)
		return
	}
	if slices.Contains(u.Groups, gid) {
		http.Redirect(w, r, "/coffee?g="+gid, http.StatusSeeOther)
		return
	}
	if len(u.Groups) >= MaxGroups {
		http.Error(w, "in too many groups", http.StatusBadRequest)
		return
	}
	me, err := c.C.MemberID(ctx, u.ID)
	if err == nil && me == by {
		http.Error(w, "that's your own invite", http.StatusBadRequest)
		return
	}
	if err == nil {
		err = c.C.AddMember(ctx, gid, me)
	}
	if err == nil {
		err = c.Accounts.Update(ctx, u, func(x *account.User) { x.Groups = append(x.Groups, gid) })
	}
	if err != nil {
		log.Printf("coffee: join %s: %v", gid, err)
		http.Error(w, "could not join", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/coffee?g="+gid, http.StatusSeeOther)
}

type coffeeGroup struct {
	Currency, Sym string        // the group's own currency name and symbol (96k)
	Links         []coffee.Link // groups this one is linked with
	Gid, Name     string
	Balance, UD   string // K$, two decimals
	Days          string // the balance in days of dividend
	Members       int64
	Unavailable   bool
}

type coffeePage struct {
	page
	Groups   []coffeeGroup
	MayStart bool
	Focus    string
	// Msg: how the last payment went (paid, funds, not_member, self, amount,
	// error), with To and Amount for "paid".
	Msg, To, Amount string
	// Pairs: two of your groups not linked yet (you can link them).
	Pairs [][2]coffeeGroup
	// Asks: requests to link addressed to you (96n).
	Asks []askView
}

type askView struct {
	ID, From, FromGroup, ToGroup, Amount, Sym string
}

// Show renders /coffee: your groups (balance, today's dividend), invites,
// starting a group.
func (c *Coffee) Show(w http.ResponseWriter, r *http.Request) {
	p := coffeePage{page: c.Home.newPage(r, "CoffeeBucks"), Focus: r.FormValue("g"), Msg: r.FormValue("m"), To: r.FormValue("to"), Amount: r.FormValue("a")}
	p.Tab = "account"
	u := p.User
	p.MayStart = mayStart(u)
	if u != nil {
		me, err := c.C.MemberID(r.Context(), u.ID)
		for _, gid := range u.Groups {
			info := c.C.GroupInfo(r.Context(), gid)
			g := coffeeGroup{Gid: gid, Name: info.Name, Currency: info.Currency, Sym: info.Symbol}
			if g.Name == "" {
				g.Name = gid
			}
			if err == nil {
				if wl, berr := c.C.Balance(r.Context(), gid, me); berr == nil {
					g.Balance, g.UD, g.Members = coffee.Money(wl.Balance), coffee.Money(wl.CurrentUD), wl.Members
					if wl.CurrentUD > 0 {
						g.Days = fmt.Sprintf("%.1f", float64(wl.Balance)/float64(wl.CurrentUD))
					}
				} else {
					g.Unavailable = true
				}
			} else {
				g.Unavailable = true
			}
			if !g.Unavailable {
				g.Links, _ = c.C.Links(r.Context(), gid)
			}
			p.Groups = append(p.Groups, g)
		}
		for i, a := range p.Groups {
			for _, b := range p.Groups[i+1:] {
				linked := false
				for _, l := range a.Links {
					linked = linked || l.Gid == b.Gid
				}
				if !linked && !a.Unavailable && !b.Unavailable {
					p.Pairs = append(p.Pairs, [2]coffeeGroup{a, b})
				}
			}
		}
	}
	if u != nil {
		for _, a := range c.pendingAsks(r.Context(), u) {
			fi, ti := c.C.GroupInfo(r.Context(), a.FromGid), c.C.GroupInfo(r.Context(), a.ToGid)
			v := askView{ID: a.ID, FromGroup: fi.Name, ToGroup: ti.Name, Amount: coffee.Money(a.Amount), Sym: fi.Symbol}
			if payer, err := c.Accounts.ByID(r.Context(), a.FromUser); err == nil && payer != nil {
				v.From = payer.Username
			}
			p.Asks = append(p.Asks, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	c.Home.render(w, "coffee.html", p)
}

type joinPage struct {
	page
	Gid, By, T, Name string
	Valid, Member    bool
}

// ShowJoin renders /coffee/join?g=&by=&t=: which group, and Join.
func (c *Coffee) ShowJoin(w http.ResponseWriter, r *http.Request) {
	p := joinPage{page: c.Home.newPage(r, "CoffeeBucks"), Gid: r.FormValue("g"), By: r.FormValue("by"), T: r.FormValue("t")}
	p.Tab = "account"
	p.Valid = c.C.ValidInvite(r.Context(), p.Gid, p.By, p.T)
	if p.Valid {
		p.Name = c.C.GroupName(r.Context(), p.Gid)
		p.Valid = p.Name != ""
	}
	if u := p.User; u != nil {
		p.Member = slices.Contains(u.Groups, p.Gid)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer") // the link carries an invite
	c.Home.render(w, "coffee_join.html", p)
}

// Pay handles POST /coffee/pay (g, to, amount): pay @name, a member of the
// same group, in K$. The ledger sees two pseudonymous member ids; who they
// are stays with Kafumu.
func (c *Coffee) Pay(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	gid := r.FormValue("g")
	back := func(msg string, extra ...string) {
		q := url.Values{"g": {gid}, "m": {msg}}
		for i := 0; i+1 < len(extra); i += 2 {
			q.Set(extra[i], extra[i+1])
		}
		http.Redirect(w, r, "/coffee?"+q.Encode()+"#g-"+gid, http.StatusSeeOther)
	}
	if u == nil || !slices.Contains(u.Groups, gid) {
		http.Error(w, "not a member", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("to")), "@"))
	amount, ok := coffee.ParseMoney(r.FormValue("amount"))
	if !ok {
		back("bad_amount")
		return
	}
	id, err := c.Accounts.Store.LookupUsername(ctx, name)
	var to *account.User
	if err == nil && id != "" {
		to, err = c.Accounts.ByID(ctx, id)
	}
	if err != nil || to == nil {
		back("not_member", "to", name)
		return
	}
	if !slices.Contains(to.Groups, gid) {
		// In a group linked with this one? Then it's a payment across the
		// pool: show what arrives first (/coffee/confirm).
		if links, lerr := c.C.Links(ctx, gid); lerr == nil {
			for _, l := range links {
				if slices.Contains(to.Groups, l.Gid) {
					q := url.Values{"g": {gid}, "tg": {l.Gid}, "to": {name}, "a": {fmt.Sprint(amount)}}
					http.Redirect(w, r, "/coffee/confirm?"+q.Encode(), http.StatusSeeOther)
					return
				}
			}
		}
		if len(to.Groups) > 0 && to.ID != u.ID {
			// Not linked yet: ask them to link our groups and take the payment (96n).
			q := url.Values{"g": {gid}, "tg": {to.Groups[0]}, "to": {name}, "a": {fmt.Sprint(amount)}}
			http.Redirect(w, r, "/coffee/linkpay?"+q.Encode(), http.StatusSeeOther)
			return
		}
		back("not_member", "to", name)
		return
	}
	if to.ID == u.ID {
		back("self")
		return
	}
	from, err1 := c.C.MemberID(ctx, u.ID)
	them, err2 := c.C.MemberID(ctx, to.ID)
	if err1 != nil || err2 != nil {
		back("error")
		return
	}
	if err := c.C.Pay(ctx, gid, from, them, amount); err != nil {
		if strings.Contains(err.Error(), "insufficient") {
			back("funds")
		} else {
			log.Printf("coffee: pay in %s: %v", gid, err)
			back("error")
		}
		return
	}
	back("paid", "to", name, "a", coffee.Money(amount))
}

// Link handles POST /coffee/link (a, b): someone in both groups links them —
// how groups "link up" (VISION §4): a pool seeded with a hundred dividends a side.
func (c *Coffee) Link(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	a, b := r.FormValue("a"), r.FormValue("b")
	if u == nil || a == b || !slices.Contains(u.Groups, a) || !slices.Contains(u.Groups, b) {
		http.Error(w, "link groups you're in", http.StatusForbidden)
		return
	}
	m := "linked"
	if err := c.C.LinkGroups(r.Context(), a, b); err != nil {
		log.Printf("coffee: link %s-%s: %v", a, b, err)
		m = "error"
	}
	http.Redirect(w, r, "/coffee?"+url.Values{"g": {a}, "m": {m}}.Encode()+"#g-"+a, http.StatusSeeOther)
}

type confirmPage struct {
	page
	FromSym, ToSym             string
	Gid, Tg, From, ToGroup, To string
	Amount                     int64
	Pay, Gets                  string
	Ok                         bool
}

// across resolves a payment across groups from the form: the payer in g, @to
// in tg, the groups linked. ok false when any of that doesn't hold.
func (c *Coffee) across(r *http.Request) (u, to *account.User, gid, tg string, amount int64, ok bool) {
	u = UserFrom(r.Context())
	gid, tg = r.FormValue("g"), r.FormValue("tg")
	name := strings.ToLower(strings.TrimPrefix(r.FormValue("to"), "@"))
	fmt.Sscan(r.FormValue("a"), &amount)
	if u == nil || amount <= 0 || !slices.Contains(u.Groups, gid) {
		return
	}
	id, err := c.Accounts.Store.LookupUsername(r.Context(), name)
	if err != nil || id == "" {
		return
	}
	if to, err = c.Accounts.ByID(r.Context(), id); err != nil || to == nil || !slices.Contains(to.Groups, tg) || to.ID == u.ID {
		return
	}
	links, err := c.C.Links(r.Context(), gid)
	if err != nil {
		return
	}
	for _, l := range links {
		if l.Gid == tg {
			return u, to, gid, tg, amount, true
		}
	}
	return
}

// Confirm renders /coffee/confirm: what you pay, what arrives, at today's rate.
func (c *Coffee) Confirm(w http.ResponseWriter, r *http.Request) {
	p := confirmPage{page: c.Home.newPage(r, "CoffeeBucks")}
	p.Tab = "account"
	_, to, gid, tg, amount, ok := c.across(r)
	if ok {
		if dst, err := c.C.Quote(r.Context(), gid, tg, amount); err == nil {
			p.Ok, p.Gid, p.Tg, p.To, p.Amount = true, gid, tg, to.Username, amount
			p.Pay, p.Gets = coffee.Money(amount), coffee.Money(dst)
			fi, ti := c.C.GroupInfo(r.Context(), gid), c.C.GroupInfo(r.Context(), tg)
			p.From, p.ToGroup, p.FromSym, p.ToSym = fi.Name, ti.Name, fi.Symbol, ti.Symbol
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	c.Home.render(w, "coffee_confirm.html", p)
}

// Across handles POST /coffee/across: the confirmed payment, through the pool.
func (c *Coffee) Across(w http.ResponseWriter, r *http.Request) {
	u, to, gid, tg, amount, ok := c.across(r)
	if !ok {
		http.Error(w, "not possible", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	from, err1 := c.C.MemberID(ctx, u.ID)
	them, err2 := c.C.MemberID(ctx, to.ID)
	m := "sent"
	if err1 != nil || err2 != nil {
		m = "error"
	} else if _, err := c.C.Transfer(ctx, gid, from, tg, them, amount); err != nil {
		log.Printf("coffee: across %s->%s: %v", gid, tg, err)
		m = "error"
		if strings.Contains(err.Error(), "balance") {
			m = "funds"
		} else if strings.Contains(err.Error(), "busy") {
			m = "busy"
		}
	}
	q := url.Values{"g": {gid}, "m": {m}, "to": {to.Username}, "a": {coffee.Money(amount)}}
	http.Redirect(w, r, "/coffee?"+q.Encode()+"#g-"+gid, http.StatusSeeOther)
}

type leavePage struct {
	page
	Gid, Name, Balance, Sym string
	MoveTo                  []coffee.Link // linked groups you're also in
	Err, To                 string
	// Last: you're the only active member (96j): give has nobody to go to and
	// sharing out would only sit in the fund, so it's move or destroy.
	Last bool
}

// ShowLeave renders /coffee/leave?g=: what happens to your balance.
func (c *Coffee) ShowLeave(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	gid := r.FormValue("g")
	if u == nil || !slices.Contains(u.Groups, gid) {
		http.Redirect(w, r, "/coffee", http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	info := c.C.GroupInfo(ctx, gid)
	p := leavePage{page: c.Home.newPage(r, "CoffeeBucks"), Gid: gid, Name: info.Name, Sym: info.Symbol, Err: r.FormValue("m"), To: r.FormValue("to")}
	p.Tab = "account"
	if p.Name == "" {
		p.Name = gid
	}
	if me, err := c.C.MemberID(ctx, u.ID); err == nil {
		if wl, err := c.C.Balance(ctx, gid, me); err == nil {
			p.Balance = coffee.Money(wl.Balance)
			p.Last = wl.Members <= 1
		}
	}
	if links, err := c.C.Links(ctx, gid); err == nil {
		for _, l := range links {
			if slices.Contains(u.Groups, l.Gid) {
				p.MoveTo = append(p.MoveTo, l)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	c.Home.render(w, "coffee_leave.html", p)
}

// Leave handles POST /coffee/leave (g, mode: move|give|share|destroy, tg, to,
// sure): the balance goes as chosen, then you're out of the group.
func (c *Coffee) Leave(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	gid := r.FormValue("g")
	if u == nil || !slices.Contains(u.Groups, gid) {
		http.Error(w, "not a member", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	again := func(m string, extra ...string) {
		q := url.Values{"g": {gid}, "m": {m}}
		for i := 0; i+1 < len(extra); i += 2 {
			q.Set(extra[i], extra[i+1])
		}
		http.Redirect(w, r, "/coffee/leave?"+q.Encode(), http.StatusSeeOther)
	}
	if r.FormValue("sure") == "" {
		again("sure")
		return
	}
	me, err := c.C.MemberID(ctx, u.ID)
	if err != nil {
		again("error")
		return
	}
	mode := r.FormValue("mode")
	if mode == "give" || mode == "share" {
		if wl, berr := c.C.Balance(ctx, gid, me); berr == nil && wl.Members <= 1 {
			again("last")
			return
		}
	}
	switch mode {
	case "move":
		tg := r.FormValue("tg")
		if !slices.Contains(u.Groups, tg) || tg == gid {
			again("error")
			return
		}
		err = c.C.Move(ctx, gid, me, tg, me) // the same member id in every group
	case "give":
		name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("to")), "@"))
		id, lerr := c.Accounts.Store.LookupUsername(ctx, name)
		var to *account.User
		if lerr == nil && id != "" {
			to, lerr = c.Accounts.ByID(ctx, id)
		}
		if lerr != nil || to == nil || to.ID == u.ID || !slices.Contains(to.Groups, gid) {
			again("give_bad", "to", name)
			return
		}
		them, merr := c.C.MemberID(ctx, to.ID)
		if merr != nil {
			again("error")
			return
		}
		err = c.C.Leave(ctx, gid, me, "give", them)
	case "share":
		err = c.C.Leave(ctx, gid, me, "redistribute", "")
	case "destroy":
		err = c.C.Leave(ctx, gid, me, "destroy", "")
	default:
		again("error")
		return
	}
	if err == nil {
		err = c.Accounts.Update(ctx, u, func(x *account.User) {
			x.Groups = slices.DeleteFunc(x.Groups, func(g string) bool { return g == gid })
		})
	}
	if err != nil {
		log.Printf("coffee: leave %s: %v", gid, err)
		again("error")
		return
	}
	http.Redirect(w, r, "/coffee?m=left", http.StatusSeeOther)
}

// Rename handles POST /coffee/rename (g, name, currency, symbol): any member
// renames the group, its currency or its symbol (Joop: that keeps groups small
// enough to trust each other, and the whimsy in it).
func (c *Coffee) Rename(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	gid := r.FormValue("g")
	if u == nil || !slices.Contains(u.Groups, gid) {
		http.Error(w, "not a member", http.StatusForbidden)
		return
	}
	m := "renamed"
	if err := c.C.Rename(r.Context(), gid, r.FormValue("name"), r.FormValue("currency"), r.FormValue("symbol")); err != nil {
		log.Printf("coffee: rename %s: %v", gid, err)
		m = "rename_bad"
	}
	http.Redirect(w, r, "/coffee?"+url.Values{"g": {gid}, "m": {m}}.Encode()+"#g-"+gid, http.StatusSeeOther)
}

// userByName finds an account by @name.
func (c *Coffee) userByName(ctx context.Context, name string) *account.User {
	name = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "@"))
	id, err := c.Accounts.Store.LookupUsername(ctx, name)
	if err != nil || id == "" {
		return nil
	}
	u, err := c.Accounts.ByID(ctx, id)
	if err != nil {
		return nil
	}
	return u
}

// LinkAsk handles POST /coffee/linkask (g, tg, to, a): the payer asks @to to
// link the payer's group g with tg (the payee's) and take a payment of a.
func (c *Coffee) LinkAsk(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	ctx := r.Context()
	gid, tg := r.FormValue("g"), r.FormValue("tg")
	var amount int64
	fmt.Sscan(r.FormValue("a"), &amount)
	to := c.userByName(ctx, r.FormValue("to"))
	if u == nil || to == nil || to.ID == u.ID || gid == tg || amount <= 0 || !slices.Contains(u.Groups, gid) || !slices.Contains(to.Groups, tg) || c.Asks == nil {
		http.Error(w, "not possible", http.StatusBadRequest)
		return
	}
	a := &coffee.LinkAsk{FromGid: gid, ToGid: tg, FromUser: u.ID, ToUser: to.ID, Amount: amount, ExpiresAt: time.Now().Add(coffee.AskTTL)}
	if err := c.Asks.Put(ctx, a); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	c.Accounts.Update(ctx, to, func(x *account.User) { x.CoffeeAsks = append(x.CoffeeAsks, a.ID) })
	q := url.Values{"g": {gid}, "m": {"asked"}, "to": {to.Username}, "a": {coffee.Money(amount)}}
	http.Redirect(w, r, "/coffee?"+q.Encode()+"#g-"+gid, http.StatusSeeOther)
}

// pendingAsks: the link requests addressed to u that are still there.
func (c *Coffee) pendingAsks(ctx context.Context, u *account.User) []*coffee.LinkAsk {
	if c.Asks == nil || u == nil {
		return nil
	}
	var out []*coffee.LinkAsk
	for _, id := range u.CoffeeAsks {
		if a, err := c.Asks.Get(ctx, id); err == nil && a.ToUser == u.ID {
			out = append(out, a)
		}
	}
	return out
}

func (c *Coffee) dropAsk(ctx context.Context, u *account.User, id string) {
	if c.Asks != nil {
		c.Asks.Delete(ctx, id)
	}
	c.Accounts.Update(ctx, u, func(x *account.User) {
		x.CoffeeAsks = slices.DeleteFunc(x.CoffeeAsks, func(s string) bool { return s == id })
	})
}

// LinkAccept handles POST /coffee/linkaccept (id): the payee agrees; the two
// groups link (each puts a hundred days of dividend into the pool) and the
// payment goes through it.
func (c *Coffee) LinkAccept(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	ctx := r.Context()
	if u == nil || c.Asks == nil {
		http.Error(w, "sign in", http.StatusForbidden)
		return
	}
	id := r.FormValue("id")
	a, err := c.Asks.Get(ctx, id)
	if err != nil || a.ToUser != u.ID {
		http.Redirect(w, r, "/coffee?m=ask_gone", http.StatusSeeOther)
		return
	}
	payer, perr := c.Accounts.ByID(ctx, a.FromUser)
	if perr != nil || payer == nil || !slices.Contains(payer.Groups, a.FromGid) || !slices.Contains(u.Groups, a.ToGid) {
		c.dropAsk(ctx, u, id)
		http.Redirect(w, r, "/coffee?m=ask_gone", http.StatusSeeOther)
		return
	}
	from, e1 := c.C.MemberID(ctx, payer.ID)
	me, e2 := c.C.MemberID(ctx, u.ID)
	if e1 != nil || e2 != nil {
		http.Redirect(w, r, "/coffee?m=error", http.StatusSeeOther)
		return
	}
	if err := c.C.LinkGroups(ctx, a.FromGid, a.ToGid); err != nil && !strings.Contains(err.Error(), "exist") && !strings.Contains(err.Error(), "already") {
		log.Printf("coffee: link on accept: %v", err)
		http.Redirect(w, r, "/coffee?m=error", http.StatusSeeOther)
		return
	}
	// The link opens in a moment; the payment waits for it (a few tries).
	var terr error
	for i := 0; i < 25; i++ {
		if _, terr = c.C.Transfer(ctx, a.FromGid, from, a.ToGid, me, a.Amount); terr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	c.dropAsk(ctx, u, id)
	m := "accepted"
	if terr != nil {
		log.Printf("coffee: pay on accept: %v", terr)
		m = "linked" // linked, but the payment didn't go: the payer can pay now
	}
	q := url.Values{"g": {a.ToGid}, "m": {m}, "a": {coffee.Money(a.Amount)}}
	http.Redirect(w, r, "/coffee?"+q.Encode()+"#g-"+a.ToGid, http.StatusSeeOther)
}

// LinkDecline handles POST /coffee/linkdecline (id).
func (c *Coffee) LinkDecline(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Error(w, "sign in", http.StatusForbidden)
		return
	}
	id := r.FormValue("id")
	if a, err := c.Asks.Get(r.Context(), id); err == nil && a.ToUser == u.ID {
		c.dropAsk(r.Context(), u, id)
	}
	http.Redirect(w, r, "/coffee?m=declined", http.StatusSeeOther)
}

type linkPayPage struct {
	page
	Gid, Tg, To, A, Pay, Sym, From, ToGroup string
	Ok                                      bool
}

// LinkPay renders /coffee/linkpay?g=&tg=&to=&a=: your groups aren't linked;
// what linking means, in plain words; ask @to to link and accept.
func (c *Coffee) LinkPay(w http.ResponseWriter, r *http.Request) {
	p := linkPayPage{page: c.Home.newPage(r, "CoffeeBucks"), Gid: r.FormValue("g"), Tg: r.FormValue("tg"), A: r.FormValue("a")}
	p.Tab = "account"
	u := p.User
	var amount int64
	fmt.Sscan(p.A, &amount)
	if to := c.userByName(r.Context(), r.FormValue("to")); u != nil && to != nil && amount > 0 && slices.Contains(u.Groups, p.Gid) && slices.Contains(to.Groups, p.Tg) && p.Gid != p.Tg {
		fi, ti := c.C.GroupInfo(r.Context(), p.Gid), c.C.GroupInfo(r.Context(), p.Tg)
		p.Ok, p.To, p.Pay, p.Sym, p.From, p.ToGroup = true, to.Username, coffee.Money(amount), fi.Symbol, fi.Name, ti.Name
	}
	w.Header().Set("Cache-Control", "no-store")
	c.Home.render(w, "coffee_linkpay.html", p)
}
