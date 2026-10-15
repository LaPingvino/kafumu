package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/coffee"
)

// Coffee is CoffeeBucks on Kafumu's side (VISION §4): start a group, invite,
// join. The money itself lives in the lp service; this knows who is signed in.
type Coffee struct {
	Home     *Home
	Accounts *account.Service
	C        *coffee.Client
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
	Links       []coffee.Link // groups this one is linked with
	Gid, Name   string
	Balance, UD string // K$, two decimals
	Days        string // the balance in days of dividend
	Members     int64
	Unavailable bool
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
			g := coffeeGroup{Gid: gid, Name: c.C.GroupName(r.Context(), gid)}
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
			p.From, p.ToGroup = c.C.GroupName(r.Context(), gid), c.C.GroupName(r.Context(), tg)
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
	Gid, Name, Balance string
	MoveTo             []coffee.Link // linked groups you're also in
	Err, To            string
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
	p := leavePage{page: c.Home.newPage(r, "CoffeeBucks"), Gid: gid, Name: c.C.GroupName(ctx, gid), Err: r.FormValue("m"), To: r.FormValue("to")}
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
