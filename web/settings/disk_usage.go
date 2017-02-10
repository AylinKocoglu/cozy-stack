// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

type apiDiskUsage struct {
	Used int64 `json:"used,string"`
}

func (j *apiDiskUsage) ID() string                             { return consts.DiskUsageID }
func (j *apiDiskUsage) Rev() string                            { return "" }
func (j *apiDiskUsage) DocType() string                        { return consts.Settings }
func (j *apiDiskUsage) SetID(_ string)                         {}
func (j *apiDiskUsage) SetRev(_ string)                        {}
func (j *apiDiskUsage) Relationships() jsonapi.RelationshipMap { return nil }
func (j *apiDiskUsage) Included() []jsonapi.Object             { return nil }
func (j *apiDiskUsage) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/disk-usage"}
}

// Settings objects permissions are only on ID
func (j *apiDiskUsage) Valid(k, f string) bool { return false }

func diskUsage(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var result apiDiskUsage

	// Check permissions, but also allow every request from the logged-in user
	// as this route is used by the cozy-bar from all the client-side apps
	if err := permissions.Allow(c, permissions.GET, &result); err != nil {
		if !middlewares.IsLoggedIn(c) {
			return err
		}
	}

	used, err := vfs.DiskUsage(instance)
	if err != nil {
		return err
	}

	result.Used = used
	return jsonapi.Data(c, http.StatusOK, &result, nil)
}
