package api

import "log/slog"

func (o *Ops) log() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// auditTranscript writes the one transcript.read line a read leaves: who read, which run, attempt
// and range, and how much came back. rd holds no content, and nothing else is logged.
func (o *Ops) auditTranscript(rd transcriptRead, err error) {
	via := "bearer"
	if rd.caller.ViaSession {
		via = "session"
	}
	attrs := []any{"principal", rd.caller.Principal.ID, "via", via, "run", rd.ref.String(), "attempt", rd.attempt,
		"seq_from", seqAttr(rd.from), "seq_to", seqAttr(rd.to), "item", rd.item, "full", rd.full}
	if err != nil {
		o.log().Warn("transcript.read", append(attrs, "code", AsAPIError(err).Code)...)
		return
	}
	o.log().Info("transcript.read", append(attrs, "items", rd.items, "redactions", rd.redactions)...)
}

// seqAttr is a bound as the audit line shows it: the number, or "open" for none (the session's
// live end, or a read refused before its range was known).
func seqAttr(p *int64) any {
	if p == nil {
		return "open"
	}
	return *p
}
