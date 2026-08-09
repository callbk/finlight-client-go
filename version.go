package finlight

import (
	"runtime/debug"
	"sync"
)

const modulePath = "github.com/callbk/finlight-client-go"

// clientVersion reports the module version of this library as embedded in the
// consuming binary's build info ("devel" during local development). It is sent
// to the server in the x-client-version header and User-Agent.
var clientVersion = sync.OnceValue(func() string {
	version := "devel"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range bi.Deps {
			if dep.Path == modulePath {
				version = dep.Version
				break
			}
		}
	}
	return "go/finlight-client-go@" + version
})
