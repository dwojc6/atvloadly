package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bitxeno/atvloadly/internal/ipa"
)

// AppleIcon is app artwork returned by Apple's App Store catalog.
type AppleIcon struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	ArtworkURL string `json:"artwork_url"`
	StoreURL   string `json:"store_url"`
}

type appleCatalogApp struct {
	ID           int64  `json:"trackId"`
	Name         string `json:"trackName"`
	Artist       string `json:"artistName"`
	Artwork      string `json:"artworkUrl512"`
	SmallArtwork string `json:"artworkUrl100"`
}

type appleIconCatalog struct {
	client  *http.Client
	baseURL string
}

var appleIcons = appleIconCatalog{
	client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many Apple redirects")
		}
		if req.URL.Scheme != "https" || (req.URL.Hostname() != "itunes.apple.com" && !appleArtworkURL(req.URL.String())) {
			return fmt.Errorf("unexpected Apple redirect")
		}
		return nil
	}},
	baseURL: "https://itunes.apple.com",
}

var appleStoreID = regexp.MustCompile(`(?:^|/)id([0-9]+)(?:$|[/?#])`)
var appleCountry = regexp.MustCompile(`^[a-z]{2}$`)

func appleIconCountry(country string) (string, error) {
	country = strings.ToLower(strings.TrimSpace(country))
	if country == "" {
		country = "us"
	}
	if !appleCountry.MatchString(country) {
		return "", fmt.Errorf("enter a two-letter store country code")
	}
	return country, nil
}

func (a appleIconCatalog) request(ctx context.Context, endpoint string, params url.Values) ([]appleCatalogApp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the Apple catalog is unavailable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the Apple catalog returned HTTP %d; try again later", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, fmt.Errorf("the Apple catalog response is too large")
	}
	var data struct {
		Results []appleCatalogApp `json:"results"`
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("invalid Apple catalog response: %w", err)
	}
	return data.Results, nil
}

// SearchAppleIcons searches by app name, then looks up tvOS editions in one
// batch. A direct App Store ID or URL can also be used for TV-only listings.
func SearchAppleIcons(ctx context.Context, term, country string, tv bool) ([]AppleIcon, error) {
	return appleIcons.search(ctx, term, country, tv)
}

func (a appleIconCatalog) search(ctx context.Context, term, country string, tv bool) ([]AppleIcon, error) {
	term = strings.TrimSpace(term)
	if len(term) < 2 || len(term) > 200 {
		return nil, fmt.Errorf("enter an app name between 2 and 200 characters")
	}
	country, err := appleIconCountry(country)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(term, 10, 64)
	if err != nil {
		if u, e := url.Parse(term); e == nil && u.Hostname() == "apps.apple.com" {
			if match := appleStoreID.FindStringSubmatch(u.Path); len(match) == 2 {
				id, _ = strconv.ParseInt(match[1], 10, 64)
			}
		}
	}
	entity := "software"
	if tv {
		entity = "tvSoftware"
	}
	var apps []appleCatalogApp
	if id > 0 {
		apps, err = a.request(ctx, "/lookup", url.Values{"id": {strconv.FormatInt(id, 10)}, "entity": {entity}, "country": {country}})
	} else {
		apps, err = a.request(ctx, "/search", url.Values{"term": {term}, "media": {"software"}, "entity": {"software"}, "country": {country}, "limit": {"25"}})
		if err == nil && tv && len(apps) > 0 {
			ids := make([]string, 0, len(apps))
			for _, app := range apps {
				if app.ID > 0 {
					ids = append(ids, strconv.FormatInt(app.ID, 10))
				}
			}
			if len(ids) > 0 {
				apps, err = a.request(ctx, "/lookup", url.Values{"id": {strings.Join(ids, ",")}, "entity": {entity}, "country": {country}})
			} else {
				apps = nil
			}
		}
	}
	if err != nil {
		return nil, err
	}
	result := []AppleIcon{}
	seen := map[int64]bool{}
	for _, app := range apps {
		artwork := app.Artwork
		if artwork == "" {
			artwork = app.SmallArtwork
		}
		if app.ID <= 0 || seen[app.ID] || !appleArtworkURL(artwork) {
			continue
		}
		seen[app.ID] = true
		// Apple's CDN fits the artwork into this box, preserving the TV aspect ratio.
		size := "1024x1024bb.jpg"
		if tv {
			size = "1280x768bb.jpg"
		}
		u, _ := url.Parse(artwork)
		parts := strings.Split(u.Path, "/")
		parts[len(parts)-1] = size
		u.Path = strings.Join(parts, "/")
		result = append(result, AppleIcon{ID: app.ID, Name: app.Name, Artist: app.Artist, ArtworkURL: u.String(), StoreURL: "https://apps.apple.com/" + country + "/app/id" + strconv.FormatInt(app.ID, 10) + "?platform=" + map[bool]string{true: "appleTV", false: "iphone"}[tv]})
	}
	return result, nil
}

func appleArtworkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") && strings.HasSuffix(host, ".mzstatic.com")
}

// DownloadAppleIcon snapshots Apple-hosted artwork into the same validated data
// URL as an uploaded image, so refreshes never depend on the remote URL.
func DownloadAppleIcon(ctx context.Context, rawURL string) (string, error) {
	if !appleArtworkURL(rawURL) {
		return "", fmt.Errorf("artwork must be hosted on Apple's image CDN")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := appleIcons.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download Apple icon: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the Apple artwork returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return "", err
	}
	if len(raw) > 4<<20 {
		return "", fmt.Errorf("the Apple icon is larger than 4 MB")
	}
	mime := http.DetectContentType(raw)
	if mime != "image/png" && mime != "image/jpeg" {
		return "", fmt.Errorf("the Apple artwork is not a PNG or JPEG")
	}
	data := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
	if _, err := ipa.DecodeCustomIcon(data); err != nil {
		return "", err
	}
	return data, nil
}
