package rpc

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

// some of the upstream examples
var localMethods = []string{"some_weirdo_method", "shh_newMessageFilter", "net_version"}

func TestRoutes(t *testing.T) {
	Convey("Router should route requests properly", t, func() {
		Convey("With remote enabled", func() {
			router := newRouter(true)
			Convey("Remote methods", func() {
				for _, method := range remoteMethods {
					got := router.routeRemote(method)
					So(got, ShouldBeTrue)
				}
			})
			Convey("Local methods", func() {
				for _, method := range localMethods {
					got := router.routeRemote(method)
					So(got, ShouldBeFalse)
				}
			})
		})
		Convey("Without remote", func() {
			router := newRouter(false)
			Convey("Remote methods", func() {
				for _, method := range remoteMethods {
					got := router.routeRemote(method)
					So(got, ShouldBeFalse)
				}
			})
			Convey("Local methods", func() {
				for _, method := range localMethods {
					got := router.routeRemote(method)
					So(got, ShouldBeFalse)
				}
			})
		})
	})
}
