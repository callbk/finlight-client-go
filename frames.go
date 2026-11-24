package finlight

import (
	"encoding/json"
	"fmt"
)

// This file is the WebSocket frame decoder (R-WS-1). It dispatches on "event"
// first. A frame with only "action" is a control frame in the v2 form, which
// the server sends before the first v3 subscribe of a connection (admit, pong,
// preempted, error); it is mapped onto the same v3 types. A v2 article frame
// (action "sendArticle") is never delivered as an article.

type frameKind int

const (
	frameIgnored frameKind = iota
	frameArticle
	frameAdmitted
	framePong
	framePreempted
	frameError
	frameOther
)

type frame struct {
	kind      frameKind
	event     StreamEventName
	article   *Article
	dataRaw   json.RawMessage
	admitted  AdmittedInfo
	preempted *PreemptedError
	errCode   ConnectionErrorCode
	errMsg    string
	errIssues []FieldIssue
	note      string // why a frame was ignored
	raw       json.RawMessage
	decodeErr error
}

func decodeFrame(text []byte) frame {
	f := frame{raw: append(json.RawMessage(nil), text...)}
	o, err := parseObject(text, "frame")
	if err != nil {
		f.note = "not a JSON object"
		f.decodeErr = err
		return f
	}
	if _, ok := o["event"]; ok {
		name, err := o.reqString("frame", "event")
		if err != nil {
			f.note = "event is not a string"
			f.decodeErr = err
			return f
		}
		f.event = StreamEventName(name)
		f.dataRaw = o["data"]
		decodeV3(&f, o)
		return f
	}
	if _, ok := o["action"]; ok {
		action, _ := o.plainString("frame", "action")
		decodeV2Control(&f, o, action)
		return f
	}
	f.note = "frame has neither event nor action"
	return f
}

func decodeV3(f *frame, o jsonObject) {
	switch f.event {
	case StreamEventArticleCreated, StreamEventArticleUpdated:
		a, err := decodeArticle(f.dataRaw, "data")
		if err != nil {
			f.kind = frameIgnored
			f.decodeErr = err
			f.note = "article frame could not be decoded"
			return
		}
		f.kind = frameArticle
		f.article = a
	case StreamEventConnectionAdmitted:
		f.kind = frameAdmitted
		if d, err := parseObject(f.dataRaw, "data"); err == nil {
			f.admitted = admittedFrom(d)
		}
	case StreamEventConnectionPong:
		f.kind = framePong
	case StreamEventConnectionPreempted:
		f.kind = framePreempted
		d, err := parseObject(f.dataRaw, "data")
		if err != nil {
			d = jsonObject{}
		}
		f.preempted = preemptedFrom(d)
	case StreamEventConnectionError:
		f.kind = frameError
		d, err := parseObject(f.dataRaw, "data")
		if err != nil {
			d = jsonObject{}
		}
		code, _ := d.plainString("data", "code")
		f.errCode = ConnectionErrorCode(code)
		f.errMsg, _ = d.plainString("data", "message")
		f.errIssues = parseFieldIssues(d["details"])
	default:
		f.kind = frameOther
	}
}

func decodeV2Control(f *frame, o jsonObject, action string) {
	switch action {
	case "admit":
		f.kind = frameAdmitted
		f.event = StreamEventConnectionAdmitted
		f.admitted = admittedFrom(o)
	case "pong":
		f.kind = framePong
		f.event = StreamEventConnectionPong
	case "preempted":
		f.kind = framePreempted
		f.event = StreamEventConnectionPreempted
		f.preempted = preemptedFrom(o)
	case "error":
		f.kind = frameError
		f.event = StreamEventConnectionError
		f.errMsg = v2ErrorText(o)
	default:
		// Includes the v2 article frame: never delivered as an article.
		f.note = fmt.Sprintf("v2 frame with action %q ignored", action)
	}
}

func v2ErrorText(o jsonObject) string {
	for _, k := range []string{"error", "data"} {
		raw, ok := o[k]
		if !ok || isJSONNull(raw) {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return string(raw)
	}
	return ""
}

func admittedFrom(o jsonObject) AdmittedInfo {
	var a AdmittedInfo
	a.LeaseID, _ = o.plainString("", "leaseId")
	if n, err := o.optInt64("", "serverNow"); err == nil && n != nil {
		a.ServerNow = *n
	}
	a.ClientNonce, _ = o.optString("", "clientNonce")
	return a
}

func preemptedFrom(o jsonObject) *PreemptedError {
	p := &PreemptedError{}
	reason, _ := o.plainString("", "reason")
	p.Reason = PreemptReason(reason)
	p.NewLeaseID, _ = o.optString("", "newLeaseId")
	if n, err := o.optInt64("", "timestamp"); err == nil && n != nil {
		p.Timestamp = *n
	}
	return p
}
