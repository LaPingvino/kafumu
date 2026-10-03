package atp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// Scopes: only what Kafumu writes — posts, events and RSVPs.
var Scopes = []string{
	"atproto",
	"repo:app.bsky.feed.post?action=create",
	"repo:community.lexicon.calendar.event?action=create",
	"repo:community.lexicon.calendar.rsvp?action=create",
}

// Service wraps indigo's OAuth client app.
type Service struct {
	App    *oauth.ClientApp
	Origin string
	Brand  string
}

// New configures a public OAuth client whose metadata lives at
// origin/oauth/client-metadata.json (localhost gets the dev config).
func New(origin, brand string, store oauth.ClientAuthStore) *Service {
	cb := origin + "/oauth/callback"
	var cfg oauth.ClientConfig
	if u, err := url.Parse(origin); err == nil && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1") {
		cfg = oauth.NewLocalhostConfig("http://127.0.0.1:"+u.Port()+"/oauth/callback", Scopes)
	} else {
		cfg = oauth.NewPublicConfig(origin+"/oauth/client-metadata.json", cb, Scopes)
	}
	cfg.UserAgent = brand + " (+https://github.com/LaPingvino/kafumu)"
	return &Service{App: oauth.NewClientApp(&cfg, store), Origin: origin, Brand: brand}
}

// Metadata serves the client metadata document.
func (s *Service) Metadata(w http.ResponseWriter, r *http.Request) {
	doc := s.App.Config.ClientMetadata()
	name, uri := s.Brand, s.Origin
	doc.ClientName, doc.ClientURI = &name, &uri
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(doc)
}

// Start begins a login for a handle or DID and returns the URL to send the
// person to (their PDS's consent page).
func (s *Service) Start(ctx context.Context, identifier string) (string, error) {
	identifier = strings.TrimPrefix(strings.TrimSpace(identifier), "@")
	if identifier == "" || len(identifier) > 256 {
		return "", fmt.Errorf("atp: enter your handle, like you.bsky.social")
	}
	return s.App.StartAuthFlow(ctx, identifier)
}

// Finish completes the callback and returns the account DID and session id.
func (s *Service) Finish(ctx context.Context, q url.Values) (did, sessionID string, err error) {
	sd, err := s.App.ProcessCallback(ctx, q)
	if err != nil {
		return "", "", err
	}
	return sd.AccountDID.String(), sd.SessionID, nil
}

// Disconnect revokes and forgets a session.
func (s *Service) Disconnect(ctx context.Context, did, sessionID string) error {
	d, err := syntax.ParseDID(did)
	if err != nil {
		return err
	}
	return s.App.Logout(ctx, d, sessionID)
}

// CreateRecord writes a record to the person's own repo and returns its
// at:// URI.
func (s *Service) CreateRecord(ctx context.Context, did, sessionID, collection string, record map[string]any) (string, error) {
	d, err := syntax.ParseDID(did)
	if err != nil {
		return "", err
	}
	sess, err := s.App.ResumeSession(ctx, d, sessionID)
	if err != nil {
		return "", err
	}
	c := sess.APIClient()
	var out struct {
		URI string `json:"uri"`
	}
	body := map[string]any{"repo": did, "collection": collection, "record": record}
	if err := c.Post(ctx, "com.atproto.repo.createRecord", body, &out); err != nil {
		return "", err
	}
	return out.URI, nil
}

// Handle looks up the current handle for a DID on the public AppView.
func Handle(ctx context.Context, did string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.bsky.app/xrpc/app.bsky.actor.getProfile?actor="+url.QueryEscape(did), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var p struct {
		Handle string `json:"handle"`
	}
	json.NewDecoder(resp.Body).Decode(&p)
	return p.Handle
}

// Now is the ATproto datetime format.
func Now() string { return syntax.DatetimeNow().String() }
