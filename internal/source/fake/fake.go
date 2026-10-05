// Package fake provides a scripted Source for tests and a development data set that
// lets superwitness run end to end before the real evidence routes exist.
package fake

import (
	"context"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type Source struct {
	SourceName source.Name
	Fragment   source.Fragment
	Status     source.SourceStatus // "" or ok: answer with Fragment
	Delay      time.Duration       // wait this long, or until ctx ends (then timeout)
	Panic      bool
	Match      func(source.RunRef) bool // nil matches every ref
	Miss       *source.Fragment         // answered with ok when Match rejects; nil answers not_found

	mu   sync.Mutex
	seen []source.RunRef
}

func (s *Source) Name() source.Name { return s.SourceName }

func (s *Source) Fetch(ctx context.Context, ref source.RunRef) (source.Fragment, source.SourceStatus) {
	s.mu.Lock()
	s.seen = append(s.seen, ref)
	s.mu.Unlock()
	if s.Panic {
		panic("fake source " + string(s.SourceName) + " panicked")
	}
	if s.Delay > 0 {
		t := time.NewTimer(s.Delay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return source.Fragment{}, source.StatusTimeout
		}
	}
	if s.Status != "" && s.Status != source.StatusOK {
		return source.Fragment{}, s.Status
	}
	f := s.Fragment
	if s.Match != nil && !s.Match(ref) {
		if s.Miss == nil {
			return source.Fragment{}, source.StatusNotFound
		}
		f = *s.Miss
	}
	f.Source = s.SourceName
	f.FetchedAt = time.Now().UTC()
	return f, source.StatusOK
}

func (s *Source) Seen() []source.RunRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]source.RunRef(nil), s.seen...)
}
