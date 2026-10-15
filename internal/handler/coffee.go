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
}

// Show renders /coffee: your groups (balance, today's dividend), invites,
// starting a group.
func (c *Coffee) Show(w http.ResponseWriter, r *http.Request) {
	p := coffeePage{page: c.Home.newPage(r, "CoffeeBucks"), Focus: r.FormValue("g")}
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
			p.Groups = append(p.Groups, g)
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
