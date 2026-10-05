// Package errors (imported as errsrc) lists a run's error-level log lines. GlitchTip
// replaces it in sub-project F behind the same Source interface.
package errors

import (
	"context"
	"fmt"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/logs"
)

const MaxErrors = 50

type Adapter struct{ Client *logs.Client }

func New(c *logs.Client) *Adapter { return &Adapter{Client: c} }

func (a *Adapter) Name() source.Name { return source.Errors }

func Query(ref source.RunRef) string {
	lf, _ := logs.LevelFilter("error")
	return fmt.Sprintf("_time:%s %s %s | sort by (_time) | limit %d", logs.Lookback, logs.RunFilter(ref), lf, MaxErrors)
}

func (a *Adapter) Fetch(ctx context.Context, ref source.RunRef) (source.Fragment, source.SourceStatus) {
	rows, st := a.Client.Query(ctx, Query(ref))
	if st != source.StatusOK {
		return source.Fragment{}, st
	}
	out := make([]source.ErrorLine, 0, len(rows))
	for _, r := range rows {
		l := logs.RowLine(r)
		out = append(out, source.ErrorLine{Service: l.Service, Message: l.Message, TraceID: l.TraceID, At: l.At})
	}
	return source.Fragment{Source: source.Errors, FetchedAt: time.Now().UTC(), Errors: &source.ErrorsFragment{Errors: out}}, source.StatusOK
}

func (a *Adapter) Ping(ctx context.Context) source.SourceStatus { return a.Client.Ping(ctx) }
