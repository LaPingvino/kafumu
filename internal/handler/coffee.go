package handler

import (
	"encoding/json"
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
