// Package dynhist implements dynamic histogram collector.
package dynhist

import (
	"fmt"
	"math"
	"runtime/metrics"
	"strings"
	"sync"
)

// DefaultBucketsLimit is a default maximum number of buckets.
const DefaultBucketsLimit = 20

// Collector groups and counts values by size using buckets.
type Collector struct {
	sync.Mutex

	// BucketsLimit limits total number of buckets used.
	BucketsLimit int

	// Bucket keeps total count.
	Bucket

	// Buckets is a list of available buckets.
	Buckets []Bucket

	// PrintSum enables printing of a summary value in a bucket.
	PrintSum bool

	// RawValues stores incoming events, disabled by default. Use non-nil value to enable.
	RawValues []float64

	// WeightFunc calculates weight of adjacent buckets with total available. Pair with minimal weight is merged.
	// AvgWidth is used by default.
	// See also LatencyWidth, ExpWidth.
	WeightFunc func(b1, b2, bTot Bucket) float64
}

// Bucket keeps count of values in boundaries.
type Bucket struct {
	Min   float64
	Max   float64
	Count int
	Sum   float64
}

// Snapshot is a consistent copy of a collector's distribution and bucket limit.
// Buckets and RawValues are detached from the collector and may be modified by
// the caller. RawValues is nil when retention was disabled.
type Snapshot struct {
	Bucket
	BucketsLimit int
	Buckets      []Bucket
	RawValues    []float64
}

// Snapshot copies totals, buckets, the bucket limit and retained raw values under
// one lock. It may be called concurrently with Add, AddN, Squash and other reads.
func (c *Collector) Snapshot() Snapshot {
	c.Lock()
	defer c.Unlock()

	snapshot := Snapshot{Bucket: c.Bucket, BucketsLimit: c.BucketsLimit}
	if c.Buckets != nil {
		snapshot.Buckets = make([]Bucket, len(c.Buckets))
		copy(snapshot.Buckets, c.Buckets)
	}

	if c.RawValues != nil {
		snapshot.RawValues = make([]float64, len(c.RawValues))
		copy(snapshot.RawValues, c.RawValues)
	}

	return snapshot
}

// ExpWidth creates a weight function with exponential bucket width growing.
//
// For exponentially distributed data, values of 1.2 for sumWidthPow and 1 for spacingPow should be a good fit.
// Increase sumWidthPow to widen buckets for lower values.
// Increase spacingPow to widen buckets for higher values.
func ExpWidth(sumWidthPow, spacingPow float64) func(b1, b2, bTot Bucket) float64 {
	return func(b1, b2, bTot Bucket) float64 {
		return math.Pow(b2.Max-b1.Min, sumWidthPow) / math.Pow(b2.Max-bTot.Min, spacingPow)
	}
}

// LatencyWidth is a weight function suitable for collecting latency information.
//
// It makes wider buckets for higher values, narrow buckets for lower values.
func LatencyWidth(b1, b2, bTot Bucket) float64 {
	return (b2.Max - b1.Min) / (b2.Max - bTot.Min)
}

// AvgWidth is a weight function to maintain equal width of all buckets.
//
// Should fit for normally distributed data.
func AvgWidth(b1, b2, bTot Bucket) float64 {
	return b2.Max - b1.Min
}

// Add collects one observation of v.
func (c *Collector) Add(v float64) {
	c.AddN(v, 1)
}

// AddN collects n observations of v under one lock and merges buckets once.
// It does nothing when n is zero and panics when n is negative or the total
// count would overflow int, leaving the collector unchanged in either case.
//
// With RawValues enabled, v is retained n times, using memory proportional to n.
// Otherwise collection cost is independent of n. A custom WeightFunc that uses
// counts or sums can produce different boundaries than n individual Add calls.
func (c *Collector) AddN(v float64, n int) { //nolint:funlen,cyclop
	if n < 0 {
		panic("dynhist: negative observation count")
	}

	if n == 0 {
		return
	}

	c.Lock()
	defer c.Unlock()

	if n > int(^uint(0)>>1)-c.Count {
		panic("dynhist: observation count overflows int")
	}

	defer func() { c.squash(c.BucketsLimit) }()

	if c.RawValues != nil {
		for i := 0; i < n; i++ {
			c.RawValues = append(c.RawValues, v)
		}
	}

	sum := float64(n) * v
	c.Count += n
	c.Sum += sum

	if len(c.Buckets) == 0 {
		if c.BucketsLimit == 0 {
			c.BucketsLimit = DefaultBucketsLimit
		}

		if c.WeightFunc == nil {
			c.WeightFunc = AvgWidth
		}

		c.Buckets = make([]Bucket, 1, c.BucketsLimit)
		c.Buckets[0].Min = v
		c.Buckets[0].Max = v
		c.Buckets[0].Count = n
		c.Buckets[0].Sum = sum

		c.Min = v
		c.Max = v

		return
	}

	if v < c.Min {
		c.Buckets = append([]Bucket{{Count: n, Min: v, Max: v, Sum: sum}}, c.Buckets...)
		c.Min = v

		return
	}

	if v > c.Max {
		c.Buckets = append(c.Buckets, Bucket{Count: n, Min: v, Max: v, Sum: sum})
		c.Max = v

		return
	}

	//  [1 3] [4 4] 5 [7 9]
	for i, b := range c.Buckets {
		if v >= b.Min {
			if v <= b.Max {
				c.Buckets[i].Count += n
				c.Buckets[i].Sum += sum

				return
			}
		} else {
			// Insert new bucket.
			c.Buckets = append(c.Buckets, Bucket{})
			copy(c.Buckets[i+1:], c.Buckets[i:])
			c.Buckets[i] = Bucket{Count: n, Min: v, Max: v, Sum: sum}

			return
		}
	}
}

// Squash sets the bucket limit and merges adjacent buckets until at most limit
// remain, using WeightFunc (AvgWidth by default). Existing buckets are never
// split. Counts, sums, observed bounds and RawValues are preserved.
// Subsequent additions respect the new limit. Squash panics if limit is not
// positive, leaving the collector unchanged.
func (c *Collector) Squash(limit int) {
	if limit <= 0 {
		panic("dynhist: bucket limit must be positive")
	}

	c.Lock()
	defer c.Unlock()

	c.BucketsLimit = limit
	c.squash(limit)
}

// squash requires the collector lock to be held.
func (c *Collector) squash(limit int) {
	if c.WeightFunc == nil {
		c.WeightFunc = AvgWidth
	}

	for len(c.Buckets) > limit {
		mergePoint := 1
		minWeight := c.WeightFunc(c.Buckets[0], c.Buckets[1], c.Bucket)

		for i := 2; i < len(c.Buckets); i++ {
			weight := c.WeightFunc(c.Buckets[i-1], c.Buckets[i], c.Bucket)
			if weight < minWeight {
				minWeight = weight
				mergePoint = i
			}
		}

		b1 := &c.Buckets[mergePoint-1]
		b2 := c.Buckets[mergePoint]
		b1.Count += b2.Count
		b1.Sum += b2.Sum
		b1.Max = b2.Max

		copy(c.Buckets[mergePoint:], c.Buckets[mergePoint+1:])
		c.Buckets = c.Buckets[:len(c.Buckets)-1]
	}
}

func isInt(f float64) bool {
	return f == float64(int(f))
}

// String renders buckets value.
func (c *Collector) String() string {
	c.Lock()
	defer c.Unlock()

	if len(c.Buckets) == 0 {
		return ""
	}

	hasIntBuckets := true

	for _, b := range c.Buckets {
		if !isInt(b.Min) || !isInt(b.Max) || !isInt(b.Sum) {
			hasIntBuckets = false

			break
		}
	}

	bucketFmt := "%.2f"
	statsFmt := "[%*.2f %*.2f] %*d %5.2f%%"
	sumFmt := " %*.2f"

	if hasIntBuckets {
		bucketFmt = "%.0f"
		statsFmt = "[%*.0f %*.0f] %*d %5.2f%%"
		sumFmt = " %*.0f"
	}

	nLen := printfLen(bucketFmt, c.Min)
	if maxLen := printfLen(bucketFmt, c.Max); maxLen > nLen {
		nLen = maxLen
	}
	// if c.Max is +Inf, the second-largest element can be the longest.
	if maxLen := printfLen(bucketFmt, c.Buckets[len(c.Buckets)-1].Min); maxLen > nLen {
		nLen = maxLen
	}

	cLen := printfLen("%d", c.Count)
	sLen := 0

	var res strings.Builder

	fmt.Fprintf(&res, "[%*s %*s] %*s total%%", nLen, "min", nLen, "max", cLen, "cnt")

	if c.PrintSum {
		sLen = printfLen(bucketFmt, c.Sum)
		fmt.Fprintf(&res, " %*s", sLen, "sum")
	}

	fmt.Fprintf(&res, " (total count: %d)\n", c.Count)

	for _, b := range c.Buckets {
		percent := float64(b.Count) * 100 / float64(c.Count)

		fmt.Fprintf(&res, statsFmt, nLen, b.Min, nLen, b.Max, cLen, b.Count, percent)

		if c.PrintSum {
			fmt.Fprintf(&res, sumFmt, sLen, b.Sum)
		}

		if dots := strings.Repeat(".", int(percent)); len(dots) > 0 {
			fmt.Fprint(&res, " ", dots)
		}

		fmt.Fprintln(&res)
	}

	return res.String()
}

// LoadFromRuntimeMetrics replaces existing buckets with data from metrics.Float64Histogram.
func (c *Collector) LoadFromRuntimeMetrics(h *metrics.Float64Histogram) {
	c.Lock()
	defer c.Unlock()

	c.Buckets = make([]Bucket, len(h.Buckets)-1)
	c.BucketsLimit = len(h.Buckets)
	c.Bucket = Bucket{
		Min: h.Buckets[0],
		Max: h.Buckets[0],
	}

	for i, b := range h.Buckets[1:] {
		bb := Bucket{
			Min:   c.Max,
			Max:   b,
			Count: int(h.Counts[i]), //nolint:gosec
		}

		if bb.Count != 0 && !math.IsInf(b, 0) {
			bb.Sum = float64(bb.Count) * b
			c.Sum += bb.Sum
		}

		c.Count += bb.Count
		c.Max = b

		c.Buckets[i] = bb
	}
}

func printfLen(format string, val interface{}) int {
	s := fmt.Sprintf(format, val)

	return len(s)
}

// Percentile returns the upper boundary of the bucket containing the
// ceil(percent * Count / 100)-th observation. It is an approximate nearest-rank
// percentile based on weighted counts, without interpolation within a bucket.
// Percent zero returns the lower boundary of the first nonempty bucket.
// Empty distributions and percentages outside [0, 100], including NaN, return NaN.
func (c *Collector) Percentile(percent float64) float64 {
	c.Lock()
	defer c.Unlock()

	value, _ := percentile(percent, c.Bucket, c.Buckets)

	return value
}

// Percentiles returns the requested percentile estimates in argument order,
// holding one lock so every estimate uses the same distribution. It has the same
// nearest-rank, boundary and NaN behavior as Percentile. No arguments returns nil.
func (c *Collector) Percentiles(percents ...float64) []float64 {
	if len(percents) == 0 {
		return nil
	}

	c.Lock()
	defer c.Unlock()

	values := make([]float64, len(percents))
	for i, percent := range percents {
		values[i], _ = percentile(percent, c.Bucket, c.Buckets)
	}

	return values
}

// PercentileSum returns the sum of all buckets through the bucket selected by
// Percentile. The selected bucket is included in full. Percent zero returns zero
// for a nonempty distribution. Empty distributions and invalid percentages return NaN.
func (c *Collector) PercentileSum(percent float64) float64 {
	c.Lock()
	defer c.Unlock()

	_, sum := percentile(percent, c.Bucket, c.Buckets)

	return sum
}

func percentile(percent float64, total Bucket, buckets []Bucket) (float64, float64) {
	if total.Count == 0 || math.IsNaN(percent) || percent < 0 || percent > 100 {
		return math.NaN(), math.NaN()
	}

	// Clamp before conversion: float64(MaxInt) can round up beyond int's range.
	target := total.Count

	if percent < 100 {
		rank := math.Ceil(percent * float64(total.Count) / 100)
		if rank < float64(total.Count) {
			target = int(rank)
			if target < 1 {
				target = 1
			}
		}
	}

	count, sum := 0, 0.0

	for _, b := range buckets {
		if b.Count == 0 {
			continue
		}

		if percent == 0 {
			return b.Min, 0
		}

		count += b.Count
		sum += b.Sum

		if count >= target {
			return b.Max, sum
		}
	}

	return math.NaN(), math.NaN()
}
