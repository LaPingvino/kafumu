package handler

import (
	"context"
	"github.com/LaPingvino/kafumu/internal/kv"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

// footerSettings: who runs this site, for "Contact the maker" in the footer.
// Set in /admin; the env (KAFUMU_MAKER, KAFUMU_CONTACT) gives the defaults.
type footerSettings struct {
	Maker       string `datastore:"maker,noindex"`
	ContactURL  string `datastore:"contact_url,noindex"`
	ContactText string `datastore:"contact_text,noindex"`
}

type footerCache struct {
	mu       sync.Mutex
	settings *footerSettings
	at       time.Time
	live     bool
	liveAt   time.Time
}

func footerKey() *datastore.Key { return datastore.NameKey("Config", "footer", nil) }

// footer returns what the footer shows: the maker only while their
// kafumu.com/@name link is live (checked at most every 5 minutes).
func (h *Home) footer(ctx context.Context) (maker, url, text string) {
	h.foot.mu.Lock()
	defer h.foot.mu.Unlock()
	now := time.Now()
	if h.foot.settings == nil || now.Sub(h.foot.at) > time.Minute {
		s := &footerSettings{Maker: h.Cfg.Maker, ContactURL: h.Cfg.Contact}
		if h.DB != nil {
			var stored footerSettings
			if err := h.DB.Get(ctx, footerKey(), &stored); err == nil {
				s = &stored
			}
		} else {
			stored := map[string]footerSettings{}
			kv.Load("Config", stored) // self-hosted
			if f, ok := stored["footer"]; ok {
				s = &f
			}
		}
		h.foot.settings, h.foot.at, h.foot.liveAt = s, now, time.Time{}
	}
	s := h.foot.settings
	if s.Maker != "" && now.Sub(h.foot.liveAt) > 5*time.Minute {
		h.foot.live = h.MakerLive != nil && h.MakerLive(ctx, s.Maker)
		h.foot.liveAt = now
	}
	if h.foot.live {
		maker = s.Maker
	}
	return maker, s.ContactURL, s.ContactText
}

// SaveFooter stores new footer settings (admin) and drops the cache.
func (h *Home) SaveFooter(ctx context.Context, s footerSettings) error {
	if h.DB != nil {
		if _, err := h.DB.Put(ctx, footerKey(), &s); err != nil {
			return err
		}
	} else {
		kv.Save("Config", "footer", s)
	}
	h.foot.mu.Lock()
	h.foot.settings, h.foot.liveAt = &s, time.Time{}
	if h.DB == nil {
		h.foot.at = time.Now().Add(24 * time.Hour) // memory only: keep it
	} else {
		h.foot.at = time.Now()
	}
	h.foot.mu.Unlock()
	return nil
}
