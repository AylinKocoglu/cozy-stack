package client

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// AppManifest holds the JSON-API representation of an application.
type AppManifest struct {
	ID    string `json:"id"`
	Rev   string `json:"rev"`
	Attrs struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Source      string `json:"source"`
		State       string `json:"state"`
		Error       string `json:"error,omitempty"`
		Icon        string `json:"icon"`
		Description string `json:"description"`
		Developer   struct {
			Name string `json:"name"`
			URL  string `json:"url,omitempty"`
		} `json:"developer"`

		DefaultLocale string `json:"default_locale"`
		Locales       map[string]struct {
			Description string `json:"description"`
		} `json:"locales"`

		Version     string           `json:"version"`
		License     string           `json:"license"`
		Permissions *permissions.Set `json:"permissions"`
		Routes      map[string]struct {
			Folder string `json:"folder"`
			Index  string `json:"index"`
			Public bool   `json:"public"`
		} `json:"routes"`
	} `json:"attributes"`
}

// AppOptions holds the options to install an application.
type AppOptions struct {
	Slug      string
	SourceURL string
}

// InstallApp is used to install an application.
func (c *Client) InstallApp(opts *AppOptions) (*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method: "POST",
		// TODO replace QueryEscape with PathEscape when we will no longer support go 1.7
		Path:    "/apps/" + url.QueryEscape(opts.Slug),
		Queries: url.Values{"Source": {opts.SourceURL}},
		Headers: request.Headers{
			"Accept": "text/event-stream",
		},
	})
	if err != nil {
		return nil, err
	}
	return readAppManifest(res)
}

// UpdateApp is used to update an application.
func (c *Client) UpdateApp(opts *AppOptions) (*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method: "PUT",
		Path:   "/apps/" + url.QueryEscape(opts.Slug),
		Headers: request.Headers{
			"Accept": "text/event-stream",
		},
	})
	if err != nil {
		return nil, err
	}
	return readAppManifest(res)
}

func readAppManifest(res *http.Response) (*AppManifest, error) {
	evtch := make(chan *request.SSEEvent)
	go request.ReadSSE(res.Body, evtch)
	var lastevt *request.SSEEvent
	// get the last sent event
	for evt := range evtch {
		if evt.Error != nil {
			return nil, evt.Error
		}
		if evt.Name == "error" {
			return nil, errors.New(string(evt.Data))
		}
		lastevt = evt
	}
	if lastevt == nil {
		return nil, errors.New("No application data was sent")
	}
	app := &AppManifest{}
	if err := readJSONAPI(bytes.NewReader(lastevt.Data), &app, nil); err != nil {
		return nil, err
	}
	return app, nil
}