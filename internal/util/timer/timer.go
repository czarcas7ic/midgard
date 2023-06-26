// Reports averages and histograms.
// These metrics will show up at /debug/metrics as timer_* histograms
// There is a separate /debug/timers page for an overview of these metrics only.
package timer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pascaldekloe/metrics"
)

// Use NewTimer for construction
type Timer struct {
	histogram *metrics.Histogram
	latest    *[]time.Duration
}

var allTimers struct {
	sync.RWMutex
	timers []Timer
}

const namePrefix = "timer_"

func NewTimer(name string) (ret Timer) {
	ret = Timer{histogram: metrics.MustHistogram(
		namePrefix+name,
		"Timing histogram for : "+name,
		1e-6, 3e-6, 1e-5, 3e-5, 1e-4, 3e-4, 1e-3, 3e-3, 1e-2, 3e-1, 1e-1, 3e-1, 1, 3, 10, 30,
		60, 120, 180, 60*4, 60*5, 60*10, 60*15, 60*30),
		latest: &[]time.Duration{}}
	allTimers.Lock()
	allTimers.timers = append(allTimers.timers, ret)
	allTimers.Unlock()
	return ret
}

// Usage, note the final ():
// defer t.One()()
func (t *Timer) One() func() {
	t0 := time.Now()
	return func() {
		t.histogram.AddSince(t0)
		if len(*t.latest) < 5 {
			*t.latest = append(*t.latest, time.Since(t0))
		} else {
			*t.latest = append((*t.latest)[1:], time.Since(t0))
		}
	}
}

func (t *Timer) Print(w io.Writer) {
	fmt.Fprintf(w, "%s\n", t.histogram.Name()[len(namePrefix):])
	bucketValues := make([]uint64, 0, 20)
	bucketCounts, totalCount, totalSum := t.histogram.Get(bucketValues)
	durations := t.histogram.BucketBounds
	fmt.Fprintf(w, "    Count: %d\n", totalCount)
	if totalCount != 0 {
		fmt.Fprint(w, "    Average: ")
		writeFloatTime(w, totalSum/float64(totalCount))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, "    Histogram: ")
		cummulativeCount := uint64(0)
		for i := 0; i < len(bucketCounts); i++ {
			v := bucketCounts[i]
			if v != 0 {
				cummulativeCount += v
				writeIntTime(w, durations[i])
				fmt.Fprintf(w, ": %.3f%%, ", 100*float64(cummulativeCount)/float64(totalCount))
			}
		}
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, "    Latest: ")
		for i := len(*t.latest); i > 0; i-- {
			fmt.Fprintf(w, " %s, ", (*t.latest)[i-1].String())
		}
		fmt.Fprint(w, "\n")
	}
}

func (t *Timer) String() string {
	bb := bytes.Buffer{}
	t.Print(&bb)
	return bb.String()
}

// Usage, note the final ():
// defer t.Batch(10)()
//
// Note: this adds just one value for the full batch. Implications:
//   - the count at the summary page has to be multiplied with the average batch
//     size to get the true count.
//   - If batch sizes are different than this overrepresent small batches.
func (t *Timer) Batch(batchSize int) func() {
	t0 := time.Now()
	return func() {
		duration := float64(time.Now().UnixNano()-t0.UnixNano()) * 1e-9
		t.histogram.Add(duration / float64(batchSize))
	}
}

// Writes timing reports as json
func ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	allTimers.RLock()
	defer allTimers.RUnlock()
	for _, t := range allTimers.timers {
		t.Print(resp)
		fmt.Fprint(resp, "\n")
	}
}

func writeIntTime(w io.Writer, durationSec float64) {
	v, unit := normalize(durationSec)
	fmt.Fprintf(w, "%d%s", int(v), unit)
}

func writeFloatTime(w io.Writer, durationSec float64) {
	v, unit := normalize(durationSec)
	// Print only 3 digits out e.g. 1.23 ; 12.3 or 123
	if v < 10 {
		fmt.Fprintf(w, "%.2f%s", v, unit)
	} else if v < 100 {
		fmt.Fprintf(w, "%.1f%s", v, unit)
	} else {
		fmt.Fprintf(w, "%.0f%s", v, unit)
	}
}

func normalize(durationSec float64) (newValue float64, unit string) {
	if 1e-3 <= durationSec {
		newValue = durationSec * 1e3
		unit = "ms"
	} else if 1e-6 <= durationSec {
		newValue = durationSec * 1e6
		unit = "μs"
	} else {
		newValue = durationSec * 1e9
		unit = "Ns"
	}
	return
}
