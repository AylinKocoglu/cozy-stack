// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
//
// Terms Of Service:
//
// there are no TOS at this moment, use at your own risk we take no responsibility
//
//     Schemes: https
//     Host: localhost
//     BasePath: /
//     Version: 0.0.1
//     License: AGPL-3.0 https://opensource.org/licenses/agpl-3.0
//     Contact: Bruno Michel <bruno@cozycloud.cc> https://cozy.io/
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package web

import (
	"github.com/gin-gonic/gin"
	"github.com/nono/cozy-stack/web/status"
)

// SetupRoutes sets the routing for HTTP endpoints to the Go methods
func SetupRoutes(router *gin.Engine) {
	status.Routes(router.Group("/status"))
}