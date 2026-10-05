package logs

import (
	"context"
	"strconv"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type Adapter struct{ Client *Client }

func New(c *Client) *Adapter { return &Adapter{Client: c} }

func (a *Adapter) Name() source.Name { return source.Logs }

func (a *Adapter) Fetch(ctx context.Context, ref source.RunRef) (source.Fragment, source.SourceStatus) {
	rows, st := a.Client.Query(ctx, "_time:"+Lookback+" "+RunFilter(ref)+" | stats count() as n")
	if st != source.StatusOK {
		return source.Fragment{}, st
	}
	n := 0
	if len(rows) > 0 {
		v, err := strconv.Atoi(rows[0]["n"])
		if err != nil {
			return source.Fragment{}, source.StatusUnavailable
		}
		n = v
	}
	return source.Fragment{Source: source.Logs, FetchedAt: time.Now().UTC(), Logs: &source.LogsFragment{Count: n}}, source.StatusOK
}

func (a *Adapter) ListLogs(ctx context.Context, ref source.RunRef, q source.LogQuery) ([]source.LogLine, source.SourceStatus) {
	query, err := BuildListQuery(ref, q)
	if err != nil {
		return nil, source.StatusUnavailable
	}
	rows, st := a.Client.Query(ctx, query)
	if st != source.StatusOK {
		return nil, st
	}
	lines := make([]source.LogLine, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, RowLine(r))
	}
	return lines, source.StatusOK
}

func (a *Adapter) Ping(ctx context.Context) source.SourceStatus { return a.Client.Ping(ctx) }
