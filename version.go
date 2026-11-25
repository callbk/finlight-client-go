package finlight

// Version is the version of this SDK, without a leading "v". It is the single
// source for the User-Agent and x-client-version headers; the release workflow
// checks it against the git tag.
const Version = "2.0.0"

// PayloadVersion is the payload version this SDK speaks on the WebSocket and
// expects in webhook envelopes. The SDK always sends it as the JSON integer 3.
const PayloadVersion = 3

const (
	userAgent = "finlight-client-go/" + Version
	sdkLang   = "go"
)
